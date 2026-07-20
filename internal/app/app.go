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
	"sync/atomic"
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/platform/win32"
	"github.com/shaolei/DeskCalendar/internal/settings"
	"github.com/shaolei/DeskCalendar/internal/shell"
	"github.com/shaolei/DeskCalendar/internal/theme"
	"github.com/shaolei/DeskCalendar/internal/todo"
	"github.com/shaolei/DeskCalendar/internal/ui"
	"github.com/shaolei/DeskCalendar/internal/weather"
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
	Window   shell.WindowController
	Tray     platform.TrayManager
	Anchor   func() image.Rectangle
	Startup  platform.StartupManager  // 开机自启；nil → 菜单「开机启动」仅改 config
	Theme    *theme.ThemeProvider     // 主题应用；nil → 菜单「主题」仅改 config
	Calendar calendar.CalendarService // 日历聚合根；nil → 不渲染（仅测试）

	// StartMinimized 为 true 时仅驻托盘、启动不弹窗（对应自启注册值
	// `exe --minimized`，见 docs/20-Platform/Startup.md：v1.0 MVP 待实现项）。
	// false（默认）则正常启动即弹窗，点击托盘再隐藏。
	StartMinimized bool

	// Weather 天气服务（v1.2 EPIC #149）。nil → 不显示天气带（天气区空出给日历）。
	// 由 main 注入；生产默认经 Open-Meteo 免 key，填 QWeatherKey 自动切和风。
	Weather *weather.Service

	// Todo 待办服务（v1.1 EPIC #148）。nil → 不显示 Tab 条与待办视图，布局与旧版
	// 完全一致（向后兼容）；非 nil 时在面板顶部加 Tab 条并接入待办视图 + 提醒调度。
	Todo *todo.Service
}

// presenter 是额外具备像素推送能力的窗口（win32.WindowController 满足；
// 测试 fakeWindow 可补充 Present 实现）。
type presenter interface {
	shell.WindowController
	Present(b *image.RGBA)
}

// clicker 是额外具备左键点击回调能力的窗口（win32.WindowController 满足；
// 测试 fakeWindow 可补充 OnClick 实现）。#113 点击命中测试经此接线。
type clicker interface {
	OnClick(fn func(x, y int))
}

// keyboarder 是额外具备键入回调能力的窗口（win32.WindowController 满足；
// 测试 fakeWindow 未实现 → 断言失败、键盘路径静默跳过，不影响其它命令）。
// #148 待办输入框经此接线：OnChar 投递录入字符，OnKey 投递功能键。
type keyboarder interface {
	OnChar(fn func(r rune))
	OnKey(fn func(int))
}

// dpiAware 是额外暴露物理像素尺寸的窗口（win32.*win32Window 满足；
// 测试 fakeWindow 未实现 → 断言失败、Scale 退化为 0（逻辑分辨率渲染），
// 不影响其它命令）。#41 高 DPI：app 经 PhysicalSize 反算渲染 Scale 使 gg
// 位图与 DIB 1:1 清晰。不纳入 shell.WindowController 接口（接口隔离）。
type dpiAware interface {
	PhysicalSize() (int, int)
}

// dpiChangeListener 是额外支持注册 DPI 变更回调的窗口（win32.*win32Window 满足；
// 测试 fakeWindow 未实现 → 断言失败、DPI 变更不触发重渲，仅影响真实换屏场景）。
// 回调在窗口线程调用，但只经 channel 向主循环投递「需重渲」信号，不直改业务状态（S1）。
type dpiChangeListener interface {
	OnDPIChanged(fn func())
}

// dismisser 是额外支持注册「点击外部自动隐藏」回调的窗口（win32.*win32Window 满足；
// 测试 fakeWindow 未实现 → 断言失败、自动隐藏同步路径静默跳过，不影响其它命令）。
// 窗口线程在 waInactive 决定隐藏后调用，仅经 channel 向主循环投递信号，不直改业务
// 状态（S1 单写者）；主循环据此把生命周期的期望可见态回写为 false（#151 显示/隐藏卡死）。
type dismisser interface {
	OnDismissed(fn func())
}

// 功能键键码（与 win32 VK_* 对齐，仅 app 内部消费，避免反向 import win32 私有常量）。
const (
	keyEnter  = 0x0D // 提交待办草稿
	keyBack   = 0x08 // 删除草稿末字符
	keyTab    = 0x09 // 切换日历/待办视图
	keyDelete = 0x2E // 删除选中待办
)

