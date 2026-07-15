package todo

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// ── newID 辅助（放在此处便于单测与阅读）─────────────────────────────

// randRead 包装 crypto/rand.Read（变量形式便于测试注入）。
var randRead = func(b []byte) (int, error) { return rand.Read(b) }

// timeNow 包装 time.Now（变量形式便于测试注入固定时钟）。
var timeNow = time.Now

// formatUUID 把 16 字节（已置版本/变体位）格式化为 8-4-4-4-12 的 UUID 串。
func formatUUID(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// formatUUIDFromInts 由整数段拼接 UUID 串（降级路径用）。
func formatUUIDFromInts(a uint32, b, c, d uint16, e int64) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%04x%08x",
		a, b, c, d, uint16(e>>32), uint32(e))
}

// ── Service ───────────────────────────────────────────────────────

// Service 是 TodoService 的默认实现：组合 TodoRepository 落地领域规则。
type Service struct {
	repo TodoRepository
	now  func() time.Time // 可注入时钟（默认 time.Now），便于单测
}

// NewService 构造服务。repo 不可为 nil（否则 List 等方法将 panic，由调用方保证）。
func NewService(repo TodoRepository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Add 创建待办：标题去空格、非空校验；生成 ID 与 CreatedAt；委托 repo 落库。
func (s *Service) Add(ctx context.Context, title string, opts AddOpts) (*Todo, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrEmptyTitle
	}
	now := s.now()
	t := &Todo{
		ID:         newID(),
		Title:      title,
		Due:        opts.Due,
		Status:     StatusActive,
		Tags:       dedupeTags(opts.Tags),
		ReminderAt: opts.ReminderAt,
		CreatedAt:  now,
	}
	if err := s.repo.Add(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Complete 标记完成（active->done）。已 done 幂等返回原对象。
func (s *Service) Complete(ctx context.Context, id string) (*Todo, error) {
	t, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status == StatusDone {
		return t, nil
	}
	t.Status = StatusDone
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Reopen 重新打开（done->active）。已 active 幂等返回原对象。
func (s *Service) Reopen(ctx context.Context, id string) (*Todo, error) {
	t, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status == StatusActive {
		return t, nil
	}
	t.Status = StatusActive
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Snooze 推迟提醒时间（仅当存在 ReminderAt）。at 即新的提醒时刻。
func (s *Service) Snooze(ctx context.Context, id string, at time.Time) (*Todo, error) {
	t, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ReminderAt == nil {
		return nil, ErrInvalidStatus
	}
	t.ReminderAt = &at
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Remove 删除待办。不存在返回 ErrNotFound。
func (s *Service) Remove(ctx context.Context, id string) error {
	return s.repo.Remove(ctx, id)
}

// List 列出（透传过滤条件给仓储）。
func (s *Service) List(ctx context.Context, filter ListFilter) ([]*Todo, error) {
	return s.repo.List(ctx, filter)
}

// QueryByDate 按日查询（透传给仓储）。
func (s *Service) QueryByDate(ctx context.Context, day time.Time) ([]*Todo, error) {
	return s.repo.QueryByDate(ctx, day)
}

// get 按 ID 取出待办（遍历仓储全部，因接口未暴露 Get-by-ID）。
func (s *Service) get(ctx context.Context, id string) (*Todo, error) {
	if id == "" {
		return nil, ErrInvalidID
	}
	all, err := s.repo.List(ctx, ListFilter{})
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

// dedupeTags 去重并剔除空标签，保持原顺序。
func dedupeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tg := range tags {
		tg = strings.TrimSpace(tg)
		if tg == "" {
			continue
		}
		if _, ok := seen[tg]; ok {
			continue
		}
		seen[tg] = struct{}{}
		out = append(out, tg)
	}
	return out
}
