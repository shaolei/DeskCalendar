package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
	"github.com/shaolei/DeskCalendar/internal/state"
)

func newTestStores() *state.Stores {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	return &state.Stores{
		Calendar: state.NewCalendarState(now),
		Theme:    state.NewThemeState(),
		UI:       state.NewUIState(),
	}
}

// 模拟双循环：生产者线程 Enqueue，主线程 OnUpdate 内 Pump。
func TestDispatcher_EnqueuePump_SetsSignal(t *testing.T) {
	stores := newTestStores()
	d := state.NewDispatcher(stores, log.Nop())

	if stores.UI.Visible().Get() {
		t.Fatal("initial visible should be false")
	}

	d.Enqueue(state.CmdShow{})
	d.Pump()
	if !stores.UI.Visible().Get() {
		t.Fatal("after CmdShow+Pump, visible should be true")
	}

	d.Enqueue(state.CmdToggle{})
	d.Pump()
	if stores.UI.Visible().Get() {
		t.Fatal("after CmdToggle, visible should be false")
	}

	sel := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	d.Enqueue(state.CmdSelectDate{Date: sel})
	d.Pump()
	if got := stores.Calendar.SelectedDate().Get(); !got.Equal(sel) {
		t.Fatalf("selectedDate = %v, want %v", got, sel)
	}
	if got := stores.Calendar.DisplayedMonth().Get(); got.Month() != time.March {
		t.Fatalf("displayedMonth month = %v, want March", got.Month())
	}

	d.Enqueue(state.CmdSetTheme{Mode: state.ThemeDark})
	d.Pump()
	if got := stores.Theme.Mode().Get(); got != state.ThemeDark {
		t.Fatalf("theme mode = %v, want ThemeDark", got)
	}

	pos := state.Position{X: 100, Y: 200}
	d.Enqueue(state.CmdSetPosition{Pos: pos})
	d.Pump()
	if got := stores.UI.Position().Get(); got != pos {
		t.Fatalf("position = %v, want %v", got, pos)
	}
}

func TestDispatcher_Pump_NoCommandsReturnsImmediately(t *testing.T) {
	stores := newTestStores()
	d := state.NewDispatcher(stores, log.Nop())
	d.Pump() // 不应阻塞或 panic
}

func TestDispatcher_Enqueue_NonBlockingWhenFull(t *testing.T) {
	stores := newTestStores()
	d := state.NewDispatcher(stores, log.Nop())
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			d.Enqueue(state.CmdShow{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked when channel full")
	}
	d.Pump()
}

func TestDispatcher_SubscriptionNotified(t *testing.T) {
	stores := newTestStores()
	d := state.NewDispatcher(stores, log.Nop())

	got := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stores.UI.Visible().Subscribe(ctx, func(v bool) {
		select {
		case got <- v:
		default:
		}
	})

	d.Enqueue(state.CmdShow{})
	d.Pump()

	select {
	case v := <-got:
		if !v {
			t.Fatal("subscriber should receive true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber not notified")
	}
}

func TestDispatcher_UnknownCommandLogged(t *testing.T) {
	stores := newTestStores()
	d := state.NewDispatcher(stores, log.Nop())
	d.Enqueue(unknownCmd{})
	d.Pump() // 命中 default 分支，不应 panic
}

type unknownCmd struct{}

func (unknownCmd) Name() string { return "unknown" }
