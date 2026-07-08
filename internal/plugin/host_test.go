package plugin

import (
	"context"
	"sync"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
	"github.com/shaolei/DeskCalendar/internal/state"
)

// TestHostSubscribeReceivesCoreEmit 验证「核心 feature emit → 插件 Host 订阅」闭环：
// feature 侧直接调用 state.EventBus.Publish（真实项目里由 calendar.Service 代劳），
// 经 plugin.Manager（实现 Host）订阅的 handler 能收到事件。
//
// 关键点：本测试不 import internal/calendar——feature 与 plugin 仅靠 state 的
// 事件契约（state.Event / state.TopicDateChanged / state.DateChangedPayload）解耦，
// 而非互相 import。这正是 ADR-07a 依赖倒置铁律的实体证据。
func TestHostSubscribeReceivesCoreEmit(t *testing.T) {
	bus := state.NewEventBus(log.Nop())
	mgr := NewManager(bus, log.Nop())

	var mu sync.Mutex
	var gotYear int
	var received bool
	unsub, err := mgr.Subscribe(state.TopicDateChanged, func(ctx context.Context, e state.Event) error {
		if p, ok := e.Payload.(state.DateChangedPayload); ok {
			mu.Lock()
			received = true
			gotYear = p.Year
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	// feature 侧 emit（真实项目里由 calendar.Service.EmitDateChanged 调用）。
	bus.Publish(context.Background(), state.Event{
		Topic:   state.TopicDateChanged,
		Payload: state.DateChangedPayload{Year: 2026, Month: 7, Day: 8},
	})

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("plugin Host handler did not receive the core emit")
	}
	if gotYear != 2026 {
		t.Fatalf("payload year = %d, want 2026", gotYear)
	}
}

// TestHostHasNoPublish 在编译期即断言：plugin.Host 接口不提供 Publish 方法。
// 若未来有人在 Host 上误加 Publish，本测试会因类型断言失败而编译报错，
// 把「插件只能订阅不能 emit」的铁律钉死在类型系统里。
func TestHostHasNoPublish(t *testing.T) {
	var h Host = NewManager(state.NewEventBus(log.Nop()), log.Nop())

	type publisher interface {
		Publish(ctx context.Context, e state.Event)
	}
	if _, ok := h.(publisher); ok {
		t.Fatal("Host MUST NOT expose Publish — 插件只能订阅核心事件，不能 emit")
	}
}
