// Package state 持有进程内响应式状态（Signal/Store）与领域事件总线 EventBus。
//
// 事件总线归属本包（ADR-07a）：核心 feature（calendar/ui/theme/shell）经 state.Publish
// emit 领域事件；插件经 plugin.Host.Subscribe 委托到本总线订阅。依赖方向严格单向：
// feature → state、plugin → state；plugin 绝不反向编译依赖 feature（依赖倒置铁律）。
package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
)

// EventTopic 事件主题（字符串别名，便于扩展自定义主题）。
type EventTopic string

// 核心领域事件主题常量（插件可订阅）。
const (
	TopicDateChanged      EventTopic = "calendar.date.changed"
	TopicPanelShown       EventTopic = "ui.panel.shown"
	TopicPanelHidden      EventTopic = "ui.panel.hidden"
	TopicThemeChanged     EventTopic = "theme.changed"
	TopicLifecycleChanged EventTopic = "app.lifecycle.changed"
)

// Event 一个领域事件。
type Event struct {
	Topic   EventTopic
	Payload any
	At      time.Time
}

// EventHandler 事件处理器；返回 error 仅被记录，不阻断其他 handler。
// 严禁在 handler 内阻塞主线程或做长 IO（应起 goroutine + channel 回写）。
type EventHandler func(ctx context.Context, e Event) error

// Unsubscribe 退订句柄。
type Unsubscribe func()

// EventBus 进程内发布-订阅总线。
type EventBus interface {
	// Subscribe 订阅某主题，返回退订句柄。
	Subscribe(topic EventTopic, h EventHandler) (Unsubscribe, error)
	// Publish 同步派发（逐 handler，panic 被 recover 隔离）。
	Publish(ctx context.Context, e Event)
	// PublishAsync 异步派发到独立 goroutine（不阻塞发送方）。
	PublishAsync(ctx context.Context, e Event)
}

// DateChangedPayload 日期变更载荷。
type DateChangedPayload struct {
	Year    int
	Month   int
	Day     int
	IsMonth bool // true=整月切换，false=单日选中变更
}

// PanelPayload 面板显隐载荷。
type PanelPayload struct {
	Visible bool
	X, Y    int
	W, H    int
}

// ThemeChangedPayload 主题变更载荷。
type ThemeChangedPayload struct {
	Dark   bool
	Scheme string // 主题方案名
}

// LifecyclePayload App 生命周期载荷。
type LifecyclePayload struct {
	State string // starting/running/stopping
}

// subscription 带唯一 token 的订阅记录，便于精确退订。
type subscription struct {
	token   uint64
	handler EventHandler
}

// inProcessBus 默认实现（线程安全）。
type inProcessBus struct {
	mu        sync.RWMutex
	handlers  map[EventTopic][]subscription
	nextToken uint64
	log       log.Logger
}

// NewEventBus 构造默认总线。
func NewEventBus(logger log.Logger) EventBus {
	return &inProcessBus{
		handlers: map[EventTopic][]subscription{},
		log:      logger,
	}
}

// Subscribe 注册 handler，返回可精确退订的 Unsubscribe。
func (b *inProcessBus) Subscribe(topic EventTopic, h EventHandler) (Unsubscribe, error) {
	if h == nil {
		return nil, fmt.Errorf("nil handler for topic %q", topic)
	}
	b.mu.Lock()
	b.nextToken++
	tok := b.nextToken
	b.handlers[topic] = append(b.handlers[topic], subscription{token: tok, handler: h})
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		hs := b.handlers[topic]
		for i, s := range hs {
			if s.token == tok {
				b.handlers[topic] = append(hs[:i], hs[i+1:]...)
				break
			}
		}
	}, nil
}

// Publish 同步派发。
func (b *inProcessBus) Publish(ctx context.Context, e Event) {
	// S4 审查结论：调用方未显式设时间戳时，总线自动补章，使 At 成为总线契约的一部分，
	// 避免某 feature 漏设 time.Now() 导致下游按时间排序/去重的订阅者拿到零值时间。
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.mu.RLock()
	hs := append([]subscription(nil), b.handlers[e.Topic]...)
	b.mu.RUnlock()
	for _, s := range hs {
		h := s.handler
		func() {
			defer func() {
				if r := recover(); r != nil {
					b.log.Error("event handler panic", "topic", e.Topic, "panic", r)
				}
			}()
			if err := h(ctx, e); err != nil {
				b.log.Warn("event handler error", "topic", e.Topic, "err", err)
			}
		}()
	}
}

// PublishAsync 异步派发。
func (b *inProcessBus) PublishAsync(ctx context.Context, e Event) {
	go b.Publish(ctx, e)
}
