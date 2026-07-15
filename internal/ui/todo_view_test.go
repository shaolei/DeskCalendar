package ui

import (
	"image"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/theme"
)

func todoTestTheme(t *testing.T) *theme.Theme {
	t.Helper()
	tp, err := theme.NewProvider(theme.WithInitialScheme(theme.SchemeLight))
	if err != nil {
		t.Fatalf("theme provider: %v", err)
	}
	return tp.Current()
}

func TestHitTest_TabStrip(t *testing.T) {
	th := todoTestTheme(t)
	opts := RenderOptions{Width: 360, Height: 480, TabStripH: DefaultTabStripH}
	// 天气带=0，Tab 条在 y∈[0,36)。左半=日历，右半=待办。
	if r := HitTest(10, 18, opts); r.Kind != HitTabCalendar {
		t.Errorf("left half = %v, want HitTabCalendar", r.Kind)
	}
	if r := HitTest(300, 18, opts); r.Kind != HitTabTodo {
		t.Errorf("right half = %v, want HitTabTodo", r.Kind)
	}
	// 渲染 Tab 条不应 panic，且整图不透明。
	m := Model{ViewMode: ViewCalendar, Todos: nil}
	img := Render(m, opts, th)
	if img.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Errorf("tab render bounds = %v, want 360x480", img.Bounds())
	}
}

func TestHitTest_TodoRows(t *testing.T) {
	opts := RenderOptions{
		Width: 360, Height: 480, TabStripH: DefaultTabStripH,
		ViewMode: ViewTodo, TodoCount: 3,
	}
	// 内容区起点 = bandH(0)+tabH(36) = 36。待办标题区 todoTitleH=36 → [36,72) 非交互。
	if r := HitTest(100, 50, opts); r.Kind != HitNone {
		t.Errorf("title area = %v, want HitNone", r.Kind)
	}
	// 第 0 行：listTop=72，rowTop=72，行内 y∈[72,110)。x 在左侧 → HitTodoRow row0。
	if r := HitTest(100, 90, opts); r.Kind != HitTodoRow || r.Row != 0 {
		t.Errorf("row0 = %v row=%d, want HitTodoRow/0", r.Kind, r.Row)
	}
	// 第 2 行（rowTop=72+2*38=148）：x 在右侧删除区(w-40=320) → HitTodoDelete row2。
	if r := HitTest(340, 167, opts); r.Kind != HitTodoDelete || r.Row != 2 {
		t.Errorf("row2 delete = %v row=%d, want HitTodoDelete/2", r.Kind, r.Row)
	}
	// 底部输入框：面板 y ∈ [440,480) 为草稿区（contentTop=36, 内容高 444, 草稿 40）。
	if r := HitTest(100, 460, opts); r.Kind != HitTodoDraft {
		t.Errorf("draft = %v, want HitTodoDraft", r.Kind)
	}
	// 超出实际条数（第 5 行，但 TodoCount=3）→ 非交互。
	if r := HitTest(100, 300, opts); r.Kind != HitNone {
		t.Errorf("beyond count = %v, want HitNone", r.Kind)
	}
}

func TestRender_TodoView(t *testing.T) {
	th := todoTestTheme(t)
	now := time.Now()
	due := now.Add(time.Hour)
	opts := RenderOptions{Width: 360, Height: 480, TabStripH: DefaultTabStripH, ViewMode: ViewTodo}
	m := Model{
		ViewMode: ViewTodo,
		Todos: []*TodoItem{
			{ID: "1", Title: "买菜", Status: "active", Due: &due, Overdue: false},
			{ID: "2", Title: "已完成项", Status: "done"},
		},
		Draft: "草稿文字",
	}
	img := Render(m, opts, th)
	if img.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Fatalf("todo render bounds = %v, want 360x480", img.Bounds())
	}
	// 整图不透明（flattenAlpha）。
	for y := 0; y < 480; y++ {
		for x := 0; x < 360; x++ {
			if img.Pix[img.PixOffset(x, y)+3] != 255 {
				t.Fatalf("pixel (%d,%d) alpha != 255", x, y)
			}
		}
	}
}

// TestHitTest_BackwardCompat 回归：TabStripH=0 时布局与旧版完全一致——日历视图下
// 原本命中网格/导航的位置依旧命中，Tab 分支不介入。
func TestHitTest_BackwardCompat(t *testing.T) {
	opts := RenderOptions{Width: 360, Height: 480} // 无天气带、无 Tab 条
	// 上一月按钮：w=360 → prev 矩形 (268,14)-(296,42)，中点 282,28。
	if r := HitTest(282, 28, opts); r.Kind != HitPrevMonth {
		t.Errorf("prev month = %v, want HitPrevMonth (backward compat)", r.Kind)
	}
	// 网格某格（row=2,col=3 → 约 180,249）。
	if r := HitTest(180, 249, opts); r.Kind != HitCell {
		t.Errorf("cell = %v, want HitCell (backward compat)", r.Kind)
	}
}
