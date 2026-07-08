// DeskCalendar POC — 透明圆角验证
//
// 目的：验证 gogpu/ui 在打过 patch 的 gogpu 之上，能否输出「每像素 alpha」，
// 从而让圆角面板之外的区域透出桌面 —— 这正是 360 小清新日历的核心观感。
//
// 依赖的 gogpu patch（已应用到 D:\workspace\github\gogpu，见 README「Patch 清单」）：
//  1. renderer.go: CompositeAlphaModeOpaque -> CompositeAlphaModePremultiplied
//  2. platform_windows.go: CreateWindowExW 的 dwExStyle 加
//     WS_EX_LAYERED | WS_EX_NOREDIRECTIONBITMAP
//
// 判读标准（在 Windows 上运行后观察）：
//
//	✅ 成功：看到一个「蓝色圆角面板」悬浮在桌面之上，
//	         面板四角之外的区域能透出后面的桌面/窗口。
//	❌ 失败（黑/白实心矩形）：上述 patch 未生效或需微调（见 README「排错」）。
//
// 注意：本程序使用 GPU + CGO（wgpu FFI），只能在 Windows + 带 C 工具链的环境编译运行，
//
//	当前沙箱（无显示/无 GPU）无法执行，需在你的 Windows 机器上跑。
package main

import (
	"log"

	_ "github.com/gogpu/gg/gpu" // 启用 GPU SDF 加速（参照 gogpu/ui hello 示例）

	"github.com/gogpu/gogpu"
	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/desktop"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

// panelColor 近似 360 小清新截图的蓝。
const panelColor = 0x3B6FE0

func main() {
	m3 := material3.New(widget.Hex(panelColor))

	// 关键：把根背景设为透明。
	// gogpu/ui 默认会用 ThemeBackground 填充满整窗（desktop.go 的 flushBoundaryToTexture），
	// 且它尊重 alpha —— 所以透明背景 => 圆角面板之外透出桌面。
	// 注意：必须先 AsTheme() 拿到即将传入 App 的 *theme.Theme，再改它的 Colors.Background，
	//       避免 AsTheme 内部拷贝导致修改不生效。
	th := m3.AsTheme()
	th.Colors.Background = widget.RGBA8(0, 0, 0, 0)

	gogpuApp := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("DeskCalendar POC — 透明圆角验证").
		WithSize(360, 460).
		WithFrameless(true), // 无边框；配合 patched gogpu 的 WS_EX_LAYERED 透明窗体
	)

	uiApp := app.New(
		app.WithWindowProvider(gogpuApp),
		app.WithPlatformProvider(gogpuApp),
		app.WithEventSource(gogpuApp.EventSource()),
		app.WithTheme(th),
	)
	uiApp.SetRoot(buildUI())

	if err := desktop.Run(gogpuApp, uiApp); err != nil {
		log.Fatal(err)
	}
}

func buildUI() *primitives.BoxWidget {
	// 内层圆角面板：不透明蓝底 + 文字。
	panel := primitives.Box(
		primitives.Text("DeskCalendar").
			FontSize(22).
			Bold().
			Color(widget.RGBA8(255, 255, 255, 255)),
		primitives.Text("透明圆角 POC 验证").
			FontSize(14).
			Color(widget.RGBA8(255, 255, 255, 235)),
	).
		Padding(28).
		Gap(12).
		Background(widget.RGBA8(59, 111, 224, 255)). // #3B6FE0
		Rounded(16).
		ShadowLevel(3)

	// 根 Box 不设背景（透明）。整窗只有 panel 不透明。
	return primitives.Box(panel).Padding(0)
}
