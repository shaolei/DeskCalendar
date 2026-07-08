package calendar

import (
	"context"
	"sync"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
	"github.com/shaolei/DeskCalendar/internal/state"
)

// TestServiceEmitsDateChanged 验证 feature → state 的 emit 方向在真实编译/运行中成立：
// calendar.Service 经 state.EventBus.Publish 广播，订阅者（同样只依赖 state）能收到。
func TestServiceEmitsDateChanged(t *testing.T) {
	bus := state.NewEventBus(log.Nop())

	var mu sync.Mutex
	var received bool
	var year int
	_, _ = bus.Subscribe(state.TopicDateChanged, func(ctx context.Context, e state.Event) error {
		if p, ok := e.Payload.(state.DateChangedPayload); ok {
			mu.Lock()
			received = true
			year = p.Year
			mu.Unlock()
		}
		return nil
	})

	svc := NewService(bus)
	svc.EmitDateChanged(context.Background(), 2026, 7, 8, false)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("calendar emit was not received by a state subscriber")
	}
	if year != 2026 {
		t.Fatalf("payload year = %d, want 2026", year)
	}
}
