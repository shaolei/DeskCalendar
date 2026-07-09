package calendar

import (
	"context"
	"testing"
	"time"
)

// TestEmbedHolidayRepository_Loads 验证内嵌种子数据可被构造并查询。
func TestEmbedHolidayRepository_Loads(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if !repo.IsHoliday(d) {
		t.Error("IsHoliday(2026-01-01) = false, want true (元旦)")
	}
	if name := repo.Name(d); name != "元旦" {
		t.Errorf("Name(2026-01-01) = %q, want 元旦", name)
	}
}

// TestEmbedHolidayRepository_Workday 验证调休补班日识别（且不误判为节假日）。
func TestEmbedHolidayRepository_Workday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 2, 14, 0, 0, 0, 0, time.Local) // 春节补班（种子）
	if !repo.IsWorkday(d) {
		t.Error("IsWorkday(2026-02-14) = false, want true (春节补班)")
	}
	if repo.IsHoliday(d) {
		t.Error("IsWorkday day 2026-02-14 should NOT be IsHoliday")
	}
	if name := repo.Name(d); name != "春节补班" {
		t.Errorf("Name(2026-02-14) = %q, want 春节补班", name)
	}
}

// TestEmbedHolidayRepository_NonHoliday 验证普通日既非节假日也非补班。
func TestEmbedHolidayRepository_NonHoliday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	if repo.IsHoliday(d) {
		t.Error("IsHoliday(2026-03-10) = true, want false")
	}
	if repo.IsWorkday(d) {
		t.Error("IsWorkday(2026-03-10) = true, want false")
	}
	if name := repo.Name(d); name != "" {
		t.Errorf("Name(2026-03-10) = %q, want empty", name)
	}
}

// TestEmbedHolidayRepository_Refresh 验证 Refresh 重放内嵌数据不报错。
func TestEmbedHolidayRepository_Refresh(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	if err := repo.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh = %v, want nil", err)
	}
	// 刷新后数据仍在。
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	if !repo.IsHoliday(d) || repo.Name(d) != "国庆" {
		t.Errorf("after Refresh: 2026-10-01 holiday=%v name=%q, want 国庆", repo.IsHoliday(d), repo.Name(d))
	}
}

// TestCalendarService_GetDayInfo_Holiday 验证聚合根经真实 HolidayRepository 组合出节假日信息。
func TestCalendarService_GetDayInfo_Holiday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	// 用 fake lunar（不关心农历字段），真实 holiday repo。
	svc := NewCalendarService(nil, &fakeLunar{}, repo, WithToday(time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)))
	info := svc.GetDayInfo(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	if !info.Holiday.IsHoliday || info.Holiday.Name != "元旦" {
		t.Errorf("GetDayInfo holiday = %+v, want IsHoliday + 元旦", info.Holiday)
	}
	if !info.IsToday {
		t.Error("GetDayInfo(2026-01-01) should be IsToday under WithToday(2026-01-01)")
	}
}
