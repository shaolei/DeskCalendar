package calendar

import "time"

// Cell 月网格单格。
type Cell struct {
	Date           time.Time
	InCurrentMonth bool      // 是否属于当前月（补位格=false）
	IsToday        bool
	IsSelected     bool
	Lunar          LunarInfo   // 来自 LunarService（未注入则为空值）
	Holiday        HolidayInfo // 来自 HolidayRepository（未注入则为空值）
}

// MonthGrid 月视图网格（固定 6 行 7 列 = 42 格）。
type MonthGrid struct {
	Year      int
	Month     time.Month
	Weeks     [6][7]Cell
	FirstCell time.Time // 左上角格日期（可能属上月）
	LastCell  time.Time // 右下角格日期（可能属下月）
}

// GridOptions 网格生成选项。
type GridOptions struct {
	Today     time.Time        // 用于判定 IsToday，默认 time.Now()
	Selected  time.Time        // 用于判定 IsSelected
	Lunar     LunarService     // 注入农历服务（可 nil）
	Holiday   HolidayRepository // 注入节假日仓储（可 nil）
	WeekStart time.Weekday      // 周起始（time.Sunday / time.Monday）
}

// GenMonthGrid 生成指定年月的月网格（纯函数，可单测）。
// 规则：以 WeekStart 为列首，向上取满首格、向下补满 6 行；
// 非当月日期 InCurrentMonth=false；每格填充 Lunar/Holiday 与高亮标志。
func GenMonthGrid(year int, month time.Month, opts GridOptions) MonthGrid {
	today := opts.Today
	if today.IsZero() {
		today = time.Now()
	}
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	start := weekStart(first, opts.WeekStart)

	grid := MonthGrid{Year: year, Month: month}
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			d := start.AddDate(0, 0, r*7+c)
			cell := Cell{
				Date:           d,
				InCurrentMonth: d.Year() == year && d.Month() == month,
				IsToday:        isSameDay(d, today),
				IsSelected:     isSameDay(d, opts.Selected),
			}
			if opts.Lunar != nil {
				cell.Lunar = opts.Lunar.SolarToLunar(d)
			}
			cell.Holiday = dayInfo(opts.Holiday, d)
			grid.Weeks[r][c] = cell
		}
	}
	grid.FirstCell = grid.Weeks[0][0].Date
	grid.LastCell = grid.Weeks[5][6].Date
	return grid
}

// isSameDay 判断两个时间是否同一公历日（忽略时分秒与时区偏移差异按本地日历比对）。
func isSameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && int(a.Month()) == int(b.Month()) && a.Day() == b.Day()
}

// weekStart 返回 date 所在周的周首（按 ws 指定的星期几为列首），时间归零到当天午夜。
func weekStart(date time.Time, ws time.Weekday) time.Time {
	offset := (int(date.Weekday()) - int(ws) + 7) % 7
	d := date.AddDate(0, 0, -offset)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}
