// Package plugin 是插件宿主层（Post-MVP 骨架）。
//
// 依赖方向（ADR-07a）：本包仅 import internal/state 来订阅领域事件，
// 绝不 import internal/calendar 等 feature 包。插件经 Host.Subscribe 委托到
// state 的 EventBus 订阅；Host 不暴露 Publish——保证插件只能订阅、不能 emit
// 核心领域事件，核心状态唯一真源不被插件逆向改写。
package plugin

import (
	"github.com/shaolei/DeskCalendar/internal/state"
)

// Logger 插件可用的极简日志接口。
// 在 plugin 包内自定，避免 plugin 反向依赖 infra/log 的具体实现类型。
type Logger interface {
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Info(msg string, args ...any)
}

// Host 是插件在运行时拿到的宿主能力接口。
// 插件经 Host.Subscribe 订阅领域事件——委托给 state 的 EventBus（ADR-07a）。
// 注意：Host 不提供 Publish，从类型层面封锁插件 emit 核心事件的能力。
type Host interface {
	// Subscribe 订阅某主题，返回退订句柄。
	Subscribe(topic state.EventTopic, h state.EventHandler) (state.Unsubscribe, error)
	// Log 返回宿主日志能力（插件侧诊断）。
	Log() Logger
}

// Manager 实现 Host，把订阅委托给 state.EventBus。
type Manager struct {
	bus state.EventBus
	log Logger
}

// NewManager 构造插件管理器，注入事件总线与日志。
func NewManager(bus state.EventBus, log Logger) *Manager {
	return &Manager{bus: bus, log: log}
}

// Subscribe 委托到 state.EventBus.Subscribe。
func (m *Manager) Subscribe(topic state.EventTopic, h state.EventHandler) (state.Unsubscribe, error) {
	return m.bus.Subscribe(topic, h)
}

// Log 返回宿主日志。
func (m *Manager) Log() Logger { return m.log }
