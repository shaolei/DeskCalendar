package calendar

import (
	"testing"
	"time"
)

// TestLunarService_Contract 验证 fake 满足 LunarService 接口，且 SolarToLunar 被调用返回预期。
func TestLunarService_Contract(t *testing.T) {
	date := time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local)
	want := LunarInfo{DayStr: "初十", SolarTerm: "大暑"}
	var svc LunarService = &fakeLunar{byDate: map[string]LunarInfo{date.Format("2006-01-02"): want}}

	got := svc.SolarToLunar(date)
	// LunarInfo 含 []string 字段不可直接用 == 比较，逐字段核对关键项。
	if got.DayStr != want.DayStr || got.SolarTerm != want.SolarTerm {
		t.Errorf("SolarToLunar = %+v, want DayStr=%q SolarTerm=%q", got, want.DayStr, want.SolarTerm)
	}
}

// TestLunarInfo_ZeroValueSafe 验证未注入农历时网格不产生 nil 恐慌（空 LunarInfo 安全）。
func TestLunarInfo_ZeroValueSafe(t *testing.T) {
	grid := GenMonthGrid(2026, time.July, GridOptions{WeekStart: time.Monday}) // 无 Lunar 注入
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			if grid.Weeks[r][c].Lunar.SolarTerm != "" {
				t.Error("expected empty LunarInfo when no LunarService injected")
			}
		}
	}
}
