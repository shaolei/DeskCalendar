package ui

import (
	"image"
	"strconv"

	"github.com/gogpu/gg"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// CalendarView 绘制月历面板内容：月份标题 + 星期表头 + 6×7 网格。
// 是 MVP 唯一子视图（Settings 走托盘菜单，非视图）。
type CalendarView struct{}

// OnShow MVP 无操作（日历数据由调用方每次重渲时重建）。
func (CalendarView) OnShow() {}

// OnHide MVP 无操作。
func (CalendarView) OnHide() {}

// layout 面板内容区布局尺寸（纯函数，便于单测与多尺寸复用）。
type layout struct {
	headerH, weekH, colW, gridTop, gridH, rowH float64
}

// computeLayout 由面板逻辑尺寸推导各区块坐标/尺寸（周一首列时列宽均分）。
func computeLayout(w, h int) layout {
	headerH := 56.0
	weekH := 28.0
	colW := float64(w) / 7.0
	gridTop := headerH + weekH
	gridH := float64(h) - gridTop
	return layout{
		headerH: headerH,
		weekH:   weekH,
		colW:    colW,
		gridTop: gridTop,
		gridH:   gridH,
		rowH:    gridH / 6.0,
	}
}

// Draw 在 rect 内绘制整月历。rect 已由 Render 填好不透明背景。
func (CalendarView) Draw(dc *gg.Context, rect image.Rectangle, m Model, th *theme.Theme) {
	w, h := rect.Dx(), rect.Dy()
	lay := computeLayout(w, h)
	x0, y0 := float64(rect.Min.X), float64(rect.Min.Y)

	// 1) 月份标题（左上，大号）
	applyFont(dc, 22)
	dc.SetColor(opaque(th.Palette.Foreground))
	dc.DrawStringAnchored(m.MonthLabel, x0+16, y0+lay.headerH/2, 0, 0.5)

	// 2) 星期表头（居中，静色）
	applyFont(dc, 13)
	dc.SetColor(opaque(th.Palette.Muted))
	for i, label := range m.Weekdays {
		cx := x0 + lay.colW*(float64(i)+0.5)
		dc.DrawStringAnchored(label, cx, y0+lay.headerH+lay.weekH/2, 0.5, 0.5)
	}

	// 3) 表头分隔线
	dc.SetColor(opaque(th.Palette.Border))
	dc.SetLineWidth(1)
	dc.DrawLine(x0, y0+lay.headerH+lay.weekH, x0+float64(w), y0+lay.headerH+lay.weekH)
	_ = dc.Stroke()

	// 4) 6×7 网格
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			cell := m.Weeks[r][c]
			cx0 := x0 + lay.colW*float64(c)
			cy0 := y0 + lay.gridTop + lay.rowH*float64(r)

			// 今日：浅色底填充（半透明叠绘，flatten 后整图不透明）
			if cell.IsToday {
				dc.SetColor(withAlpha(th.Palette.TodayBlue, 40))
				dc.DrawRectangle(cx0, cy0, lay.colW, lay.rowH)
				_ = dc.Fill()
			}

			// 公历日数字（左上）
			dayColor := th.Palette.Foreground
			switch {
			case !cell.InMonth:
				dayColor = th.Palette.Muted
			case cell.IsHoliday:
				dayColor = th.Palette.HolidayRed
			case cell.IsSelected:
				dayColor = th.Palette.Accent
			}
			applyFont(dc, 16)
			dc.SetColor(opaque(dayColor))
			dc.DrawStringAnchored(strconv.Itoa(cell.Day), cx0+10, cy0+14, 0, 0)

			// 农历/节假日小字（底部居中）
			sub := cell.Lunar
			if cell.Holiday != "" {
				sub = cell.Holiday
			}
			if sub != "" {
				applyFont(dc, 11)
				subColor := th.Palette.Muted
				if cell.IsHoliday {
					subColor = th.Palette.HolidayRed
				}
				dc.SetColor(opaque(subColor))
				dc.DrawStringAnchored(sub, cx0+lay.colW/2, cy0+lay.rowH-10, 0.5, 0.5)
			}
		}
	}
}
