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
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/platform/win32"
	"github.com/shaolei/DeskCalendar/internal/settings"
	"github.com/shaolei/DeskCalendar/internal/shell"
	"github.com/shaolei/DeskCalendar/internal/theme"
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
		bmp := ui.Render(model, ui.RenderOptions{Width: opts.Width, Height: opts.Height, WeatherBandH: weatherBandH}, opts.Theme.Current())
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
		res := ui.HitTest(p.X, p.Y, ui.RenderOptions{Width: opts.Width, Height: opts.Height, WeatherBandH: weatherBandH})
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
	menu := settings.BuildTrayMenu(settings.Deps{
		Config: opts.Config,
		SendCmd: func(c platform.TrayCommand) {
			if c == platform.CmdQuit {
				// 退出路由到 quitCh（可靠），不占 cmdCh 缓冲，杜绝满载丢命令。
				select {
				case quitCh <- struct{}{}:
				default:
				}
				return
			}
			platform.SendCommand(cmdCh, c)
		},
		Ctx: ctx,
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

	// 每日刷新「今天」基准：跨午夜后 IsToday 自动纠正（S4）。转发为 CmdRefreshToday
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

	// 启动即显隐策略（v1.0 MVP，见 docs/20-Platform/Startup.md）：
	// 默认（非 --minimized）正常启动即弹窗；自启（--minimized）仅驻托盘，
	// 等用户点托盘才显示。经 life.Handle(CmdShow) 复用与托盘「显示/隐藏」同源
	// 的显隐路径（锚定→Show），严守 ADR-02 双循环铁律（主 goroutine 发起窗口操作）。
	if !opts.StartMinimized {
		life.Handle(platform.CmdShow, win)
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
		case <-quitCh:
			// 可靠退出路径（S4 根治）：经 quitCh 必达，不受 cmdCh 缓冲影响。
			life.Handle(platform.CmdQuit, win) // 置 StateQuit + 持久化配置 + 取消 ctx
			win.Quit()                         // 显式收口窗口线程（N1）
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
