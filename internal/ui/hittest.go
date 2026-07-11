package ui

import "image"

// HitKind 命中区域类别（#113 点击命中测试）。
type HitKind int

const (
	// HitNone 命中空白/非交互区（头部非按钮处、面板外溢出等）。
	HitNone HitKind = iota
	// HitCell 命中 6×7 网格中的某格；Row/Col 有效。
	HitCell
	// HitPrevMonth 命中「上一月」按钮。
	HitPrevMonth
	// HitNextMonth 命中「下一月」按钮。
	HitNextMonth
	// HitToday 命中「今天」按钮。
	HitToday
)

// HitResult 命中测试结果。Kind==HitCell 时 Row/Col 为网格行列索引（0-based）。
type HitResult struct {
	Kind HitKind
	Row  int
	Col  int
}

// 导航按钮布局常量（逻辑坐标，96 DPI 基准）。与 CalendarView.Draw 共用同一来源，
// 绘制与命中测试绝不能各写一份，否则按钮位置漂移导致点不中。
const (
	navBtnSize = 28 // 上一月/下一月按钮边长
	navBtnY    = 14 // 按钮顶部 y（头部区内）
	todayBtnW  = 44 // 「今天」按钮宽度
)

// computeNav 由面板逻辑宽推导三个头部导航按钮的矩形（纯函数）。
// 布局：月份标题居左（x0+16）；上一月/下一月靠右，今天按钮夹在其左。
// 返回矩形均以面板左上角为原点（与 Draw 的 x0/y0 对齐，Draw 内会再叠加偏移）。
func computeNav(w int) (prev, next, today image.Rectangle) {
	prev = image.Rect(w-92, navBtnY, w-92+navBtnSize, navBtnY+navBtnSize)
	next = image.Rect(w-56, navBtnY, w-56+navBtnSize, navBtnY+navBtnSize)
	today = image.Rect(w-150, navBtnY, w-150+todayBtnW, navBtnY+navBtnSize)
	return
}

// inRect 判断点 (x,y) 是否落在矩形 r 内（左闭右开，含 Min 不含 Max）。
func inRect(r image.Rectangle, x, y int) bool {
	return x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y
}

// HitTest 将面板逻辑坐标 (x,y) 映射到命中区域（纯函数，易单测，#113）。
// 调用方（app 主循环）应传入与 Render 相同的 RenderOptions（同宽高/96-DPI 基准）。
// 返回 HitNone 表示未命中任何交互元素。
func HitTest(x, y int, opts RenderOptions) HitResult {
	w, h := opts.Width, opts.Height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	lay := computeLayout(w, h)
	res := HitResult{} // 默认 Kind=HitNone

	// 1) 头部区（y ≤ headerH）：优先判三个导航按钮。
	if y >= 0 && float64(y) <= lay.headerH {
		prev, next, today := computeNav(w)
		if inRect(prev, x, y) {
			return HitResult{Kind: HitPrevMonth}
		}
		if inRect(next, x, y) {
			return HitResult{Kind: HitNextMonth}
		}
		if inRect(today, x, y) {
			return HitResult{Kind: HitToday}
		}
		return res // 头部其它位置（如月份标题）非交互
	}

	// 2) 网格区（y ≥ gridTop）：按列宽/行高反算行列。
	if x >= 0 && float64(y) >= lay.gridTop {
		col := int(float64(x) / lay.colW)
		row := int((float64(y) - lay.gridTop) / lay.rowH)
		if col >= 0 && col < 7 && row >= 0 && row < 6 {
			return HitResult{Kind: HitCell, Row: row, Col: col}
		}
	}
	return res
}
