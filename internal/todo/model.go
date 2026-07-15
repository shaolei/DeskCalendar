// Package todo 实现 v1.1 待办领域（聚合根 + 仓储 + 服务 + 提醒调度）。
//
// 设计契约见 docs/60-Todo/{Model,Reminder,SQLite}.md。本包零 CGO、纯 stdlib +
// internal/infra/log，不依赖 platform/state——保持领域层纯净，便于单测与后续
// 存储/通知实现替换（接口隔离、可逆）。
//
// 关于运行时的关键事实（与 60-Todo 设计文档的差异说明）：
//   - 设计文档假设 internal/state 的 Signal 流转与 gogpu/ui 控件树；但本仓运行时
//     （路径 D / ADR-08）使用 gg 即时模式光栅化 + Model 快照重渲，并不存在
//     internal/state 的运行时依赖。因此本包不向 state 广播，仅暴露 TodoService /
//     ReminderService 供 app 在重渲时按需读取。
//   - 设计文档的 SQLite 落地（modernc.org/sqlite，纯 Go 但 ~50MB）与本仓轻量零 CGO
//     取向不符，且沙箱拉取风险高；故提供 JSONFileRepository 作为可逆默认持久化，
//     TodoRepository 接口保持不变，后续可无缝替换为 SQLite。
package todo

import (
	"context"
	"errors"
	"time"
)

// Status 表示待办的完成状态（字符串枚举，便于 JSON/SQLite 存储）。
type Status string

const (
	StatusActive Status = "active" // 进行中
	StatusDone   Status = "done"   // 已完成
)

// 领域错误，供调用方区分处理。
var (
	ErrNotFound      = errors.New("todo: not found")
	ErrInvalidID     = errors.New("todo: invalid id")
	ErrInvalidStatus = errors.New("todo: invalid status")
	ErrEmptyTitle    = errors.New("todo: empty title")
)

// Todo 是待办聚合根（Aggregate Root）。
// 所有时间字段统一使用 time.Time（本地时区），持久化时序列化为 RFC3339 字符串；
// *time.Time 字段用 omitempty，nil 表示"无"。
type Todo struct {
	ID         string     `json:"id"`                    // 唯一标识（UUID v4）
	Title      string     `json:"title"`                 // 标题，不可为空
	Due        *time.Time `json:"due,omitempty"`         // 截止时间，nil 表示无期限
	Status     Status     `json:"status"`                // active | done
	Tags       []string   `json:"tags"`                  // 标签集合（去重）
	ReminderAt *time.Time `json:"reminder_at,omitempty"` // 提醒时间，nil 表示不提醒
	CreatedAt  time.Time  `json:"created_at"`            // 创建时间（不可变）
}

// IsOverdue 领域规则：未完成且已过截止时间（即"延期"）。
// 已完成的待办不算延期；无 Due 的待办永不延期。
func (t *Todo) IsOverdue(now time.Time) bool {
	return t.Status == StatusActive && t.Due != nil && now.After(*t.Due)
}

// IsDueForReminder 领域规则：未完成、已设置提醒、且当前时间已到达（含超过）提醒时刻。
// 完成态的待办不会触发提醒；无 ReminderAt 的待办永远不触发。
func (t *Todo) IsDueForReminder(now time.Time) bool {
	return t.Status == StatusActive && t.ReminderAt != nil && !now.Before(*t.ReminderAt)
}

// ListFilter 是 List 查询的可选过滤条件（Value Object）。
// 所有字段零值表示"不过滤"，由 Repository 实现自行组合。
type ListFilter struct {
	Status  *Status    // 按状态过滤，nil 表示不过滤
	Tag     string     // 按标签过滤（精确匹配），空表示不过滤
	From    *time.Time // Due 区间下界（含），nil 表示不限
	To      *time.Time // Due 区间上界（含），nil 表示不限
	OnlyDue bool       // true 时仅返回设置了 Due 的待办
}

// AddOpts 是创建待办的可选参数（Value Object）。
type AddOpts struct {
	Due        *time.Time // 截止时间，nil 表示无期限
	Tags       []string   // 标签，可空
	ReminderAt *time.Time // 提醒时间，nil 表示不提醒
}

// TodoRepository 持久化接口（接口隔离，零 CGO 存储可替换、可 mock）。
type TodoRepository interface {
	// Add 新增一条待办，ID 由调用方在构造 Todo 时填充（UUID）。
	Add(ctx context.Context, t *Todo) error
	// Update 全量更新（按 ID 覆盖字段）；不存在返回 ErrNotFound。
	Update(ctx context.Context, t *Todo) error
	// Remove 按 ID 删除；不存在返回 ErrNotFound。
	Remove(ctx context.Context, id string) error
	// List 按过滤条件列出（按 Due 升序、无 Due 置底、再按 CreatedAt 升序排序）。
	List(ctx context.Context, filter ListFilter) ([]*Todo, error)
	// QueryByDate 返回 Due 落在 day 当天 [00:00, 24:00) 的待办（含已完成）。
	QueryByDate(ctx context.Context, day time.Time) ([]*Todo, error)
}

// TodoService 领域服务接口：在落库前后施加领域规则。
type TodoService interface {
	// Add 创建待办（标题去空格、非空校验；生成 ID 与 CreatedAt）。
	Add(ctx context.Context, title string, opts AddOpts) (*Todo, error)
	// Complete 标记完成（active->done）。已 done 幂等返回原对象。
	Complete(ctx context.Context, id string) (*Todo, error)
	// Reopen 重新打开（done->active）。
	Reopen(ctx context.Context, id string) (*Todo, error)
	// Snooze 推迟提醒时间（仅当存在 ReminderAt）。
	Snooze(ctx context.Context, id string, at time.Time) (*Todo, error)
	// Remove 删除。
	Remove(ctx context.Context, id string) error
	// List 列出。
	List(ctx context.Context, filter ListFilter) ([]*Todo, error)
	// QueryByDate 按日查询。
	QueryByDate(ctx context.Context, day time.Time) ([]*Todo, error)
}
