package calendar

import (
	"context"
	"time"
)

// ---- 测试用 fake 实现（依赖倒置的契约验证 + 纯逻辑单测） ----

// fakeLunar 按日期返回预设的 LunarInfo；未命中返回零值。
type fakeLunar struct {
	byDate map[string]LunarInfo
}

func (f *fakeLunar) SolarToLunar(d time.Time) LunarInfo {
	if f.byDate != nil {
		if v, ok := f.byDate[d.Format("2006-01-02")]; ok {
			return v
		}
	}
	return LunarInfo{}
}

// fakeHoliday 按日期返回预设的节假日/补班信息。
type fakeHoliday struct {
	holidays map[string]bool
	workdays map[string]bool
	names    map[string]string
}

func (f *fakeHoliday) IsHoliday(d time.Time) bool {
	return f.holidays != nil && f.holidays[d.Format("2006-01-02")]
}
func (f *fakeHoliday) IsWorkday(d time.Time) bool {
	return f.workdays != nil && f.workdays[d.Format("2006-01-02")]
}
func (f *fakeHoliday) Name(d time.Time) string {
	if f.names == nil {
		return ""
	}
	return f.names[d.Format("2006-01-02")]
}
func (f *fakeHoliday) Refresh(ctx context.Context) error { return nil }
