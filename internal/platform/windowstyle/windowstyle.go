// Package windowstyle 提供窗口样式原语（ADR-03：无边框 + 每像素 alpha + 圆角 + DWM 阴影）。
//
// Phase 0 范围：定义 WindowStyle 配置结构与 WindowStyler 接口，并给出 MVP 默认样式。
// 具体把样式落到窗口（WS_EX_LAYERED / 每像素 alpha / DWM 圆角阴影）的
// WindowStyler 实现推迟到 Phase 3（shell 装配时）；届时由 shell 提供
// RenderMode → gogpu.RenderMode 的自由函数适配器（方法必须定义在本包，故用函数）。
package windowstyle

// RenderMode 渲染模式（与本地 gogpu fork 的 gogpu.RenderMode 对齐：Auto/CPU/GPU）。
//
// 说明：早期设计稿曾设想 gogpu 提供"宿主托管式"渲染模式常量，但本地 gogpu fork
// （D:/workspace/github/gogpu，config.go）实际只导出 RenderModeAuto / RenderModeCPU /
// RenderModeGPU。为避免把重量级 gogpu/wgpu 栈引入基础层、并保证离线零 CGO 构建，本包用
// 本地枚举表达渲染模式，待 shell 阶段再经适配器映射到 gogpu.RenderMode。
type RenderMode int

const (
	// RenderModeAuto 自动选择：软件适配器走 CPU 光栅化，真实 GPU 走 GPU 路径。
	RenderModeAuto RenderMode = iota
	// RenderModeCPU 强制 CPU 光栅化（调试/无 GPU 环境）。
	RenderModeCPU
	// RenderModeGPU 强制 GPU 路径（着色器测试）。
	RenderModeGPU
)

// WindowStyle 描述窗口样式配置（ADR-03）。
// 所有字段映射到 gogpu 的 Frameless/Layered/alpha/圆角/阴影设置。
type WindowStyle struct {
	Frameless     bool       // 无边框
	Layered       bool       // WS_EX_LAYERED 分层窗口
	PerPixelAlpha bool       // 每像素 alpha 透明
	CornerRadius  int        // DWM 圆角半径（像素），0=系统默认
	Shadow        bool       // DWM 外阴影
	RenderMode    RenderMode // 渲染模式
}

// DefaultWindowStyle 返回 MVP 默认样式：无边框+分层+每像素alpha+圆角16+阴影。
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

// WindowStyler 窗口样式应用者。实现方封装 gogpu 装配细节（Phase 3）。
type WindowStyler interface {
	// Apply 在主线程将样式应用到主窗口（仅 OnUpdate 调用）。
	Apply(style WindowStyle) error
	// Current 返回当前生效样式。
	Current() WindowStyle
}
