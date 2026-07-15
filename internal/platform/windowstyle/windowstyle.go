// Package windowstyle 提供窗口样式配置原语（ADR-03 意图：无边框 + 每像素 alpha + 圆角 + DWM 阴影）。
//
// 当前状态（ADR-08 降级后）：本包是一个**独立的样式声明 / 常量模块**，描述 DeskCalendar 期望的
// 窗口视觉属性，但本身不直接操作窗口。真实的窗口由 internal/platform/win32 以自拥 Win32 弹窗
// （WS_POPUP + WS_EX_TOPMOST，DIBSection + WM_PAINT/BitBlt）承载，面板由 github.com/gogpu/gg
// （纯 Go CPU 光栅、零 CGO）即时绘制，再经 WindowController.Present 推送到屏幕。
//
// 因此在 ADR-08 下：
//   - 不存在 gogpu / wgpu 依赖，也不存在把本包映射到 gogpu.RenderMode 的适配器；
//   - 窗口"无边框 + 弹窗"由 win32 包直接落实；
//   - 自 v1.1（#147）起，圆角经 DWM DWMWA_WINDOW_CORNER_PREFERENCE 落地、阴影经窗口类
//     CS_DROPSHADOW 落地，均由 win32 包在窗口创建时应用——本包的 CornerRadius / Shadow 字段
//     仍是声明基线，与真实实现保持一致（见各字段注释）；
//   - 每像素 alpha / 分层窗（Layered / PerPixelAlpha）在 v1.1 仍未采用（与 ADR-08 一致），
//     WindowStyler 为运行时换肤钩子，MVP 未实现（win32 包直接应用样式）。
package windowstyle

// RenderMode 渲染模式（本地枚举）。
//
// 说明：早期设计稿曾设想映射到 gogpu.RenderMode（Auto/CPU/GPU）并经 shell 适配器衔接。
// ADR-08 降级后，窗口绘制固定走 gg 的纯 Go CPU 光栅路径（零 CGO、离线可用），不存在 GPU
// 渲染分支，因此本枚举在 ADR-08 下仅为**信息性预留**，当前不被 win32/gg 路径消费。
// 若未来引入可选 GPU 加速后端，再据此枚举选择渲染实现。
type RenderMode int

const (
	// RenderModeAuto 自动选择（ADR-08 下等价于 CPU：gg 纯 Go 光栅）。
	RenderModeAuto RenderMode = iota
	// RenderModeCPU 强制 CPU 光栅化（gg 当前唯一路径）。
	RenderModeCPU
	// RenderModeGPU 预留：未来可选 GPU 加速后端（当前未实现）。
	RenderModeGPU
)

// WindowStyle 描述窗口样式配置（ADR-03 意图）。
// 这些字段是期望的视觉属性声明；在 ADR-08 下，真实的窗口外观由 win32 包（WS_POPUP + WS_EX_TOPMOST
// 弹窗）+ gg 即时绘制落实，Frameless/Layered/CornerRadius/Shadow/RenderMode 等作为声明字段，
// 其中圆角 / 阴影已在 v1.1（#147）经 DWM / CS_DROPSHADOW 真正落地（见各字段注释）。
type WindowStyle struct {
	Frameless     bool       // 无边框（ADR-08 下 win32 弹窗即无边框）
	Layered       bool       // WS_EX_LAYERED 分层窗口（预留：ADR-08/v1.1 均走普通弹窗，非分层）
	PerPixelAlpha bool       // 每像素 alpha 透明（预留：v1.1 仍为不透明 gg 面板）
	CornerRadius  int        // DWM 圆角半径（像素）。v1.1(#147)：经 DwmSetWindowAttribute(DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND) 落地，固定系统圆角（半径值由 DWM 决定）
	Shadow        bool       // 外阴影。v1.1(#147)：经窗口类 CS_DROPSHADOW 落地（tool window 才显示，本弹窗满足）
	RenderMode    RenderMode // 渲染模式（预留：ADR-08 固定 gg CPU 光栅）
}

// DefaultWindowStyle 返回默认样式声明。
// 注：Layered/PerPixelAlpha/CornerRadius/Shadow 保留为声明默认值，与真实实现（win32 包 + gg
// 绘制 + #147 的 DWM 圆角 / CS_DROPSHADOW 阴影）保持一致，可作为后续运行时换肤的基线。
func DefaultWindowStyle() WindowStyle {
	return WindowStyle{
		Frameless:     true,
		Layered:       true,
		PerPixelAlpha: true,
		CornerRadius:  16,
		Shadow:        true,
		RenderMode:    RenderModeAuto,
	}
}

// WindowStyler 窗口样式应用者（预留钩子，MVP 未实现）。
// 未来由 shell/主题在窗口线程（win32 消息泵 goroutine）调用 Apply，
// 将样式变更应用到自拥 Win32 弹窗（而非旧设计的 gogpu 主窗口 / OnUpdate 帧循环）。
type WindowStyler interface {
	// Apply 将样式应用到窗口（应在窗口线程调用，非主 goroutine 命令循环）。
	Apply(style WindowStyle) error
	// Current 返回当前生效样式。
	Current() WindowStyle
}
