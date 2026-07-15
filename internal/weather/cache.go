package weather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrCacheMiss 表示既无新鲜数据也无可用旧缓存。
var ErrCacheMiss = fmt.Errorf("weather: cache miss")

// Entry 单条缓存记录（同时是磁盘 JSON 的结构）。
type Entry struct {
	Weather    *Weather  `json:"weather"`
	FetchedAt  time.Time `json:"fetched_at"`
	ExpiresAt  time.Time `json:"expires_at"`  // 新鲜截止（TTL 之后）
	StaleUntil time.Time `json:"stale_until"` // 降级宽限截止
}

// Cache 内存 + 磁盘双级缓存。并发安全（sync.Mutex 保护 mem）。
type Cache struct {
	mu  sync.Mutex
	mem map[string]*Entry
	dir string
	ttl time.Duration
}

// NewCache 创建缓存；dir 不存在则创建。ttl 默认 30min。
func NewCache(dir string, ttl time.Duration) (*Cache, error) {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("weather: make cache dir: %w", err)
	}
	return &Cache{
		mem: make(map[string]*Entry),
		dir: dir,
		ttl: ttl,
	}, nil
}

// Key 缓存键：坐标(4位小数) + 日期。同坐标同日共享一条。
func (c *Cache) Key(loc Location, date time.Time) string {
	return fmt.Sprintf("%.4f,%.4f@%s", loc.Lat, loc.Lng, date.Format("2006-01-02"))
}

// Get 读取：新鲜返回 (w,true,nil)；过期但在宽限内返回 (w,false,nil) 降级；
// 否则 (nil,false,ErrCacheMiss)。内存未命中回退磁盘。
func (c *Cache) Get(loc Location, date time.Time) (*Weather, bool, error) {
	key := c.Key(loc, date)
	c.mu.Lock()
	e, ok := c.mem[key]
	c.mu.Unlock()
	if !ok {
		// 回退磁盘
		e = c.loadFile(key)
		if e == nil {
			return nil, false, ErrCacheMiss
		}
		c.mu.Lock()
		c.mem[key] = e // 回填内存
		c.mu.Unlock()
	}
	now := time.Now()
	switch {
	case now.Before(e.ExpiresAt):
		return e.Weather, true, nil
	case now.Before(e.StaleUntil):
		return e.Weather, false, nil // 降级：返回旧数据
	default:
		return nil, false, ErrCacheMiss
	}
}

// Set 写入内存并落盘。staleUntil = expiresAt + ttl（宽限一个周期）。
func (c *Cache) Set(loc Location, date time.Time, w *Weather) error {
	if w == nil {
		return fmt.Errorf("weather: set nil")
	}
	now := time.Now()
	e := &Entry{
		Weather:    w,
		FetchedAt:  now,
		ExpiresAt:  now.Add(c.ttl),
		StaleUntil: now.Add(2 * c.ttl),
	}
	key := c.Key(loc, date)
	c.mu.Lock()
	c.mem[key] = e
	c.mu.Unlock()
	return c.saveFile(key, e)
}

// file 将 key 转安全文件名。Key() 仅含数字/./,/@/-（均已文件名安全），
// 故只替换理论上的非法字符 "/" 与 ":"；生成的文件名可经 TrimSuffix(".json")
// 精确还原为原 key，保证 Load 回填的 mem 键与 Get 计算的键一致。
func (c *Cache) file(key string) string {
	safe := strings.NewReplacer("/", "_", ":", "_").Replace(key)
	return filepath.Join(c.dir, safe+".json")
}

func (c *Cache) saveFile(key string, e *Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("weather: marshal cache: %w", err)
	}
	return os.WriteFile(c.file(key), b, 0o600)
}

func (c *Cache) loadFile(key string) *Entry {
	b, err := os.ReadFile(c.file(key))
	if err != nil {
		return nil
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil
	}
	return &e
}

// Load 启动时把磁盘缓存批量读入内存（进程重启后即时降级可用）。
func (c *Cache) Load() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil // 目录空/不存在不致命
	}
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.dir, f.Name()))
		if err != nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(b, &e); err != nil {
			continue
		}
		// key 从文件名还原（去掉 .json 即原 key；file() 的安全替换对 Key()
		// 生成的字符串是恒等变换，故无需逆映射）。
		k := strings.TrimSuffix(f.Name(), ".json")
		c.mu.Lock()
		c.mem[k] = &e
		c.mu.Unlock()
	}
	return nil
}
