package calendar

import "time"

// WeekGrid 周视图网格（固定 7 格）。
type WeekGrid struct {
	Anchor time.Time // 锚定日（决定所属周）
	Start  time.Time // 周首（按 WeekStart）
	End    time.Time // 周尾（Start + 6 天）
	Days   [7]Cell   // 复用 Month 的 Cell
}

// GenWeekGrid 以 anchor 所在周生成周网格（纯函数，可单测）。
// 规则：以 opts.WeekStart 定位周首，填充 7 格；每格复用标注规则。
// 周视图不强调跨月，InCurrentMonth 统一置 true。
func GenWeekGrid(anchor time.Time, opts GridOptions) WeekGrid {
	today := opts.Today
	if today.IsZero() {
		today = time.Now()
	}
	start := weekStart(anchor, opts.WeekStart)
	wg := WeekGrid{
		Anchor: anchor,
		Start:  start,
		End:    start.AddDate(0, 0, 6),
	}
	for i := 0; i < 7; i++ {
		d := start.AddDate(0, 0, i)
		cell := Cell{
			Date:           d,
			InCurrentMonth: true,
			IsToday:        isSameDay(d, today),
			IsSelected:     isSameDay(d, opts.Selected),
		}
		if opts.Lunar != nil {
			cell.Lunar = opts.Lunar.SolarToLunar(d)
		}
		cell.Holiday = dayInfo(opts.Holiday, d)
		wg.Days[i] = cell
	}
	return wg
}
