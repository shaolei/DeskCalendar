package calendar

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
	"github.com/shaolei/DeskCalendar/internal/state"
)

// TestCalendarService_GetDayInfo 验证聚合根组合农历+节假日+公历信息。
func TestCalendarService_GetDayInfo(t *testing.T) {
	date := time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local)
	sel := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)

	lunar := &fakeLunar{byDate: map[string]LunarInfo{
		date.Format("2006-01-02"): {DayStr: "初十", SolarTerm: "大暑", Zodiac: "马"},
	}}
	holiday := &fakeHoliday{
		holidays: map[string]bool{date.Format("2006-01-02"): true},
		names:    map[string]string{date.Format("2006-01-02"): "大暑假"},
	}

	svc := NewCalendarService(nil, lunar, holiday,
		WithSelected(sel), WithToday(date))

	info := svc.GetDayInfo(date)
	if info.Solar.Year != 2026 || info.Solar.Month != time.July || info.Solar.Day != 24 {
		t.Errorf("Solar = %+v, want 2026-07-24", info.Solar)
	}
	if info.Lunar.DayStr != "初十" || info.Lunar.SolarTerm != "大暑" {
		t.Errorf("Lunar = %+v, want DayStr=初十 SolarTerm=大暑", info.Lunar)
	}
	if !info.Holiday.IsHoliday || info.Holiday.Name != "大暑假" {
		t.Errorf("Holiday = %+v, want IsHoliday + Name=大暑假", info.Holiday)
	}
	if !info.IsToday {
		t.Error("IsToday should be true (today=2026-07-24)")
	}
	if info.IsSelected {
		t.Error("IsSelected should be false (selected=2026-07-01)")
	}
}

// TestCalendarService_SetSelectedDateEmits 验证 feature → state 的 emit（ADR-07a 实体证据）。
func TestCalendarService_SetSelectedDateEmits(t *testing.T) {
	bus := state.NewEventBus(log.Nop())
	var mu sync.Mutex
	var got state.DateChangedPayload
	var received bool

	_, _ = bus.Subscribe(state.TopicDateChanged, func(ctx context.Context, e state.Event) error {
		if p, ok := e.Payload.(state.DateChangedPayload); ok {
			mu.Lock()
			received = true
			got = p
			mu.Unlock()
		}
		return nil
	})

	svc := NewCalendarService(bus, &fakeLunar{}, &fakeHoliday{})
	sel := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	svc.SetSelectedDate(sel)

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("SetSelectedDate did not emit TopicDateChanged")
	}
	if got.Year != 2026 || got.Month != 3 || got.Day != 15 {
		t.Errorf("payload = %+v, want 2026-03-15", got)
	}
	if got.IsMonth {
		t.Error("selected-date change should have IsMonth=false")
	}
	if !svc.SelectedDate().Equal(sel) {
		t.Error("SelectedDate() did not reflect SetSelectedDate")
	}
}

// TestCalendarService_SetViewCurrentView 验证视图模式切换（MVP 周视图未启用，仅内部状态）。
func TestCalendarService_SetViewCurrentView(t *testing.T) {
	svc := NewCalendarService(nil, &fakeLunar{}, &fakeHoliday{})
	if svc.CurrentView() != ViewMonth {
		t.Error("default view should be ViewMonth")
	}
	svc.SetView(ViewWeek)
	if svc.CurrentView() != ViewWeek {
		t.Error("CurrentView should be ViewWeek after SetView")
	}
}

// TestCalendarService_VisibleRange 验证月/周视图可见范围。
func TestCalendarService_VisibleRange(t *testing.T) {
	sel := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	svc := NewCalendarService(nil, &fakeLunar{}, &fakeHoliday{}, WithSelected(sel))

	// 月视图：当月首末日。
	start, end := svc.VisibleRange()
	if start.Day() != 1 || start.Month() != time.July {
		t.Errorf("month start = %v, want 2026-07-01", start)
	}
	if end.Day() != 31 || end.Month() != time.July {
		t.Errorf("month end = %v, want 2026-07-31", end)
	}

	// 周视图：选中周首末（周一~周日）。
	svc.SetView(ViewWeek)
	ws, we := svc.VisibleRange()
	if ws.Weekday() != time.Monday {
		t.Errorf("week start weekday = %v, want Monday", ws.Weekday())
	}
	if we.Sub(ws).Hours() != 24*6 {
		t.Errorf("week span = %v, want 6 days", we.Sub(ws))
	}
}
