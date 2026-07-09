package calendar

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// holidayFS 在编译期嵌入烘焙的节假日数据（ADR-05：每年构建期烘焙）。
// 当前离线兜底为 internal/calendar/embed/holidays/YYYY.json（SEED，发布前替换）。
//
//go:embed embed/holidays/*.json
var holidayFS embed.FS

// yearFile 是单个年度节假日文件的解析结构。
// 键为 "MM-DD"；holidays 为法定节假日名，workdays 为调休补班标注。
type yearFile struct {
	Holidays map[string]string `json:"holidays"`
	Workdays map[string]string `json:"workdays"`
}

// embedHolidayRepo 基于内嵌 JSON 的 HolidayRepository 真实实现。
// 线程安全（只读，Refresh 仅重放 embed，不触网）。
type embedHolidayRepo struct {
	mu       sync.RWMutex
	holidays map[string]string // "YYYY-MM-DD" -> 名称
	workdays map[string]string // "YYYY-MM-DD" -> 名称（补班）
}

// NewHolidayRepository 构造并立即加载内嵌数据。失败返回 error（调用方须处理）。
func NewHolidayRepository() (*embedHolidayRepo, error) {
	r := &embedHolidayRepo{}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// load 解析 embed 中所有 YYYY.json，按文件名年份补全日期键。
func (r *embedHolidayRepo) load() error {
	holidays := make(map[string]string)
	workdays := make(map[string]string)

	entries, err := holidayFS.ReadDir("embed/holidays")
	if err != nil {
		return fmt.Errorf("read embed holidays dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		yearStr := strings.TrimSuffix(e.Name(), ".json")
		year, perr := strconv.Atoi(yearStr)
		if perr != nil {
			return fmt.Errorf("holiday file %q: bad year in name: %w", e.Name(), perr)
		}
		data, rerr := holidayFS.ReadFile("embed/holidays/" + e.Name())
		if rerr != nil {
			return fmt.Errorf("read embed %q: %w", e.Name(), rerr)
		}
		var yf yearFile
		if jerr := json.Unmarshal(data, &yf); jerr != nil {
			return fmt.Errorf("parse embed %q: %w", e.Name(), jerr)
		}
		for mmdd, name := range yf.Holidays {
			key, kerr := joinDate(year, mmdd)
			if kerr != nil {
				return fmt.Errorf("holiday file %q: %w", e.Name(), kerr)
			}
			holidays[key] = name
		}
		for mmdd, name := range yf.Workdays {
			key, kerr := joinDate(year, mmdd)
			if kerr != nil {
				return fmt.Errorf("holiday file %q: %w", e.Name(), kerr)
			}
			workdays[key] = name
		}
	}
	if len(holidays) == 0 {
		return fmt.Errorf("no holiday data loaded from embed")
	}

	r.mu.Lock()
	r.holidays = holidays
	r.workdays = workdays
	r.mu.Unlock()
	return nil
}

// joinDate 把文件名年份与 "MM-DD" 拼成 "YYYY-MM-DD"，并校验合法性。
func joinDate(year int, mmdd string) (string, error) {
	parts := strings.SplitN(mmdd, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid MM-DD key %q", mmdd)
	}
	m, err1 := strconv.Atoi(parts[0])
	d, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("invalid MM-DD key %q", mmdd)
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return "", fmt.Errorf("out-of-range MM-DD key %q", mmdd)
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, m, d), nil
}

func (r *embedHolidayRepo) IsHoliday(d time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.holidays[d.Format("2006-01-02")]
	return ok
}

func (r *embedHolidayRepo) IsWorkday(d time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.workdays[d.Format("2006-01-02")]
	return ok
}

func (r *embedHolidayRepo) Name(d time.Time) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := d.Format("2006-01-02")
	if n, ok := r.holidays[key]; ok {
		return n
	}
	if n, ok := r.workdays[key]; ok {
		return n
	}
	return ""
}

// Refresh 重放内嵌数据。MVP 阶段不触网；真实 holiday-cn 烘焙接入后，
// 此处可改为读取构建期生成的外部文件/远端，失败回退 embed。
func (r *embedHolidayRepo) Refresh(ctx context.Context) error {
	return r.load()
}

// compile-time 接口符合检查（保证 embedHolidayRepo 满足 HolidayRepository）。
var _ HolidayRepository = (*embedHolidayRepo)(nil)
