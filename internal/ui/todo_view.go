package ui

import (
	"fmt"
	"image"

	"github.com/gogpu/gg"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// TodoView 面板待办列表视图（v1.1 #148）。
//
// 直接从持有的 Model.Todos 读取绘制，不依赖 internal/todo（app 在重渲时把
// todo.Todo 映射为 ui.TodoItem，保持 ui 不反向依赖领域包，ADR-07a）。列表每行
// [✓]/[○] 勾选 + 标题 + 截止时间（延期红色）+ 右侧 ✕ 删除；底部为待办输入框。
// 交互命中由 HitTest 负责（HitTodoRow/HitTodoDelete/HitTodoDraft），本视图只画。
type TodoView struct{}

// OnShow MVP 无操作（待办数据由 app 主循环每次重渲时重建）。
func (TodoView) OnShow() {}

// OnHide MVP 无操作。
func (TodoView) OnHide() {}

// todo 视图布局常量（逻辑像素，96 DPI 基准）。与 HitTest 共用同一来源，绘制与
// 命中测试绝不能各写一份，否则行高漂移导致点不中（与 CalendarView/computeLayout 同理）。
const (
	todoTitleH = 36.0 // 标题区高度（"待办"+计数）
	todoRowH   = 38.0 // 单行高度
	todoDraftH = 40.0 // 底部输入框高度
)

// Draw 在 rect（待办内容区，rect 已由 Render 填好不透明背景）内绘制待办列表。
func (TodoView) Draw(dc *gg.Context, rect image.Rectangle, m Model, th *theme.Theme) {
	x0, y0 := float64(rect.Min.X), float64(rect.Min.Y)
	w := float64(rect.Dx())
	h := float64(rect.Dy())

	// 标题行：「待办」+ 未完成计数。
	applyFont(dc, 18)
	dc.SetColor(opaque(th.Palette.Foreground))
	dc.DrawStringAnchored("待办", x0+16, y0+todoTitleH/2, 0, 0.5)
	activeCount := 0
	for _, it := range m.Todos {
		if it.Status == "active" {
			activeCount++
		}
	}
	if activeCount > 0 {
		applyFont(dc, 12)
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored(fmt.Sprintf("%d 项待完成", activeCount), x0+w-16, y0+todoTitleH/2, 1, 0.5)
	}

	// 列表区（可滚动概念上以首屏为主，超出不画——MVP 不实现滚动）。
	listTop := y0 + todoTitleH
	baseY := listTop
	for i, it := range m.Todos {
		rowTop := baseY + float64(i)*todoRowH
		// 行底部分隔线（轻）。
		dc.SetColor(opaque(th.Palette.Border))
		dc.SetLineWidth(1)
		dc.DrawLine(x0, rowTop+todoRowH-0.5, x0+w, rowTop+todoRowH-0.5)
		_ = dc.Stroke()

		done := it.Status == "done"
		main := th.Palette.Foreground
		if done {
			main = th.Palette.Muted
		}
		// 勾选框：[✓] 已完成 / [○] 进行中。
		applyFont(dc, 16)
		dc.SetColor(opaque(main))
		check := "○"
		if done {
			check = "✓"
		}
		dc.DrawStringAnchored(check, x0+18, rowTop+todoRowH/2, 0.5, 0.5)

		// 标题（进行中正常色；已完成弱化）。
		applyFont(dc, 14)
		dc.SetColor(opaque(main))
		dc.DrawStringAnchored(it.Title, x0+40, rowTop+todoRowH/2, 0, 0.5)

		// 截止时间（右侧）：延期红色，否则静色。
		if it.Due != nil {
			dcol := th.Palette.Muted
			if it.Overdue {
				dcol = th.Palette.HolidayRed
			}
			applyFont(dc, 11)
			dc.SetColor(opaque(dcol))
			dueStr := it.Due.Format("15:04")
			dc.DrawStringAnchored(dueStr, x0+w-52, rowTop+todoRowH/2, 0, 0.5)
		}

		// 删除按钮（最右侧 ✕）。
		applyFont(dc, 14)
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored("✕", x0+w-16, rowTop+todoRowH/2, 0.5, 0.5)
	}

	// 空状态提示（无待办且无草稿）。
	if len(m.Todos) == 0 {
		applyFont(dc, 13)
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored("暂无待办，在下方输入框添加", x0+w/2, baseY+40, 0.5, 0.5)
	}

	// 底部输入框（草稿）。
	draftTop := y0 + h - todoDraftH
	dc.SetColor(opaque(th.Palette.Surface))
	dc.DrawRectangle(x0, draftTop, w, todoDraftH)
	_ = dc.Fill()
	dc.SetColor(opaque(th.Palette.Border))
	dc.SetLineWidth(1)
	dc.DrawRectangle(x0+0.5, draftTop+0.5, w-1, todoDraftH-1)
	_ = dc.Stroke()
	applyFont(dc, 14)
	if m.Draft != "" {
		dc.SetColor(opaque(th.Palette.Foreground))
		dc.DrawStringAnchored(m.Draft, x0+10, draftTop+todoDraftH/2, 0, 0.5)
	} else {
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored("输入待办，回车添加…", x0+10, draftTop+todoDraftH/2, 0, 0.5)
	}
}
