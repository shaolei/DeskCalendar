// Package platform 提供 DeskCalendar 的平台原语（DPI / 多屏定位 / 托盘 / 自启 / 通知）。
//
// 设计原则（路径 D · 零依赖自有平台代码）：
//   - 纯逻辑（坐标换算、锚定）为纯函数，易单测、可跨平台编译；
//   - 与 OS 交互的部分（注册表 / 显示器枚举 / DPI 感知 / 托盘）通过 seam 接口注入，
//     真实实现走 golang.org/x/sys/windows（零 CGO），测试用 fake 验证逻辑；
//   - 托盘（Tray）封装 gogpu/systray（纯 Go·零 CGO，本地 replace 引入）。
package platform

// Rect 屏幕坐标矩形（物理像素）。
type Rect struct {
	X, Y, W, H int
}

// Monitor 抽象单台显示器（实现封装零 CGO 的 EnumDisplayMonitors 等）。
type Monitor interface {
	// Bounds 返回该显示器工作区（不含任务栏）矩形。
	Bounds() Rect
	// DPI 返回该显示器有效 DPI。
	DPI() int
}

// PanelAnchor 面板锚定策略。
type PanelAnchor interface {
	// AnchorAboveTray 将面板锚定到托盘图标正上方居中，并做边界钳制。
	// 详见包级函数 AnchorAboveTray。
	AnchorAboveTray(panelW, panelH, margin int, tray Rect, mon Monitor) Rect
}

// defaultPanelAnchor 默认锚定实现。
type defaultPanelAnchor struct{}

// NewPanelAnchor 构造默认锚定器。
func NewPanelAnchor() PanelAnchor { return &defaultPanelAnchor{} }

// AnchorAboveTray 默认实现（委托包级函数）。
func (a *defaultPanelAnchor) AnchorAboveTray(panelW, panelH, margin int, tray Rect, mon Monitor) Rect {
	return AnchorAboveTray(panelW, panelH, margin, tray, mon)
}

// AnchorAboveTray 将面板锚定到托盘图标正上方居中。
//
// 公式：x = tray.X + tray.W/2 - panelW/2；y = tray.Y - panelH - margin。
// 边界处理：
//   - 上方空间不足(y < mon.Bounds().Y) → 落到托盘下方 y = tray.Y + tray.H + margin
//   - 水平超出屏宽 → 钳制到 [mon.X, mon.X+mon.W-panelW]
//   - 面板底部超出屏高 → 钳制 y 使底部对齐屏底
//
// panelW/panelH 为物理像素（已据 mon.DPI() 由 DPIScaler 换算）。
func AnchorAboveTray(panelW, panelH, margin int, tray Rect, mon Monitor) Rect {
	b := mon.Bounds()
	x := tray.X + tray.W/2 - panelW/2
	y := tray.Y - panelH - margin
	if y < b.Y { // 上方不足 → 落到托盘下方
		y = tray.Y + tray.H + margin
	}
	if x < b.X {
		x = b.X
	}
	if x+panelW > b.X+b.W {
		x = b.X + b.W - panelW
	}
	if y+panelH > b.Y+b.H { // 面板底部超出屏高 → 钳制对齐屏底
		y = b.Y + b.H - panelH
	}
	return Rect{X: x, Y: y, W: panelW, H: panelH}
}
