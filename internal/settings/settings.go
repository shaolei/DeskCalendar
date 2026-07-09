// Package settings 构建托盘右键菜单（声明式 MenuItem 树），将菜单交互
// 接到 config 持久化与副作用（注册表自启 / 主题应用）。
//
// 路径 D / ADR-08：原 SettingsView 独立窗（gogpu/ui 控件树）降级为 v1.3 后备
// （见 #119）；MVP 设置走托盘右键菜单，经 gogpu/systray 的 AddCheckbox/AddSubmenu
// 渲染（由 internal/platform 负责）。本包只产出 platform.TrayMenu 数据模型，
// 不依赖 systray / UI 框架，因而可纯单测。
package settings

import (
	"context"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
)

// ThemeController 是主题应用的最小接口（由 *theme.ThemeProvider 满足）。
// 解耦 settings 与 theme 包内部细节。
type ThemeController interface {
	// ApplyMode 按 "system"|"light"|"dark" 应用主题。
	ApplyMode(mode string) error
}

// Deps 是构建托盘菜单所需的依赖（由 app 在装配期注入）。
type Deps struct {
	// Config 可变配置指针：菜单回调就地修改并（经 Persist）落盘。
	Config *config.Config
	// Persist 写 config.json（变更后调用）。
	Persist func() error
	// Startup 自启管理器（nil → 跳过注册表读写，仅改 config）。
	Startup platform.StartupManager
	// Theme 主题控制器（nil → 跳过主题应用，仅改 config）。
	Theme ThemeController
	// SendCmd 向主循环推送命令（显示/隐藏、退出）。
	SendCmd func(platform.TrayCommand)
	// Ctx 用于启动管理器查询（nil 时退化为 context.Background()）。
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
//	主题               (submenu: 浅色 / 深色 / 跟随系统)
//	------------------
//	退出               (CmdQuit)
//
// 注：T1 的「主题跟随系统」以子菜单的「跟随系统」项实现（单一可信源，避免
// 与复选框重复造成 UX 歧义）。
func BuildTrayMenu(d Deps) *platform.TrayMenu {
	cfg := d.Config
	if cfg == nil {
		c := config.Default()
		cfg = &c
	}

	// 开机启动初始勾选态：以注册表实际状态为准（若无管理器则用 config）。
	autoChecked := cfg.Startup.AutoStart
	if d.Startup != nil {
		if en, err := d.Startup.Enabled(d.ctx()); err == nil {
			autoChecked = en
		}
	}

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
				OnToggle: func(checked bool) {
					cfg.Display.ShowLunar = checked
					d.persist()
				},
			},
			{
				Label:   "显示节假日",
				Checked: cfg.Display.ShowHoliday,
				OnToggle: func(checked bool) {
					cfg.Display.ShowHoliday = checked
					d.persist()
				},
			},
			{
				Label:   "开机启动",
				Checked: autoChecked,
				OnToggle: func(checked bool) {
					if d.Startup != nil {
						if checked {
							_ = d.Startup.Enable(d.ctx())
						} else {
							_ = d.Startup.Disable(d.ctx())
						}
					}
					cfg.Startup.AutoStart = checked
					d.persist()
				},
			},
			{
				Label: "主题",
				Submenu: []*platform.MenuItem{
					{Label: "浅色", OnClick: d.applyTheme("light")},
					{Label: "深色", OnClick: d.applyTheme("dark")},
					{Label: "跟随系统", OnClick: d.applyTheme("system")},
				},
			},
			{Separator: true},
			{
				Label:   "退出",
				OnClick: func() { d.SendCmd(platform.CmdQuit) },
			},
		},
	}
}

// applyTheme 返回主题子菜单项点击回调：写 config.Mode + 应用主题 + 持久化。
func (d Deps) applyTheme(mode string) func() {
	return func() {
		if d.Config != nil {
			d.Config.Theme.Mode = mode
		}
		if d.Theme != nil {
			_ = d.Theme.ApplyMode(mode)
		}
		d.persist()
	}
}

// persist 安全调用 Persist（nil 时跳过，便于无持久化场景测试）。
func (d Deps) persist() {
	if d.Persist != nil {
		_ = d.Persist()
	}
}
