package calendar

import (
	"context"
	"testing"
	"time"
)

// TestHolidayRepository_Contract 验证 fake 满足 HolidayRepository 接口。
func TestHolidayRepository_Contract(t *testing.T) {
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	var repo HolidayRepository = &fakeHoliday{
		holidays: map[string]bool{date.Format("2006-01-02"): true},
		names:    map[string]string{date.Format("2006-01-02"): "元旦"},
	}
	if !repo.IsHoliday(date) {
		t.Error("IsHoliday(元旦) = false, want true")
	}
	if repo.Name(date) != "元旦" {
		t.Errorf("Name(元旦) = %q, want 元旦", repo.Name(date))
	}
	if err := repo.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh = %v, want nil", err)
	}
}

// TestDayInfo_BuildsFromRepo 验证 dayInfo 由接口组合出 HolidayInfo。
func TestDayInfo_BuildsFromRepo(t *testing.T) {
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	repo := &fakeHoliday{
		holidays: map[string]bool{date.Format("2006-01-02"): true},
		names:    map[string]string{date.Format("2006-01-02"): "元旦"},
	}
	info := dayInfo(repo, date)
	if !info.IsHoliday || info.Name != "元旦" {
		t.Errorf("dayInfo = %+v, want IsHoliday + Name=元旦", info)
	}
}

// TestDayInfo_NilRepo 验证 nil 仓储返回零值（不恐慌）。
func TestDayInfo_NilRepo(t *testing.T) {
	info := dayInfo(nil, time.Now())
	if info != (HolidayInfo{}) {
		t.Errorf("dayInfo(nil) = %+v, want zero HolidayInfo", info)
	}
}