// todoNotifier 把 platform.NotificationSender 适配为 todo.Notifier（接口隔离，
// 不要求 todo 包反向依赖 platform）。v1.1 真实 Toast 实现前 platform 侧为 noop 占位，
// 故提醒经此路径触发但不会真实弹窗，等待 v1.1 Toast 落地。
type todoNotifier struct {
	sender platform.NotificationSender
}

func (n *todoNotifier) Notify(ctx context.Context, title, body string) error {
	if n.sender == nil {
		return nil
	}
	return n.sender.Send(ctx, platform.Notification{Title: title, Body: body, Silent: false})
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
	// 装配窗口与托盘，并检测窗口是否支持 Present（像素推送）。
	win := opts.Window
	if win == nil {
		win = win32.NewWindow(win32.Options{
			Width:  opts.Width,
			Height: opts.Height,
			Margin: opts.Margin,
		})
	}
	pr, canPresent := win.(presenter)
	// dpiAware 断言（#41 高 DPI）：仅 win32 真实窗口支持 PhysicalSize，用来反算
	// 渲染 Scale = 物理宽 / 逻辑宽。测试 fakeWindow 不支持 → getScale 留 nil，
	// render() 时 Scale=0（逻辑分辨率渲染），与既有单测行为完全一致（向后兼容）。
	// 注意：getScale 在主循环/render 闭包中调用，读取的是 win32 原子化的 dibW，
	// 故换屏期间并发读取也安全（S1 范畴）。
	var getScale func() float64
	if da, ok := win.(dpiAware); ok {
		getScale = func() float64 {
			pw, _ := da.PhysicalSize()
			w := opts.Width
			if w <= 0 {
				w = ui.DefaultWidth
			}
			if pw <= 0 || w <= 0 {
				return 0
			}
			return float64(pw) / float64(w)
		}
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

	// 天气带高度：服务注入时预留顶部区域，日历区整体下移（#149）。
	wsvc := opts.Weather
	weatherBandH := 0
	if wsvc != nil {
		weatherBandH = ui.DefaultWeatherBandH
	}

	// 待办 Tab 条高度：仅注入待办服务时预留（#148）；否则 0，布局与旧版完全一致。
	tabStripH := 0
	if opts.Todo != nil {
		tabStripH = ui.DefaultTabStripH
	}

	// 应用级 UI 瞬态（仅由主循环单写者维护，S1）：待办视图模式、输入框草稿、编辑态、
	// 选中待办 ID。启动时默认日历视图。待办条数在 handleClick/render 时现取，不缓存。
	viewMode := ui.ViewCalendar
	draft := ""
	editing := false
	selectedTodoID := ""

	// 渲染闭包：model → ui.Render → Present。依赖非空时可用（测试可注入 fake）。
	render := func() {
		if pr == nil || opts.Calendar == nil || opts.Theme == nil {
			return
		}
		grid := opts.Calendar.MonthGrid()
		model := ui.NewMonthModel(grid, opts.Config.Display.ShowLunar, opts.Config.Display.ShowHoliday)
		if wsvc != nil {
			// 从天气服务快照映射天气卡片（保持 ui 不反向依赖 internal/weather）。
			model.Weather = mapWeatherSnapshot(wsvc.Snapshot())
		}
		todoCount := 0
		if opts.Todo != nil {
			// 待办视图字段由 app 从 todo.Service 映射（ui 不反向依赖 internal/todo，ADR-07a）。
			model.ViewMode = viewMode
			model.Draft = draft
			model.Editing = editing
			now := time.Now()
			todos, err := opts.Todo.List(context.Background(), todo.ListFilter{})
			if err == nil {
				items := make([]*ui.TodoItem, 0, len(todos))
				for _, t := range todos {
					items = append(items, &ui.TodoItem{
						ID:         t.ID,
						Title:      t.Title,
						Due:        t.Due,
						Status:     string(t.Status),
						Tags:       t.Tags,
						ReminderAt: t.ReminderAt,
						Overdue:    t.IsOverdue(now),
						DueSoon:    t.IsDueForReminder(now),
					})
				}
				model.Todos = items
				todoCount = len(items)
			}
		}
		// RenderOptions 与 HitTest 共用：天气带 + Tab 条 + 当前视图 + 待办条数，
		// 保证「画在哪、点哪」一致（#113/#148）。
		// Scale（#41 高 DPI）：仅在真实 win32 窗口下经 getScale 反算非 0，使
		// ui.Render 产出与 DIB 1:1 的物理位图（清晰）；fake/未支持时为 0（逻辑分辨率）。
		var scale float64
		if getScale != nil {
			scale = getScale()
		}
		bmp := ui.Render(model, ui.RenderOptions{
			Width:        opts.Width,
			Height:       opts.Height,
			WeatherBandH: weatherBandH,
			TabStripH:    tabStripH,
			ViewMode:     viewMode,
			TodoCount:    todoCount,
			Scale:        scale,
		}, opts.Theme.Current())
		pr.Present(bmp)
	}
	render() // 预渲初始帧，使首次 Show 瞬间有画面。

	// applyConfigCommand 是主循环内唯一写共享状态（opts.Config / opts.Theme /
	// opts.Calendar / opts.Startup）之处，落实单写者（代码审查 S1）：菜单回调与
	// 后台 goroutine 仅经 cmdCh 投递命令，从不直改上述字段；render() 也只在此处
	// 被主循环调用。返回 true 表示该命令已被本函数消费（无需再交 lifecycle.Handle）。
	applyConfigCommand := func(cmd platform.TrayCommand) bool {
		switch cmd {
		case platform.CmdToggleLunar:
			opts.Config.Display.ShowLunar = !opts.Config.Display.ShowLunar
		case platform.CmdToggleHoliday:
			opts.Config.Display.ShowHoliday = !opts.Config.Display.ShowHoliday
		case platform.CmdToggleStartup:
			opts.Config.Startup.AutoStart = !opts.Config.Startup.AutoStart
			if opts.Startup != nil {
				if opts.Config.Startup.AutoStart {
					_ = opts.Startup.Enable(ctx)
				} else {
					_ = opts.Startup.Disable(ctx)
				}
			}
		case platform.CmdThemeLight:
			opts.Config.Theme.Mode = "light"
			if opts.Theme != nil {
				_ = opts.Theme.ApplyMode("light")
			}
		case platform.CmdThemeDark:
			opts.Config.Theme.Mode = "dark"
			if opts.Theme != nil {
				_ = opts.Theme.ApplyMode("dark")
			}
		case platform.CmdThemeSystem:
			opts.Config.Theme.Mode = "system"
			if opts.Theme != nil {
				_ = opts.Theme.ApplyMode("system")
			}
		case platform.CmdRefreshToday:
			if opts.Calendar != nil {
				opts.Calendar.RefreshToday()
			}
		case platform.CmdRender:
			// 仅重渲，无配置变更。
		default:
			return false
		}
		// 配置变更类命令需持久化；纯渲染/刷新命令跳过。
		switch cmd {
		case platform.CmdToggleLunar, platform.CmdToggleHoliday, platform.CmdToggleStartup,
			platform.CmdThemeLight, platform.CmdThemeDark, platform.CmdThemeSystem:
			if persist != nil {
				_ = persist()
			}
		}
		if canPresent && win.Visible() {
			render()
		}
		return true
	}

	// handleClick 处理窗口客户区左键点击：命中测试 → 改日历状态 → 重渲。
	// 仅在主循环（主 goroutine）调用，落实单写者（S1）；窗口线程只经 OnClick 回调
	// 把逻辑坐标投递到 clickCh，不在此直改业务状态（ADR-02）。覆盖 #113（点击命中 +
	// 上/下月导航 + 今天按钮 + 格子选中）与 #114（选中/月份变更后重渲，高亮即时反映）。
	handleClick := func(p image.Point) {
		if opts.Calendar == nil {
			return
		}
		// 待办视图下，HitTest 需当前待办条数判定行命中（避免越界）。无论窗口是否可见
		// 都重新 list 取最新条数——隐藏态不重渲，缓存条数会过期，故此处必须现取，
		// 否则点击行会误判为 HitNone（#148 交互闭环）。
		todoCount := 0
		var todos []*todo.Todo
		if opts.Todo != nil && viewMode == ui.ViewTodo {
			if l, err := opts.Todo.List(context.Background(), todo.ListFilter{}); err == nil {
				todos = l
				todoCount = len(l)
			}
		}
		res := ui.HitTest(p.X, p.Y, ui.RenderOptions{
			Width:        opts.Width,
			Height:       opts.Height,
			WeatherBandH: weatherBandH,
			TabStripH:    tabStripH,
			ViewMode:     viewMode,
			TodoCount:    todoCount,
		})
		switch res.Kind {
		case ui.HitPrevMonth:
			opts.Calendar.PrevMonth()
		case ui.HitNextMonth:
			opts.Calendar.NextMonth()
		case ui.HitToday:
			opts.Calendar.GoToToday()
		case ui.HitCell:
			grid := opts.Calendar.MonthGrid()
			if res.Row >= 0 && res.Row < 6 && res.Col >= 0 && res.Col < 7 {
				opts.Calendar.SetSelectedDate(grid.Weeks[res.Row][res.Col].Date)
			}
		case ui.HitTabCalendar:
			// #148：切到日历视图。
			viewMode = ui.ViewCalendar
		case ui.HitTabTodo:
			// #148：切到待办视图。
			viewMode = ui.ViewTodo
		case ui.HitTodoRow, ui.HitTodoDelete:
			// #148：待办行交互（切换完成态 / 删除），按当前列表顺序定位领域对象。
			if opts.Todo == nil || res.Row < 0 || res.Row >= len(todos) {
				return
			}
			t := todos[res.Row]
			if res.Kind == ui.HitTodoDelete {
				_ = opts.Todo.Remove(context.Background(), t.ID)
				if selectedTodoID == t.ID {
					selectedTodoID = ""
				}
			} else { // HitTodoRow：切换完成态（active<->done），并记录选中。
				if t.Status == todo.StatusActive {
					_, _ = opts.Todo.Complete(context.Background(), t.ID)
				} else {
					_, _ = opts.Todo.Reopen(context.Background(), t.ID)
				}
				selectedTodoID = t.ID
			}
		case ui.HitTodoDraft:
			// #148：聚焦输入框。
			editing = true
		default:
			return
		}
		// #114：状态变更后重渲，今日/选中高亮（IsToday/IsSelected 描边）即时刷新。
		if canPresent && win.Visible() {
			render()
		}
	}

	// 托盘右键菜单（声明式，由 settings 包仅产出命令；配置/主题/自启的写与持久化
	// 收口于下方主循环的 applyConfigCommand，确保单写者，消除 S1 并发竞争）。
	// 普通命令通道（配置/主题/刷新/渲染/切换）。缓冲 16 容纳一次性命令突发
	// （用户点击 + 主题变更 + 每日刷新），避免主循环 render() 期间短暂阻塞丢命令。
	cmdCh := make(chan platform.TrayCommand, 16)
	// 退出信号走独立可靠通道（缓冲 1），与 cmdCh 解耦：即便 cmdCh 被高频命令占满，
	// 单次退出也不会被非阻塞 SendCommand 静默丢弃 → 根治 S4 退出死锁（替代原 16 缓冲缓解）。
	quitCh := make(chan struct{}, 1)
	// clickCh 承载窗口客户区左键点击的逻辑坐标（#113）。窗口线程经 OnClick 回调
	// 仅投递坐标，不直改业务状态（ADR-02）；命中测试与日历变更在主循环消费。
	clickCh := make(chan image.Point, 8)
	// charCh/keyCh 承载键盘输入（#148 待办输入框）。窗口线程经 OnChar/OnKey 回调
	// 仅投递字符/键码，不直改业务状态（S1 单写者）；主循环消费后改草稿/视图/待办。
	charCh := make(chan rune, 16)
	keyCh := make(chan int, 16)
	// redrawCh 承载 DPI 变更后的重渲信号（#41 高 DPI）。窗口线程在 WM_DPICHANGED
	// 重建 DIB 后，经 OnDPIChanged 回调仅非阻塞投递此信号（不直改业务状态，S1 单写者）；
	// 主循环消费后重渲，使 gg 位图以新 DPI 物理分辨率产出、与 DIB 1:1 清晰。
	// 缓冲 1 即可：DPI 变更是低频事件，且重渲本身串行于主循环，无需堆积。
	redrawCh := make(chan struct{}, 1)
	// dismissedCh 承载窗口「点击外部自动隐藏」(waInactive) 后的唤醒信号（#151 显示/隐藏
	// 一致性）。窗口线程经 OnDismissed 回调仅非阻塞投递唤醒（不直改业务状态，S1 单写者）；
	// 主循环消费后把生命周期的期望可见态回写为 false，使后续托盘「显示/隐藏」切换基于正确意图。
	// 缓冲 1；真正的「待回写次数」用 dismissCount 计数，避免突发自动隐藏时信号被丢弃导致
	// desiredVisible 与真实可见态错位（#151 后续 desync 修复：此前缓冲 1 非阻塞发送会丢信号）。
	dismissedCh := make(chan struct{}, 1)
	var dismissCount int64
	sendCmd := func(c platform.TrayCommand) {
		if c == platform.CmdQuit {
			// 退出路由到 quitCh（可靠），不占 cmdCh 缓冲，杜绝满载丢命令。
			select {
			case quitCh <- struct{}{}:
			default:
			}
			return
		}
		platform.SendCommand(cmdCh, c)
	}
	menu := settings.BuildTrayMenu(settings.Deps{
		Config:  opts.Config,
		SendCmd: sendCmd,
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

	// 窗口左键点击 → clickCh：仅投递逻辑坐标，命中测试与状态变更交由主循环
	// （单写者 + 双循环铁律）。若窗口实现不支持 OnClick（如仅实现 shell 接口的
	// 测试 fake），此断言失败，点击路径静默跳过，不影响其它命令。
	if c, ok := win.(clicker); ok {
		c.OnClick(func(x, y int) {
			select {
			case clickCh <- image.Point{X: x, Y: y}:
			default:
			}
		})
	}
	// 键盘输入 → charCh/keyCh（#148 待办输入框）：仅投递字符/键码，业务状态变更
	// 交由主循环（单写者）。测试 fake 未实现 keyboarder → 断言失败，键盘路径跳过。
	if kb, ok := win.(keyboarder); ok {
		kb.OnChar(func(r rune) {
			select {
			case charCh <- r:
			default:
			}
		})
		kb.OnKey(func(k int) {
			select {
			case keyCh <- k:
			default:
			}
		})
	}
	// DPI 变更 → redrawCh（#41 高 DPI 重渲）：仅投递信号，重渲交给主循环（单写者）。
	// 测试 fake 未实现 dpiChangeListener → 断言失败，换屏重渲路径静默跳过，不影响其它命令。
	if dc, ok := win.(dpiChangeListener); ok {
		dc.OnDPIChanged(func() {
			select {
			case redrawCh <- struct{}{}:
			default:
			}
		})
	}
	// 点击外部自动隐藏 → dismissedCh（#151 显示/隐藏一致性）：窗口线程经 OnDismissed
	// 仅非阻塞投递唤醒，主循环消费后把生命周期期望可见态回写为 false。测试 fake 未实现
	// dismisser → 断言失败，自动隐藏同步路径静默跳过，不影响其它命令。
	// 用 dismissCount 计「待回写次数」再非阻塞唤醒：突发自动隐藏（含 wmClose/Esc 路径）
	// 时即便唤醒被丢弃，计数仍累积，主循环消费时一次性回写，杜绝 desiredVisible 错位。
	if d, ok := win.(dismisser); ok {
		d.OnDismissed(func() {
			atomic.AddInt64(&dismissCount, 1)
			select {
			case dismissedCh <- struct{}{}:
			default:
			}
		})
	}

	go func() { _ = tray.Run(ctx, menu) }()

	// 天气后台刷新：启动后异步首拉 + 每 30min 定时刷新；每次刷新完成经 onUpdate
	// 向主循环发 CmdRender 重渲（onUpdate 在后台 goroutine 调用，仅发 channel，
	// 不直写共享状态——单写者铁律）。断网/超时不阻塞，自动降级到缓存/空。
	// 进程退出时 Stop 收口刷新 goroutine，杜绝泄漏。
	if wsvc != nil {
		wsvc.SetOnUpdate(func() {
			platform.SendCommand(cmdCh, platform.CmdRender)
		})
		go wsvc.Start(ctx)
		defer wsvc.Stop()
	}

	// 待办提醒调度（#148）：注入待办服务时启动。每 30s（启动即扫描一次）扫描到期
	// 提醒，经 todoNotifier 适配 platform.Notification 弹通知（v1.1 真实 Toast 落地前
	// 为 noop 占位）；store 传 nil（运行时不依赖 internal/state）。后台 goroutine，
	// ctx 取消即停，杜绝泄漏。
	if opts.Todo != nil {
		notifier := &todoNotifier{sender: platform.NewNotificationSender("DeskCalendar")}
		reminder := todo.NewReminderService(opts.Todo, notifier, nil, todo.SchedulerConfig{
			Interval:     30 * time.Second,
			ImmediateRun: true,
		})
		go func() {
			_ = reminder.Start(ctx)
		}()
		defer reminder.Stop()
	}

	// 主题跟随：系统浅/深切换经 Watch 推送；转发为 CmdRender 命令，由主循环
	// 重渲（不在本 goroutine 读写共享状态，S1 单写者）。
	if canPresent && opts.Theme != nil {
		go func() {
			ch := opts.Theme.Watch(ctx)
			for range ch {
				platform.SendCommand(cmdCh, platform.CmdRender)
			}
		}()
	}

	// handleKey 处理功能键（#148 待办输入框）：Enter 提交草稿、Backspace 删字符、
	// Tab 切视图、Delete 删选中待办。仅主循环调用（单写者）。视图切换/草稿编辑
	// 在任何视图下都生效；草稿提交/删除待办仅待办视图 + 待办服务注入时生效。
	handleKey := func(k int) {
		switch k {
		case keyTab:
			// 切换日历/待办视图（任一视图下均可）。
			if viewMode == ui.ViewTodo {
				viewMode = ui.ViewCalendar
			} else {
				viewMode = ui.ViewTodo
			}
		case keyEnter:
			if viewMode == ui.ViewTodo && opts.Todo != nil && draft != "" {
				_, _ = opts.Todo.Add(context.Background(), draft, todo.AddOpts{})
				draft = ""
				editing = false
			}
		case keyBack:
			if viewMode == ui.ViewTodo && len(draft) > 0 {
				draft = draft[:len(draft)-1]
			}
		case keyDelete:
			if viewMode == ui.ViewTodo && opts.Todo != nil && selectedTodoID != "" {
				_ = opts.Todo.Remove(context.Background(), selectedTodoID)
				selectedTodoID = ""
			}
		default:
			return
		}
		if canPresent && win.Visible() {
			render()
		}
	}
	// 命令，由主循环调用 calendar.RefreshToday + 重渲，避免 midnight goroutine 直写
	// calendar.today（S1 单写者）。改为每日 00:00 精确触发（P4-4，替代 30 分钟轮询）。
	if opts.Calendar != nil {
		go func() {
			for {
				now := time.Now()
				timer := time.NewTimer(time.Until(nextMidnight(now)))
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
					platform.SendCommand(cmdCh, platform.CmdRefreshToday)
				}
			}
		}()
	}

	// 等待托盘图标创建完成（同线程模型下 New 在 Run 的 goroutine 内完成），确保
	// Bounds() 返回有效托盘矩形，避免窗口被锚定到 (0,0)（真机首跑暴露"锚定左上角"）。
	// 以 ctx.Done() 兜底，避免托盘创建失败时永久阻塞启动（此时锚定退化为默认位置）。
	select {
	case <-tray.Ready():
	case <-ctx.Done():
	}

	// 启动即显隐策略（v1.0 MVP，见 docs/20-Platform/Startup.md）：
	// 默认（非 --minimized）正常启动即弹窗；自启（--minimized）仅驻托盘，
	// 等用户点托盘才显示。经 life.Handle(CmdShow) 复用与托盘「显示/隐藏」同源
	// 的显隐路径（锚定→Show），严守 ADR-02 双循环铁律（主 goroutine 发起窗口操作）。
	if !opts.StartMinimized {
		life.Handle(platform.CmdShow, win)
	} else {
	}

	// 主循环：消费托盘命令并驱动状态机（路径 D 替代 desktop.Run）。
	for {
		select {
		case cmd := <-cmdCh:
			// 单写者：先由 applyConfigCommand 消费配置/主题/刷新/渲染命令。
			if applyConfigCommand(cmd) {
				continue
			}
			life.Handle(cmd, win)
			if life.State() == shell.StateQuit {
				win.Quit()    // N1：显式请求窗口退出消息泵 goroutine，杜绝泄漏
				tray.Remove() // 退出前移除托盘图标，避免残留
				return nil
			}
			// 窗口显示后重渲，确保显示的是最新月/主题/显示开关。
			if canPresent && win.Visible() {
				render()
			}
		case p := <-clickCh:
			// #113/#114：窗口左键点击 → 命中测试 → 改月/选中 → 重渲（单写者）。
			handleClick(p)
		case r := <-charCh:
			// #148：字符输入 → 仅待办视图下追加到草稿（单写者）。
			if viewMode == ui.ViewTodo && opts.Todo != nil {
				draft += string(r)
				if canPresent && win.Visible() {
					render()
				}
			}
		case k := <-keyCh:
			// #148：功能键 → 视图切换 / 草稿提交 / 删除待办（单写者）。
			handleKey(k)
		case <-redrawCh:
			// #41 高 DPI：换屏后 DIB 已按新 DPI 重建，重渲使 gg 位图以新物理
			// 分辨率产出、与 DIB 1:1 清晰。仅在窗口可见时重渲（隐藏时无需像素）。
			if canPresent && win.Visible() {
				render()
			}
		case <-dismissedCh:
			// #151 显示/隐藏一致性：窗口「点击外部/关闭」自行隐藏后，把生命周期的期望可见态
			// 同步为 false，使后续托盘「显示/隐藏」切换基于正确意图（而非过时的 win.Visible()），
			// 避免自动隐藏后首次切换因状态机与窗口实际态不一致而「吞掉一次切换」。dismissCount
			// 累积突发次数，交换归零后只要 >0 即回写一次（idempotent），确保信号不丢、不重。
			if atomic.SwapInt64(&dismissCount, 0) > 0 {
				life.NotifyAutoHidden()
			}
		case <-quitCh:
			// 可靠退出路径（S4 根治）：经 quitCh 必达，不受 cmdCh 缓冲影响。
			life.Handle(platform.CmdQuit, win) // 置 StateQuit + 持久化配置 + 取消 ctx
			win.Quit() // 显式收口窗口线程（N1）
			tray.Remove()
			return nil
		case <-ctx.Done():
			win.Quit() // N1：上下文取消（如后台 goroutine 异常）也收口窗口 goroutine
			tray.Remove()
			return nil
		}
	}
}

// nextMidnight 返回 now 之后本地时区的下一个 00:00:00（严格晚于 now 的次日零点）。
// 用 AddDate(0,0,1) 而非 Add(24h)，规避夏令时切换日的时长偏差（P4-4）。
func nextMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
}

// mapWeatherSnapshot 把 weather.Service.Snapshot 映射为 ui.WeatherCard（保持 ui
// 不反向依赖 internal/weather，满足 ADR-07a 依赖方向）。映射规则：
//   - StatusReady/StatusStale → ui.WeatherReady（Stale 时 card.Stale=true 显示「旧数据」）
//   - StatusLoading          → ui.WeatherLoading（无数据则 UI 显示降级占位）
//   - StatusDisabled/Error   → ui.WeatherError（UI 显示「天气暂不可用」）
func mapWeatherSnapshot(s weather.Snapshot) *ui.WeatherCard {
	card := &ui.WeatherCard{
		Stale: s.Stale,
	}
	if s.Current != nil {
		card.Source = s.Current.Source
	} else if len(s.Forecast) > 0 && s.Forecast[0] != nil {
		card.Source = s.Forecast[0].Source
	}
	switch s.Status {
	case weather.StatusReady, weather.StatusStale:
		card.Status = ui.WeatherReady
	case weather.StatusLoading:
		card.Status = ui.WeatherLoading
	default: // StatusDisabled / StatusError
		card.Status = ui.WeatherError
	}
	if s.Current != nil {
		card.Current = &ui.WeatherItem{
			TempC:         s.Current.TempC,
			LowC:          s.Current.LowC,
			ConditionText: s.Current.ConditionText,
			Icon:          s.Current.Icon,
			IsDay:         s.Current.IsDay,
			Pop:           s.Current.Pop,
			HasRange:      false,
		}
	}
	for _, f := range s.Forecast {
		if f == nil {
			continue
		}
		card.Forecast = append(card.Forecast, &ui.WeatherItem{
			TempC:         f.TempC,
			LowC:          f.LowC,
			ConditionText: f.ConditionText,
			Icon:          f.Icon,
			Pop:           f.Pop,
			HasRange:      true,
		})
	}
	return card
}
