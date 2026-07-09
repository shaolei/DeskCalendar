// Package app 负责进程级装配（wire）：把平台层托盘、窗口层 win32 弹窗、
// shell 生命周期状态机接成可运行的双循环程序，并处理优雅退出。
//
// 路径 D / ADR-08：不依赖 gogpu.App / desktop.Run / gogpu/ui。窗口为自拥
// WS_POPUP 普通弹窗（internal/platform/win32），其消息泵在专属 goroutine；
// 托盘消息泵在独立 goroutine（platform.TrayManager.Run）；主 goroutine 运行
// 命令分发循环，消费托盘经 channel 下发的 platform.TrayCommand，驱动
// shell.Lifecycle.Handle —— 严守 ADR-02 双循环铁律：托盘 goroutine 只发命令，
// 窗口操作经 Lifecycle 在主 goroutine 发起、最终由 SendMessage 派发到窗口线程。
package app

import (
	"context"
	"image"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/platform/win32"
	"github.com/shaolei/DeskCalendar/internal/settings"
	"github.com/shaolei/DeskCalendar/internal/shell"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// Options 是 Run 的装配选项。生产环境由 main 填充；测试可注入 fake。
type Options struct {
	// Width/Height/Margin 弹窗逻辑尺寸与锚定留白（0 用默认 360×480 / 8）。
	Width, Height, Margin int
	// Icon 托盘图标 PNG 字节；空则使用内置默认图标。
	Icon []byte
	// Config/ConfigPath 退出前持久化的配置与路径（指针：菜单回调就地修改）。
	Config     *config.Config
	ConfigPath string

	// 以下为可注入依赖（nil 时使用生产实现），便于单测替换窗口/托盘/锚定。
	Window  shell.WindowController
	Tray    platform.TrayManager
	Anchor  func() image.Rectangle
	Startup platform.StartupManager // 开机自启；nil → 菜单「开机启动」仅改 config
	Theme   *theme.ThemeProvider    // 主题应用；nil → 菜单「主题」仅改 config
}

// Run 装配并启动双循环，返回即代表进程退出（优雅或非优雅）。
//
// 启动顺序（路径 D）：
//  1. 构造窗口（内部启动窗口线程消息泵）、托盘管理器、自启/主题管理器。
//  2. 接线生命周期（anchor=托盘矩形；persist=写 config；quit=取消 ctx）。
//  3. 构造托盘右键菜单（settings.BuildTrayMenu：显示/隐藏、显示农历/节假日、
//     开机启动复选框、主题子菜单、退出），回调经 SendCommand 向主循环下发命令
//     或改 config + 副作用。
//  4. 设置托盘图标/提示，注册左键单击 → CmdToggle。
//  5. go tray.Run(ctx, menu)（托盘消息泵，独立 goroutine，渲染菜单）。
//  6. 主循环 select cmdCh：消费命令 → lifecycle.Handle；StateQuit → 清理并返回。
func Run(opts Options) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if opts.Config == nil {
		c := config.Default()
		opts.Config = &c
	}
	win := opts.Window
	if win == nil {
		win = win32.NewWindow(win32.Options{
			Width:  opts.Width,
			Height: opts.Height,
			Margin: opts.Margin,
		})
	}
	tray := opts.Tray
	if tray == nil {
		tray = platform.NewTrayManager()
	}
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		if p, err := config.DefaultPath(); err == nil {
			cfgPath = p
		}
	}

	anchor := opts.Anchor
	if anchor == nil {
		anchor = func() image.Rectangle {
			x, y, w, h := tray.Bounds()
			return image.Rect(x, y, x+w, y+h)
		}
	}
	persist := func() error { return config.Save(cfgPath, *opts.Config) }

	life := shell.NewLifecycle(anchor, persist, cancel)

	// 托盘右键菜单（声明式，由 settings 包从 config/副作用构建）。
	cmdCh := make(chan platform.TrayCommand, 1)
	menu := settings.BuildTrayMenu(settings.Deps{
		Config:  opts.Config,
		Persist: persist,
		Startup: opts.Startup,
		Theme:   opts.Theme,
		SendCmd: func(c platform.TrayCommand) { platform.SendCommand(cmdCh, c) },
		Ctx:     ctx,
	})

	// 托盘图标与提示。
	if len(opts.Icon) > 0 {
		_ = tray.SetIcon(opts.Icon)
	} else {
		_ = tray.SetIcon(defaultIcon())
	}
	tray.SetTooltip("DeskCalendar")

	// 左键单击 → 切换（与右键菜单"显示/隐藏"同源，经 cmdCh 下发；
	// 非阻塞发送避免托盘 goroutine 在主循环空闲时被阻塞）。
	tray.OnClick(func() {
		select {
		case cmdCh <- platform.CmdToggle:
		default:
		}
	})

	go func() { _ = tray.Run(ctx, menu) }()

	// 主循环：消费托盘命令并驱动状态机（路径 D 替代 desktop.Run）。
	for {
		select {
		case cmd := <-cmdCh:
			life.Handle(cmd, win)
			if life.State() == shell.StateQuit {
				tray.Remove() // 退出前移除托盘图标，避免残留
				return nil
			}
		case <-ctx.Done():
			tray.Remove()
			return nil
		}
	}
}
