package weather

import (
	"context"
	"sync"
	"time"
)

// Snapshot 是 Service 当前状态的不可变快照（app 读取后映射为 ui.Model）。
// 结构体值拷贝，避免调用方持有内部指针导致的数据竞争。
type Snapshot struct {
	Status   Status
	Current  *Weather
	Forecast []*Weather
	Stale    bool // 降级旧数据（Status=Stale 时为 true）
}

// Service 对上层（ui/app）暴露的唯一入口：拉取 + 读缓存 + 降级。
//
// 并发模型：所有状态变更发生在 Refresh（可能为后台 goroutine）；读取经 Snapshot()
// 在锁内拷贝。回调 onUpdate 在后台 goroutine 调用，必须非阻塞（仅发 channel），
// 不得直写共享状态——与 app 单写者/双循环铁律一致。
type Service struct {
	mu       sync.Mutex
	provider WeatherProvider
	cache    *Cache
	loc      Location
	ttl      time.Duration

	status   Status
	current  *Weather
	forecast []*Weather
	stale    bool

	stop     chan struct{}
	stopOnce sync.Once
	onUpdate func()
}

// NewService 装配 provider 与缓存（缓存见 Cache.md）。cacheDir 不存在则创建。
func NewService(cfg Config, cacheDir string, ttl time.Duration) (*Service, error) {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	cache, err := NewCache(cacheDir, ttl)
	if err != nil {
		return nil, err
	}
	if err := cache.Load(); err != nil {
		logger.Warn("weather: load cache", "err", err)
	}
	s := &Service{
		provider: NewProvider(cfg),
		cache:    cache,
		loc:      Location{Lat: cfg.Lat, Lng: cfg.Lng},
		ttl:      ttl,
		status:   StatusLoading,
		stop:     make(chan struct{}),
	}
	// 启动即用磁盘旧数据即时降级（若在宽限内），首个面板弹出即可见旧天气。
	if old, fresh, gerr := cache.Get(s.loc, time.Now()); gerr == nil && old != nil {
		s.current = old
		s.stale = !fresh
		s.status = StatusStale
	}
	return s, nil
}

// SetOnUpdate 注册刷新完成后的回调（如向主循环发 CmdRender）。在后台 goroutine 调用，
// 回调必须非阻塞（仅发 channel），不得直写共享状态（单写者铁律）。
func (s *Service) SetOnUpdate(fn func()) {
	s.mu.Lock()
	s.onUpdate = fn
	s.mu.Unlock()
}

// Refresh 后台拉取并写缓存；失败不返回 error 阻塞调用方，内部转降级状态。
// 完成后经 onUpdate 回调通知上层重渲（若已注册）。
func (s *Service) Refresh(ctx context.Context) {
	cur, err := s.provider.Current(ctx, s.loc)
	var fc []*Weather
	if err == nil {
		fc, _ = s.provider.Forecast(ctx, s.loc, 3)
	}

	s.mu.Lock()
	if err == nil && cur != nil {
		s.current = cur
		s.forecast = fc
		s.stale = false
		s.status = StatusReady
		if cerr := s.cache.Set(s.loc, time.Now(), cur); cerr != nil {
			logger.Warn("weather: cache set", "err", cerr)
		}
	} else {
		// 降级：回退缓存（断网/超时/key 错）。只要命中旧缓存即标 Stale
		// （设计 §6：失败路径一律降级为 Stale，不论缓存是否仍在 TTL 内——
		// 既然本次刷新失败，展示的数据至少已是"上一轮"的，显示「旧数据」角标合理）。
		old, _, gerr := s.cache.Get(s.loc, time.Now())
		if gerr == nil && old != nil {
			s.current = old
			s.forecast = nil
			s.stale = true
			s.status = StatusStale
			logger.Warn("weather: degrade to cache", "err", err)
		} else {
			s.current = nil
			s.forecast = nil
			s.stale = false
			s.status = StatusDisabled
			logger.Warn("weather: unavailable", "err", err)
		}
	}
	fn := s.onUpdate
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Snapshot 返回当前状态拷贝（线程安全）。
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur *Weather
	var fc []*Weather
	if s.current != nil {
		c := *s.current
		cur = &c
	}
	for _, w := range s.forecast {
		if w != nil {
			c := *w
			fc = append(fc, &c)
		}
	}
	return Snapshot{Status: s.status, Current: cur, Forecast: fc, Stale: s.stale}
}

// Start 启动 30min 定时刷新（每 ttl 一次），并异步触发首次刷新。
// 首次刷新异步进行，不阻塞 app.Run 启动路径；完成后经 onUpdate 触发重渲。
func (s *Service) Start(ctx context.Context) {
	go s.Refresh(ctx)
	go func() {
		ticker := time.NewTicker(s.ttl)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Refresh(ctx)
			}
		}
	}()
}

// Stop 停止后台刷新 goroutine（幂等）。
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}
