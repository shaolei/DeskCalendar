// Package settings 构建托盘右键菜单（声明式 MenuItem 树），将菜单交互
// 接到 config 持久化与副作用（注册表自启 / 主题应用）。
//
// 路径 D / ADR-08：原 SettingsView 独立窗（gogpu/ui 控件树）降级为 v1.3 后备
// （见 #119）；MVP 设置走托盘右键菜单，经 gogpu/systray 的 AddCheckbox 渲染
// （由 internal/platform 负责）。本包只产出 platform.TrayMenu 数据模型，
// 不依赖 systray / UI 框架，因而可纯单测。
package settings

import (
	"context"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
)

// Deps 是构建托盘菜单所需的依赖（由 app 在装配期注入）。
//
// 单写者约束（代码审查 S1）：本包只「产出命令」，绝不直改共享状态。菜单回调
// 一律经 SendCmd 向主循环投递 TrayCommand；Config 仅用于构建时的初始勾选态读取，
// 菜单回调不写 Config/Theme/Startup，也不持久化——这些副作用全部收口在 app.Run
// 主循环的 applyConfigCommand 中，确保跨 goroutine 唯一写者。
type Deps struct {
	// Config 可变配置指针：仅用于菜单构建期读取初始勾选态（如 显示农历/开机启动）。
	// 菜单回调不写它。
	Config *config.Config
	// SendCmd 向主循环推送命令（显示/隐藏、退出、配置切换等）。
	SendCmd func(platform.TrayCommand)
	// Ctx 预留上下文（当前仅占位，避免未来扩展时改签名）。
	Ctx context.Context
}

// ctx 返回有效 context。
func (d Deps) ctx() context.Context {
	if d.Ctx != nil {
		return d.Ctx
	}
	return context.Background()
}

// BuildTrayMenu 构造托盘右键菜单（声明式 MenuItem 树）。
//
// 结构（MVP）：
//
//	显示/隐藏          (CmdToggle)
//	------------------
//	显示农历           [✓ checkbox]
//	显示节假日         [✓ checkbox]
//	开机启动           [✓ checkbox]
//	------------------
//	浅色               (CmdThemeLight)
//	深色               (CmdThemeDark)
//	跟随系统           (CmdThemeSystem)
//	------------------
//	退出               (CmdQuit)
//
// 注：主题三选项平铺为顶层项（不再用子菜单）。原因：gogpu/systray v0.1.2 的
// showContextMenu 按顶层索引 t.menu.Items[ret-1] 派发点击，但 populateMenu 对
// 子菜单项用「每级独立 1-based ID」，含子菜单时整张菜单派发错乱，连带「退出」等
// 顶层项点击失效（#150）。平铺后规避该库已知 bug，所有项点击均正确派发 OnClick。
func BuildTrayMenu(d Deps) *platform.TrayMenu {
	cfg := d.Config
	if cfg == nil {
		c := config.Default()
		cfg = &c
	}

	// 开机启动初始勾选态：以 config 为准（真实注册表状态由主循环在应用
	// CmdToggleStartup 时经 StartupManager 对齐，settings 不直触注册表）。
	autoChecked := cfg.Startup.AutoStart

	return &platform.TrayMenu{
		Items: []*platform.MenuItem{
			{
				Label:   "显示/隐藏",
				OnClick: func() { d.SendCmd(platform.CmdToggle) },
			},
			{Separator: true},
			{
				Label:   "显示农历",
				Checked: cfg.Display.ShowLunar,
				// 单写者：仅发命令，由主循环翻转 Config.Display.ShowLunar 并持久化。
				OnToggle: func(checked bool) { d.SendCmd(platform.CmdToggleLunar) },
			},
			{
				Label:   "显示节假日",
				Checked: cfg.Display.ShowHoliday,
				OnToggle: func(checked bool) { d.SendCmd(platform.CmdToggleHoliday) },
			},
			{
				Label:   "开机启动",
				Checked: autoChecked,
				OnToggle: func(checked bool) { d.SendCmd(platform.CmdToggleStartup) },
			},
			{Separator: true},
			{
				Label:   "浅色",
				OnClick: d.applyTheme("light"),
			},
			{
				Label:   "深色",
				OnClick: d.applyTheme("dark"),
			},
			{
				Label:   "跟随系统",
				OnClick: d.applyTheme("system"),
			},
			{Separator: true},
			{
				Label:   "退出",
				OnClick: func() { d.SendCmd(platform.CmdQuit) },
			},
		},
	}
}

// applyTheme 返回主题项点击回调：仅投递命令，由主循环写 Config.Theme.Mode
// + ApplyMode + 持久化（单写者，settings 不触主题/配置写）。主题项平铺为顶层项
// （见 BuildTrayMenu 注释），规避 systray 子菜单 ID 冲突 bug。
func (d Deps) applyTheme(mode string) func() {
	return func() { d.SendCmd(cmdForMode(mode)) }
}

// cmdForMode 将字符串模式映射为对应主题命令。
func cmdForMode(mode string) platform.TrayCommand {
	switch mode {
	case "light":
		return platform.CmdThemeLight
	case "dark":
		return platform.CmdThemeDark
	default:
		return platform.CmdThemeSystem
	}
}
