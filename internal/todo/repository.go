package todo

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// filterAndSort 对给定待办集合施加 ListFilter 并排序，返回新切片（不修改入参）。
// 排序规则（与设计文档建议一致）：Due 升序（nil Due 置底），其次 CreatedAt 升序，
// 最后 ID 升序以保证稳定顺序。List 与 QueryByDate 均复用此排序保证可见顺序一致。
func filterAndSort(todos []*Todo, f ListFilter) []*Todo {
	out := make([]*Todo, 0, len(todos))
	for _, t := range todos {
		if f.Status != nil && t.Status != *f.Status {
			continue
		}
		if f.Tag != "" {
			hit := false
			for _, tag := range t.Tags {
				if tag == f.Tag {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if f.OnlyDue && t.Due == nil {
			continue
		}
		if f.From != nil && (t.Due == nil || t.Due.Before(*f.From)) {
			continue
		}
		if f.To != nil && (t.Due == nil || t.Due.After(*f.To)) {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Due == nil) != (b.Due == nil) {
			return a.Due != nil && b.Due == nil // 有 Due 在前，无 Due 置底
		}
		if a.Due != nil && !a.Due.Equal(*b.Due) {
			return a.Due.Before(*b.Due)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return out
}

// inDay 判断 t 是否落在 day 当天本地 [00:00, 24:00) 区间（含已完成）。
func inDay(t *time.Time, day time.Time) bool {
	if t == nil {
		return false
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)
	return !t.Before(start) && t.Before(end)
}

// MemoryRepository 纯内存实现（单测/临时场景）。非持久化，进程退出即丢。
type MemoryRepository struct {
	mu    sync.Mutex
	items map[string]*Todo
}

// NewMemoryRepository 构造空的内存仓储。
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]*Todo)}
}

func (r *MemoryRepository) Add(_ context.Context, t *Todo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		return ErrInvalidID
	}
	r.items[t.ID] = t
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, t *Todo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[t.ID]; !ok {
		return ErrNotFound
	}
	r.items[t.ID] = t
	return nil
}

func (r *MemoryRepository) Remove(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *MemoryRepository) List(_ context.Context, f ListFilter) ([]*Todo, error) {
	r.mu.Lock()
	all := make([]*Todo, 0, len(r.items))
	for _, t := range r.items {
		all = append(all, t)
	}
	r.mu.Unlock()
	return filterAndSort(all, f), nil
}

func (r *MemoryRepository) QueryByDate(_ context.Context, day time.Time) ([]*Todo, error) {
	r.mu.Lock()
	all := make([]*Todo, 0, len(r.items))
	for _, t := range r.items {
		all = append(all, t)
	}
	r.mu.Unlock()
	out := make([]*Todo, 0)
	for _, t := range all {
		if inDay(t.Due, day) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// JSONFileRepository 基于单个 JSON 文件的持久化实现（离线优先、零 CGO、轻量）。
// 每次写操作重写整文件（待办量小，足够）；读操作全量加载。文件不可用时（首次运行
// 或文件损坏）降级为内存态，不影响进程运行。
type JSONFileRepository struct {
	mu   sync.Mutex
	path string
	// items 为内存镜像；构造时从文件加载，之后所有读写均经它并最终落盘。
	items map[string]*Todo
}

// NewJSONFileRepository 构造 JSON 文件仓储并加载既有数据。
// path 父目录不存在时自动创建；文件不存在视为空仓储；文件损坏仅记日志并当空处理。
func NewJSONFileRepository(path string) (*JSONFileRepository, error) {
	r := &JSONFileRepository{path: path, items: make(map[string]*Todo)}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	if err := r.load(); err != nil {
		if os.IsNotExist(err) {
			return r, nil // 首次运行：空仓储
		}
		log.Warn("todo: load repository failed, starting empty", "path", path, "err", err)
		return r, nil
	}
	return r, nil
}

func (r *JSONFileRepository) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var list []*Todo
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	r.items = make(map[string]*Todo, len(list))
	for _, t := range list {
		if t != nil && t.ID != "" {
			r.items[t.ID] = t
		}
	}
	return nil
}

func (r *JSONFileRepository) save() error {
	list := make([]*Todo, 0, len(r.items))
	for _, t := range r.items {
		list = append(list, t)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

func (r *JSONFileRepository) Add(_ context.Context, t *Todo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		return ErrInvalidID
	}
	r.items[t.ID] = t
	return r.save()
}

func (r *JSONFileRepository) Update(_ context.Context, t *Todo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[t.ID]; !ok {
		return ErrNotFound
	}
	r.items[t.ID] = t
	return r.save()
}

func (r *JSONFileRepository) Remove(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return r.save()
}

func (r *JSONFileRepository) List(_ context.Context, f ListFilter) ([]*Todo, error) {
	r.mu.Lock()
	all := make([]*Todo, 0, len(r.items))
	for _, t := range r.items {
		all = append(all, t)
	}
	r.mu.Unlock()
	return filterAndSort(all, f), nil
}

func (r *JSONFileRepository) QueryByDate(_ context.Context, day time.Time) ([]*Todo, error) {
	r.mu.Lock()
	all := make([]*Todo, 0, len(r.items))
	for _, t := range r.items {
		all = append(all, t)
	}
	r.mu.Unlock()
	out := make([]*Todo, 0)
	for _, t := range all {
		if inDay(t.Due, day) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
