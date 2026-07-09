package calendar

import (
	"testing"
	"time"
)

// TestLunarGoService_SolarToLunar 用真实 lunar-go 验证抽样映射正确性。
func TestLunarGoService_SolarToLunar(t *testing.T) {
	svc := NewLunarService()

	// 2026-07-23 → 大暑（节气优先于农历日）。
	d1 := time.Date(2026, 7, 23, 0, 0, 0, 0, time.Local)
	info1 := svc.SolarToLunar(d1)
	if info1.SolarTerm != "大暑" {
		t.Errorf("SolarTerm(2026-07-24) = %q, want 大暑", info1.SolarTerm)
	}
	if info1.Zodiac != "马" {
		t.Errorf("Zodiac(2026) = %q, want 马", info1.Zodiac)
	}

	// 2026-02-17 = 农历正月初一（春节）：MonthStr=正月，DayStr=初一，LunarMonth=1，非闰月。
	d2 := time.Date(2026, 2, 17, 0, 0, 0, 0, time.Local)
	info2 := svc.SolarToLunar(d2)
	if info2.MonthStr != "正月" {
		t.Errorf("MonthStr(2026-02-17) = %q, want 正月", info2.MonthStr)
	}
	if info2.DayStr != "初一" {
		t.Errorf("DayStr(2026-02-17) = %q, want 初一", info2.DayStr)
	}
	if info2.LunarMonth != 1 || info2.LeapMonth {
		t.Errorf("LunarMonth/Leap = %d/%v, want 1/false", info2.LunarMonth, info2.LeapMonth)
	}

	// 干支抽样：2026 为 丙午年。
	if info2.GanZhiYear != "丙午" {
		t.Errorf("GanZhiYear(2026) = %q, want 丙午", info2.GanZhiYear)
	}
}
