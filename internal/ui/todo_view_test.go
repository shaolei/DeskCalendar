package ui

import (
	"fmt"
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

// TestTodoVisibleRows 验证 S3 共享纯函数：给定内容高度能容纳的可见行数。
// Draw 与 HitTest 共用，公式漂移会导致「画了点不中」。
func TestTodoVisibleRows(t *testing.T) {
	// 360×480、Tab 36、无天气带 → 内容高 444；listArea=444-40-36=368；/38=9。
	if got := todoVisibleRows(444); got != 9 {
		t.Errorf("todoVisibleRows(444) = %d, want 9", got)
	}
	// 无 Tab/天气：内容高 480；listArea=480-40-36=404；/38=10。
	if got := todoVisibleRows(480); got != 10 {
		t.Errorf("todoVisibleRows(480) = %d, want 10", got)
	}
	// 极矮：内容高 < 76（draft+title）→ 0 行，不 panic。
	if got := todoVisibleRows(50); got != 0 {
		t.Errorf("todoVisibleRows(50) = %d, want 0", got)
	}
}

// TestRender_TodoViewOverflowDoesNotCorruptDraft 验证 S3：待办超出可见行时不再
// 画到草稿框上。渲染 20 条（可见 9 行），草稿框区域应保持 Surface 底色。
func TestRender_TodoViewOverflowDoesNotCorruptDraft(t *testing.T) {
	th := todoTestTheme(t)
	opts := RenderOptions{Width: 360, Height: 480, TabStripH: DefaultTabStripH, ViewMode: ViewTodo}
	todos := make([]*TodoItem, 20)
	for i := range todos {
		todos[i] = &TodoItem{ID: fmt.Sprintf("%d", i), Title: fmt.Sprintf("待办%d", i), Status: "active"}
	}
	m := Model{ViewMode: ViewTodo, Todos: todos, Draft: "草稿文字"}
	img := Render(m, opts, th)
	if img.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Fatalf("overflow render bounds = %v, want 360x480", img.Bounds())
	}
	// 整图不透明（flattenAlpha）。
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] != 255 {
				t.Fatalf("pixel (%d,%d) alpha != 255", x, y)
			}
		}
	}
	// 草稿框区域（面板 y≈460）应为 Surface（白），证明溢出待办未画到草稿框上、
	// 也不曾覆盖输入框（S3 核心回归点）。
	surf := th.Palette.Surface
	if got := sample(img, 100, 460); got.R != surf.R || got.G != surf.G || got.B != surf.B {
		t.Errorf("draft box pixel = %v, want Surface %v (overflow must not corrupt draft)", got, surf)
	}
}

// TestHitTest_TodoOverflowClamped 验证 S3：溢出行不进入命中（既不命中待办行也不
// 命中删除），可见行与草稿区行为不变。配置：20 条待办、可见 9 行（360×480, Tab 36）。
func TestHitTest_TodoOverflowClamped(t *testing.T) {
	opts := RenderOptions{
		Width: 360, Height: 480, TabStripH: DefaultTabStripH,
		ViewMode: ViewTodo, TodoCount: 20,
	}
	// 可见第 0 行仍命中（面板 y = contentTop36 + 标题36 + 19 = 91）。
	if r := HitTest(100, 91, opts); r.Kind != HitTodoRow || r.Row != 0 {
		t.Errorf("visible row0 = %v/%d, want HitTodoRow/0", r.Kind, r.Row)
	}
	// 第 9 行（首个溢出行，面板 y≈433，calY≈397 < 草稿阈值404）应为非交互——
	// 证明溢出行既不画也不命中（S3）。
	if r := HitTest(100, 433, opts); r.Kind != HitNone {
		t.Errorf("overflow row9 = %v, want HitNone", r.Kind)
	}
	// 草稿区（面板 y=460）仍是输入框，不被溢出行劫持。
	if r := HitTest(100, 460, opts); r.Kind != HitTodoDraft {
		t.Errorf("draft = %v, want HitTodoDraft", r.Kind)
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
