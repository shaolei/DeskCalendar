package ui

import "testing"

// 布局常量（与 render.go / calendar_view.go 一致）：360×480、headerH=56、weekH=28、
// gridTop=84、colW≈51.43、rowH=66。导航按钮矩形由 computeNav(360) 推导：
//   prev  =(268,14,296,42)  next=(304,14,332,42)  today=(210,14,254,42)
func TestHitTest_NavButtons(t *testing.T) {
	opts := RenderOptions{Width: 360, Height: 480}
	cases := []struct {
		name string
		x, y int
		want HitKind
	}{
		{"prev-center", 282, 28, HitPrevMonth},
		{"next-center", 318, 28, HitNextMonth},
		{"today-center", 232, 28, HitToday},
		{"header-no-button", 10, 28, HitNone}, // 头部但非按钮区
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HitTest(c.x, c.y, opts).Kind; got != c.want {
				t.Errorf("HitTest(%d,%d).Kind = %v, want %v", c.x, c.y, got, c.want)
			}
		})
	}
}

func TestHitTest_GridCell(t *testing.T) {
	opts := RenderOptions{Width: 360, Height: 480}
	// row=2,col=3 → x≈180, y=84+2*66+33=249（gridTop=84,rowH=66）。
	res := HitTest(180, 249, opts)
	if res.Kind != HitCell {
		t.Fatalf("Kind = %v, want HitCell", res.Kind)
	}
	if res.Row != 2 || res.Col != 3 {
		t.Errorf("cell = (%d,%d), want (2,3)", res.Row, res.Col)
	}
}

func TestHitTest_OutOfGridIsNone(t *testing.T) {
	opts := RenderOptions{Width: 360, Height: 480}
	// x=400 超出面板宽：col=400/51.43≈7（≥7）→ 落空。
	if got := HitTest(400, 200, opts).Kind; got != HitNone {
		t.Errorf("Kind = %v, want HitNone", got)
	}
	// 面板外负坐标。
	if got := HitTest(-5, 100, opts).Kind; got != HitNone {
		t.Errorf("Kind = %v, want HitNone", got)
	}
}

func TestHitTest_DefaultOpts(t *testing.T) {
	// 零值 opts 回退 360×480，命中结果与显式一致。
	if got := HitTest(282, 28, RenderOptions{}).Kind; got != HitPrevMonth {
		t.Errorf("default-opts Kind = %v, want HitPrevMonth", got)
	}
}
