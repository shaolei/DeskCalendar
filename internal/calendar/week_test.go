package calendar

import (
	"testing"
	"time"
)

func TestGenWeekGrid_Structure(t *testing.T) {
	// 2026-07-24 是周四；WeekStart=Monday → Start=2026-07-20(周一)，End=2026-07-26(周日)。
	anchor := time.Date(2026, 7, 24, 12, 0, 0, 0, time.Local)
	opts := GridOptions{WeekStart: time.Monday, Selected: anchor}
	wg := GenWeekGrid(anchor, opts)

	if !wg.Start.Equal(time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)) {
		t.Errorf("Start = %v, want 2026-07-20", wg.Start)
	}
	if !wg.End.Equal(time.Date(2026, 7, 26, 0, 0, 0, 0, time.Local)) {
		t.Errorf("End = %v, want 2026-07-26", wg.End)
	}
	if !wg.Days[0].Date.Equal(wg.Start) || !wg.Days[6].Date.Equal(wg.End) {
		t.Error("Days[0]/Days[6] should equal Start/End")
	}
	if wg.Days[0].Date.Weekday() != time.Monday {
		t.Errorf("Days[0] weekday = %v, want Monday", wg.Days[0].Date.Weekday())
	}
	for i := 0; i < 7; i++ {
		if !wg.Days[i].InCurrentMonth {
			t.Errorf("week view Days[%d].InCurrentMonth should be true", i)
		}
	}
	if !wg.Days[4].IsSelected { // anchor 周四 = index 4
		t.Error("anchor (Thursday) cell should be IsSelected")
	}
}

func TestGenWeekGrid_CrossMonthAnchor(t *testing.T) {
	// 2026-07-01 是周三；WeekStart=Monday → Start=2026-06-29(周一)，跨月。
	anchor := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	wg := GenWeekGrid(anchor, GridOptions{WeekStart: time.Monday})
	if !wg.Start.Equal(time.Date(2026, 6, 29, 0, 0, 0, 0, time.Local)) {
		t.Errorf("cross-month Start = %v, want 2026-06-29", wg.Start)
	}
}
