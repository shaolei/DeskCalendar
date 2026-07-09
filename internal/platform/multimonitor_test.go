package platform

import "testing"

// rectMonitor 是测试用的 Monitor 假实现。
type rectMonitor struct {
	bounds Rect
	dpi    int
}

func (m rectMonitor) Bounds() Rect { return m.bounds }
func (m rectMonitor) DPI() int     { return m.dpi }

func TestAnchorAboveTray_CenterAbove(t *testing.T) {
	mon := rectMonitor{bounds: Rect{X: 0, Y: 0, W: 1920, H: 1080}, dpi: 96}
	// 托盘在底部中间偏右；面板 320x240，margin 8。
	tray := Rect{X: 800, Y: 900, W: 32, H: 32}
	const panelW, panelH, margin = 320, 240, 8

	got := AnchorAboveTray(panelW, panelH, margin, tray, mon)

	wantX := tray.X + tray.W/2 - panelW/2 // 800+16-160 = 656
	wantY := tray.Y - panelH - margin      // 900-240-8 = 652
	if got.X != wantX || got.Y != wantY {
		t.Errorf("center-above: got (%d,%d) want (%d,%d)", got.X, got.Y, wantX, wantY)
	}
	if got.W != panelW || got.H != panelH {
		t.Errorf("size not preserved: got %dx%d want %dx%d", got.W, got.H, panelW, panelH)
	}
}

func TestAnchorAboveTray_FallbackBelowWhenInsufficientSpaceAbove(t *testing.T) {
	mon := rectMonitor{bounds: Rect{X: 0, Y: 0, W: 1920, H: 1080}, dpi: 96}
	// 任务栏在顶部：托盘 Y 很小，上方放不下 → 落到托盘下方。
	tray := Rect{X: 800, Y: 50, W: 32, H: 32}
	const panelW, panelH, margin = 320, 240, 8

	got := AnchorAboveTray(panelW, panelH, margin, tray, mon)

	wantX := tray.X + tray.W/2 - panelW/2 // 656
	wantY := tray.Y + tray.H + margin      // 50+32+8 = 90
	if got.X != wantX || got.Y != wantY {
		t.Errorf("fallback-below: got (%d,%d) want (%d,%d)", got.X, got.Y, wantX, wantY)
	}
}

func TestAnchorAboveTray_ClampXToScreen(t *testing.T) {
	mon := rectMonitor{bounds: Rect{X: 0, Y: 0, W: 1920, H: 1080}, dpi: 96}
	// 托盘贴近左边界，面板比托盘宽 → 左越界钳制到 0。
	tray := Rect{X: 10, Y: 900, W: 32, H: 32}
	const panelW, panelH, margin = 320, 240, 8

	got := AnchorAboveTray(panelW, panelH, margin, tray, mon)

	if got.X < 0 {
		t.Errorf("x should be clamped to >=0, got %d", got.X)
	}
	// 上方居中计算 x = 10+16-160 = -134 → 钳制到 0。
	if got.X != 0 {
		t.Errorf("x clamp: got %d want 0", got.X)
	}
}

func TestAnchorAboveTray_ClampYAfterBelowFallback(t *testing.T) {
	// 小显示器（高 300），托盘在顶部附近，面板较高（280）→ 下方回退后底部仍越界，钳制对齐屏底。
	mon := rectMonitor{bounds: Rect{X: 0, Y: 0, W: 400, H: 300}, dpi: 96}
	tray := Rect{X: 200, Y: 20, W: 32, H: 32}
	const panelW, panelH, margin = 320, 280, 8

	got := AnchorAboveTray(panelW, panelH, margin, tray, mon)

	// 上方不足 → 下方 y = 20+32+8 = 60；y+280 = 340 > 300 → 钳制 y = 300-280 = 20。
	if got.Y != 20 {
		t.Errorf("y clamp after below-fallback: got %d want 20", got.Y)
	}
	// 水平同样越界钳制：x = 200+16-160 = 56（在小屏 400 宽内，不越界）。
	if got.X != 56 {
		t.Errorf("x: got %d want 56", got.X)
	}
	if got.H != panelH {
		t.Errorf("height preserved: got %d want %d", got.H, panelH)
	}
}

func TestNewPanelAnchor_AnchorAboveTray(t *testing.T) {
	// 经 PanelAnchor 接口方法调用，覆盖 defaultPanelAnchor.AnchorAboveTray
	// （包级函数 AnchorAboveTray 经接口方法委托，二者需一致）。
	anchor := NewPanelAnchor()
	mon := rectMonitor{bounds: Rect{X: 0, Y: 0, W: 1920, H: 1080}, dpi: 96}
	tray := Rect{X: 800, Y: 900, W: 32, H: 32}
	const panelW, panelH, margin = 320, 240, 8

	got := anchor.AnchorAboveTray(panelW, panelH, margin, tray, mon)

	wantX := tray.X + tray.W/2 - panelW/2 // 656
	wantY := tray.Y - panelH - margin      // 652
	if got.X != wantX || got.Y != wantY {
		t.Errorf("via PanelAnchor: got (%d,%d) want (%d,%d)", got.X, got.Y, wantX, wantY)
	}
	if got.W != panelW || got.H != panelH {
		t.Errorf("size not preserved via PanelAnchor: got %dx%d want %dx%d", got.W, got.H, panelW, panelH)
	}
}
