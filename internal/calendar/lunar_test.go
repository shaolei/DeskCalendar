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

// TestLunarService_LeapMonth_Golden 锁死闰月分支行为（S7）。
// 已知 2023-03-22 为农历 闰二月初一：GetMonth() 返回负值，月份应取绝对值且 LeapMonth=true，
// MonthStr 应为「闰二月」，DayStr「初一」。此测试钉死 lunar-go 的闰月语义，防止第三方升级漂移。
func TestLunarService_LeapMonth_Golden(t *testing.T) {
	svc := NewLunarService()
	date := time.Date(2023, 3, 22, 0, 0, 0, 0, time.Local)
	info := svc.SolarToLunar(date)
	if !info.LeapMonth {
		t.Errorf("LeapMonth = false, want true for 2023-03-22 (闰二月初一); got %+v", info)
	}
	if info.LunarMonth != 2 {
		t.Errorf("LunarMonth = %d, want 2 (abs of 闰二月)", info.LunarMonth)
	}
	if info.LunarDay != 1 {
		t.Errorf("LunarDay = %d, want 1 (初一)", info.LunarDay)
	}
	if info.MonthStr != "闰二月" {
		t.Errorf("MonthStr = %q, want 闰二月", info.MonthStr)
	}
	if info.DayStr != "初一" {
		t.Errorf("DayStr = %q, want 初一", info.DayStr)
	}
}
