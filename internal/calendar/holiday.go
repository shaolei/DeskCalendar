package calendar

import (
	"context"
	"time"
)

// HolidayInfo 节假日/调休信息值对象。
type HolidayInfo struct {
	IsHoliday bool
	IsWorkday bool   // 调休补班日
	Name      string // 节日/放假名，如 "元旦"；补班可为 "元旦补班"
}

// HolidayRepository 节假日仓储接口（依赖倒置，可 mock）。
// 真实实现见 holiday_embed.go（go:embed 烘焙的 holiday-cn JSON）。
type HolidayRepository interface {
	// IsHoliday 法定节假日（非补班）。
	IsHoliday(date time.Time) bool
	// IsWorkday 调休补班日（周末/休息日上班）。
	IsWorkday(date time.Time) bool
	// Name 节假日名称（含补班标注）。
	Name(date time.Time) string
	// Refresh 可选运行时年度刷新；失败返回 error（调用方应回退 embed）。
	Refresh(ctx context.Context) error
}

// dayInfo 由仓储接口组合出 HolidayInfo（包内辅助，非接口成员）。
// Month/Week 网格与 CalendarService.GetDayInfo 直接调用，避免把 dayInfo 塞进接口。
func dayInfo(r HolidayRepository, d time.Time) HolidayInfo {
	if r == nil {
		return HolidayInfo{}
	}
	return HolidayInfo{
		IsHoliday: r.IsHoliday(d),
		IsWorkday: r.IsWorkday(d),
		Name:      r.Name(d),
	}
}
