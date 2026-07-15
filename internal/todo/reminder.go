package todo

import (
	"context"
	"fmt"
	"time"
)

// Notifier 是提醒通知的最小接口（由 platform.Notification 的实现满足）。
// 接口隔离：todo 包不反向依赖 platform，仅依赖方法子集。
type Notifier interface {
	// Notify 弹出一条系统通知。title 为标题，body 为内容。
	Notify(ctx context.Context, title, body string) error
}

// TodoStore 是 state/todo Store 对本模块暴露的最小接口，用于广播提醒事件。
// 运行时本仓未使用 internal/state（见 model.go 顶部说明），故该字段可为 nil，
// tick 中做 nil 守卫——不阻塞提醒核心逻辑。
type TodoStore interface {
	// EmitReminderFired 在提醒触发后通知 Store，驱动 UI 高亮。
	EmitReminderFired(t *Todo)
}

// SchedulerConfig 调度器配置（Value Object）。
type SchedulerConfig struct {
	Interval     time.Duration // 扫描间隔，默认 30s
	ImmediateRun bool          // 启动即扫描一次，默认 true
}

// ReminderScheduler 提醒调度器接口（可替换/可 mock）。
type ReminderScheduler interface {
	// Start 启动定时扫描；ctx 取消即停止。重复调用返回 error。
	Start(ctx context.Context) error
	// Stop 立即停止（或依赖 ctx.Done）。
	Stop() error
}

// TodoLister 是提醒调度器对仓储的最小依赖（仅扫描所需）。TodoRepository 与
// TodoService 均实现 List，故二者皆可注入——app 直接传 *todo.Service 即可，
// 无需额外持有底层仓储引用。
type TodoLister interface {
	List(ctx context.Context, filter ListFilter) ([]*Todo, error)
}

// ReminderService 默认实现：组合 Repository/Service + Notifier + Store。
type ReminderService struct {
	repo     TodoLister
	notifier Notifier
	store    TodoStore
	cfg      SchedulerConfig

	// fired 记录本次运行已弹过的提醒 ID（进程内去重）。
	fired map[string]struct{}
	// cancel 用于 Stop。
	cancel context.CancelFunc
}

// NewReminderService 构造调度器。repo 可为 *todo.Service 或任意 TodoRepository；
// store 可为 nil（无 state 广播时）。
func NewReminderService(repo TodoLister, notifier Notifier, store TodoStore, cfg SchedulerConfig) *ReminderService {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	return &ReminderService{
		repo:     repo,
		notifier: notifier,
		store:    store,
		cfg:      cfg,
		fired:    make(map[string]struct{}),
	}
}

// Start 实现 ReminderScheduler：启动 ticker 并立即/周期扫描。
func (s *ReminderService) Start(ctx context.Context) error {
	if s.cancel != nil {
		return fmt.Errorf("todo: reminder already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	if s.cfg.ImmediateRun {
		_ = s.tick(runCtx)
	}
	go func() {
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				_ = s.tick(runCtx)
			}
		}
	}()
	return nil
}

// Stop 实现 ReminderScheduler。
func (s *ReminderService) Stop() error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	s.cancel = nil
	return nil
}

// tick 执行一次扫描：查出 active 待办，逐条判定到期并弹通知（进程内去重）。
func (s *ReminderService) tick(ctx context.Context) error {
	now := time.Now()
	filter := ListFilter{Status: ptr(StatusActive)} // 仅未完成
	todos, err := s.repo.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("todo: reminder scan: %w", err)
	}
	for _, t := range todos {
		if t.ReminderAt == nil || !t.IsDueForReminder(now) {
			continue
		}
		if _, ok := s.fired[t.ID]; ok {
			continue // 进程内去重，避免重复弹
		}
		s.fired[t.ID] = struct{}{}
		body := t.Title
		if t.Due != nil {
			body = fmt.Sprintf("%s（截止 %s）", t.Title, t.Due.Format("15:04"))
		}
		if err := s.notifier.Notify(ctx, "待办提醒", body); err != nil {
			log.Warn("todo: notify failed", "id", t.ID, "err", err)
			continue
		}
		if s.store != nil {
			s.store.EmitReminderFired(t)
		}
	}
	return nil
}

// ptr 为 Status 取址辅助（构造 *Status）。
func ptr(s Status) *Status { return &s }
