package app

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/shell"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// fakeWindow 观察生命周期对窗口的调用（与 win32.fakeWindow 类似，但本包测试自包含）。
// 额外实现 Present 以验证 90-UI 渲染层推送像素。
type fakeWindow struct {
	visible    bool
	anchorRect image.Rectangle
	showCalls  int
	hideCalls  int
	quitCalls  int
	presents   []*image.RGBA
	onClickFn  func(int, int) // 注册的左键点击回调（#113）
}

func (w *fakeWindow) Show()                             { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                             { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                     { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }
func (w *fakeWindow) Present(b *image.RGBA)             { w.presents = append(w.presents, b) }
func (w *fakeWindow) OnClick(fn func(int, int))         { w.onClickFn = fn }
func (w *fakeWindow) Quit()                             { w.quitCalls++ }

var _ shell.WindowController = (*fakeWindow)(nil)

// fakeTray 模拟托盘：Run 记录 app 装配的菜单并阻塞至 ctx 取消（模拟 systray
// 消息泵常驻）。命令由菜单回调（经 app 的 SendCmd 闭包）或左键回调推送至主循环，
// fakeTray 自身不持有命令通道；测试经菜单「退出」项触发 CmdQuit 退出。
type fakeTray struct {
	click    func()
	bounds   image.Rectangle
	lastMenu *platform.TrayMenu
}

func (t *fakeTray) SetIcon([]byte) error { return nil }
func (t *fakeTray) SetTooltip(string)    {}
func (t *fakeTray) OnClick(fn func())    { t.click = fn }
func (t *fakeTray) Bounds() (int, int, int, int) {
	return t.bounds.Min.X, t.bounds.Min.Y, t.bounds.Dx(), t.bounds.Dy()
}
func (t *fakeTray) Remove() error { return nil }
func (t *fakeTray) Run(ctx context.Context, menu *platform.TrayMenu) error {
	t.lastMenu = menu
	<-ctx.Done()
	return nil
}

var _ platform.TrayManager = (*fakeTray)(nil)

// findMenuItem 在菜单树（含子菜单）中按 label 查找项。
func findMenuItem(items []*platform.MenuItem, label string) *platform.MenuItem {
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.Label == label {
			return it
		}
		if it.Submenu != nil {
			if found := findMenuItem(it.Submenu, label); found != nil {
				return found
			}
		}
	}
	return nil
}

// waitVisible 轮询窗口可见态直至期望或超时。
func waitVisible(t *testing.T, w *fakeWindow, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for w.Visible() != want {
		if time.Now().After(deadline) {
			t.Fatalf("window visible = %v, want %v (timeout)", w.Visible(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// fakeStartup 记录 Enable/Disable（模拟 HKCU Run 注册表）。
type fakeStartup struct {
	enabled      bool
	enableCalls  int
	disableCalls int
}

func (f *fakeStartup) Enable(context.Context) error {
	f.enableCalls++
	f.enabled = true
	return nil
}
func (f *fakeStartup) Disable(context.Context) error {
	f.disableCalls++
	f.enabled = false
	return nil
}
func (f *fakeStartup) Enabled(context.Context) (bool, error) {
	return f.enabled, nil
}

// TestRun_MenuToggleThenQuit 验证双循环装配：托盘右键菜单回调 → SendCommand →
// 主循环 → lifecycle.Handle → 窗口；退出前持久化配置。直接触发 app 装配的菜单
// 项是端到端验证（含 settings.BuildTrayMenu 接线）。
func TestRun_MenuToggleThenQuit(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	trayRect := image.Rect(100, 900, 132, 932) // 托盘图标矩形（物理像素）
	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}

	done := make(chan error, 1)
	cfg := config.Default()
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			ConfigPath:     cfgPath,
		})
	}()

	// 等待菜单装配完成。
	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	// 显示/隐藏 → 显示（show=1）；再显示/隐藏 → 隐藏（hide=1）。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, false)

	// 退出项 → CmdQuit → 主循环退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if win.showCalls != 1 {
		t.Errorf("showCalls = %d, want 1", win.showCalls)
	}
	// 一次来自切换隐藏，一次来自退出前 Hide()。
	if win.hideCalls != 2 {
		t.Errorf("hideCalls = %d, want 2", win.hideCalls)
	}
	// 显示时锚定到托盘矩形。
	if win.anchorRect != trayRect {
		t.Errorf("anchorRect = %v, want %v", win.anchorRect, trayRect)
	}
	// 退出前持久化配置到指定路径。
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config not persisted: %v", statErr)
	}
}

// TestRun_LeftClickToggles 验证左键 OnClick 回调推送 CmdToggle，使主循环驱动窗口
// 显示。测试以 goroutine 运行 Run，手动模拟单击后再受控退出。
func TestRun_LeftClickToggles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(50, 800, 82, 832)

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			ConfigPath:     cfgPath,
		})
	}()

	// 等待 OnClick 注册完成（Run 在阻塞主循环前同步注册）。
	deadline := time.Now().Add(2 * time.Second)
	for tray.click == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.click == nil {
		t.Fatal("OnClick was not registered")
	}
	// 模拟左键单击：应使窗口可见。
	tray.click()
	waitVisible(t, win, true)

	// 经菜单「退出」项受控退出。
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_MenuAutoStartPersists 验证菜单「开机启动」勾选经 settings 回调触发
// 自启管理器 + 写 config.json（副作用联动 T3/T4）。
func TestRun_MenuAutoStartPersists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	win := &fakeWindow{}
	tray := &fakeTray{bounds: image.Rect(10, 20, 24, 24)}
	cfg := config.Default() // AutoStart=false
	su := &fakeStartup{enabled: false}

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return image.Rect(10, 20, 34, 44) },
			Config:         &cfg,
			ConfigPath:     cfgPath,
			Startup:        su,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	// 勾选「开机启动」→ 经 CmdToggleStartup 由主循环应用 Enable + 写 config。
	// 注意：SendCommand 非阻塞，命令异步由主循环消费，须轮询等待落地。
	findMenuItem(tray.lastMenu.Items, "开机启动").OnToggle(true)
	deadline = time.Now().Add(2 * time.Second)
	for su.enableCalls == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if su.enableCalls != 1 {
		t.Errorf("startup Enable called %d times, want 1", su.enableCalls)
	}
	if !cfg.Startup.AutoStart {
		t.Errorf("config.Startup.AutoStart = %v, want true (applied on main loop)", cfg.Startup.AutoStart)
	}
	// 配置已落盘且含 auto_start=true。
	got, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if !got.Startup.AutoStart {
		t.Errorf("persisted auto_start = %v, want true", got.Startup.AutoStart)
	}

	// 经菜单「退出」项退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_RendersAndPresentsCalendar 验证注入 Calendar+Theme 时，窗口显示会触发
// ui.Render 并将 360×480 像素缓冲经 Present 推送给窗口。
func TestRun_RendersAndPresentsCalendar(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	svc, err := calendar.NewDefaultCalendarService(nil, calendar.WithSelected(time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)))
	if err != nil {
		t.Fatalf("calendar service: %v", err)
	}
	tp, terr := theme.NewProvider(theme.WithInitialScheme(theme.SchemeLight))
	if terr != nil {
		t.Fatalf("theme provider: %v", terr)
	}

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			Calendar:       svc,
			Theme:          tp,
			ConfigPath:     cfgPath,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	// 切换显示 → Render → Present。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)

	// 等待渲染 goroutine 推送（render 在 win.Visible() 之后同步调用）。
	deadline = time.Now().Add(time.Second)
	for len(win.presents) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(win.presents) == 0 {
		t.Fatal("expected at least one Present call")
	}
	last := win.presents[len(win.presents)-1]
	if last.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Errorf("presented bounds = %v, want 360×480", last.Bounds())
	}

	// 退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_ClickNavigatesAndSelects 验证窗口左键点击（经 OnClick 回调 → 主循环命中
// 测试）能驱动月导航与日期选中，并触发重渲（#113 点击命中 + 上/下月导航 + 今天按钮
// + 格子选中 / #114 选中驱动重渲）。
func TestRun_ClickNavigatesAndSelects(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	// B1 修复（测试侧）：同时固定「今天」基准，使 GoToToday() 返回 2026-07-09
	// 而非真实系统今天；否则点击「今天」后 SelectedDate 落到系统当前日，断言失败。
	svc, err := calendar.NewDefaultCalendarService(nil,
		calendar.WithSelected(time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)),
		calendar.WithToday(time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)))
	if err != nil {
		t.Fatalf("calendar service: %v", err)
	}
	tp, terr := theme.NewProvider(theme.WithInitialScheme(theme.SchemeLight))
	if terr != nil {
		t.Fatalf("theme provider: %v", terr)
	}

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			Calendar:       svc,
			Theme:          tp,
			ConfigPath:     cfgPath,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	// 切换显示 → Render → Present。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)

	// 等待初次渲染推送。
	deadline = time.Now().Add(time.Second)
	for len(win.presents) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(win.presents) == 0 {
		t.Fatal("expected at least one Present call")
	}
	if win.onClickFn == nil {
		t.Fatal("OnClick was not registered on window")
	}

	// 点击「上一月」按钮（逻辑坐标：w=360 → prev 矩形 (268,14)-(296,42)，中点 282,28）。
	initialMonth := svc.MonthGrid().Month // 7 月
	win.onClickFn(282, 28)
	deadline = time.Now().Add(time.Second)
	for svc.MonthGrid().Month == initialMonth && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := svc.MonthGrid().Month; got != time.Month(6) {
		t.Fatalf("after prev click month = %v, want June(6)", got)
	}

	// 点击「今天」按钮（today 矩形 (210,14)-(254,42)，中点 232,28）→ 回 7 月并选中今天。
	win.onClickFn(232, 28)
	deadline = time.Now().Add(time.Second)
	for svc.MonthGrid().Month != initialMonth && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := svc.MonthGrid().Month; got != initialMonth {
		t.Fatalf("after today click month = %v, want %v", got, initialMonth)
	}
	wantSel := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	if !svc.SelectedDate().Equal(wantSel) {
		t.Fatalf("after today click selected = %v, want %v", svc.SelectedDate(), wantSel)
	}

	// 点击网格某格（row=2,col=3 → 逻辑坐标约 180,249）→ 选中该日并触发重渲。
	prevPresents := len(win.presents)
	win.onClickFn(180, 249)
	deadline = time.Now().Add(time.Second)
	for len(win.presents) == prevPresents && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(win.presents) == prevPresents {
		t.Fatal("expected a re-render after cell click")
	}

	// 退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_ConfigCommandsAppliedOnMainLoop 是 S1（并发）修复的回归测试：菜单回调只
// 经 SendCmd 投递命令；配置/主题/自启的写与 render() 全部由主循环单写者落地。
// 本测试触发各命令并断言主循环确实应用了变更（config 翻转、主题切换、startup 启用、
// 重渲发生），且全程无跨 goroutine 直改共享状态。
func TestRun_ConfigCommandsAppliedOnMainLoop(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	svc, err := calendar.NewDefaultCalendarService(nil, calendar.WithSelected(time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)))
	if err != nil {
		t.Fatalf("calendar service: %v", err)
	}
	tp, terr := theme.NewProvider(theme.WithInitialScheme(theme.SchemeLight))
	if terr != nil {
		t.Fatalf("theme provider: %v", terr)
	}

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default() // ShowLunar=true, Mode=system, AutoStart=false
	su := &fakeStartup{enabled: false}

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			Calendar:       svc,
			Theme:          tp,
			Startup:        su,
			ConfigPath:     cfgPath,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	// 先显示窗口（使后续重渲可见）。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)

	presentsBefore := len(win.presents)

	// 1) 显示农历：CmdToggleLunar 经主循环翻转 config + 重渲。
	findMenuItem(tray.lastMenu.Items, "显示农历").OnToggle(false)
	for cfg.Display.ShowLunar && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cfg.Display.ShowLunar {
		t.Errorf("ShowLunar should have flipped to false on main loop")
	}
	if len(win.presents) <= presentsBefore {
		t.Errorf("expected a re-render after ShowLunar toggle (presents %d vs %d)", len(win.presents), presentsBefore)
	}

	// 2) 主题→深色：CmdThemeDark 经主循环 ApplyMode + 翻转 config.Mode + 重渲。
	findMenuItem(tray.lastMenu.Items, "深色").OnClick()
	dl := time.Now().Add(2 * time.Second)
	for tp.Current().Scheme != theme.SchemeDark && time.Now().Before(dl) {
		time.Sleep(time.Millisecond)
	}
	if tp.Current().Scheme != theme.SchemeDark {
		t.Errorf("theme scheme = %v, want dark (applied on main loop)", tp.Current().Scheme)
	}
	if cfg.Theme.Mode != "dark" {
		t.Errorf("config.Theme.Mode = %q, want dark", cfg.Theme.Mode)
	}

	// 3) 开机启动：CmdToggleStartup 经主循环 Enable + 翻转 config。
	findMenuItem(tray.lastMenu.Items, "开机启动").OnToggle(true)
	for su.enableCalls == 0 && time.Now().Before(dl) {
		time.Sleep(time.Millisecond)
	}
	if su.enableCalls != 1 {
		t.Errorf("startup Enable called %d times, want 1", su.enableCalls)
	}
	if !cfg.Startup.AutoStart {
		t.Errorf("config.Startup.AutoStart = %v, want true", cfg.Startup.AutoStart)
	}

	// 退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_QuitSignalsWindowQuit 回归 N1：app.Run 的退出路径此前只 tray.Remove()+return，
// 未向窗口发 WM_QUIT/WM_DESTROY，导致窗口消息泵 goroutine 泄漏至进程退出。修复后，
// 退出路径经 window.Quit() 显式请求窗口线程退出，故 quit 时 fakeWindow.Quit 必须被调用一次。
func TestRun_QuitSignalsWindowQuit(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			ConfigPath:     cfgPath,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}

	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if win.quitCalls != 1 {
		t.Errorf("N1 regression: window.Quit called %d times on quit, want 1 (window goroutine would leak)", win.quitCalls)
	}
}

// TestRun_ShowOnLaunchByDefault 验证 v1.0 MVP 启动即弹窗（见 docs/20-Platform/Startup.md）：
// 未传 StartMinimized（默认 false）时，Run 启动即经 life.Handle(CmdShow) 显隐路径
// 弹出窗口（锚定到托盘矩形）。对应正常双击启动的场景。
func TestRun_ShowOnLaunchByDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return trayRect },
			Config:     &cfg,
			ConfigPath: cfgPath,
			// StartMinimized 默认 false → 启动弹窗
		})
	}()

	// 菜单装配完成后，窗口应已因启动弹窗而可见。
	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}
	if !win.Visible() {
		t.Fatal("expected window visible on launch by default (StartMinimized=false)")
	}
	if win.showCalls != 1 {
		t.Errorf("showCalls = %d, want 1 (initial launch show)", win.showCalls)
	}
	if win.anchorRect != trayRect {
		t.Errorf("anchorRect = %v, want %v", win.anchorRect, trayRect)
	}

	// 经菜单「退出」项退出。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRun_StartMinimizedStaysHidden 验证 --minimized（StartMinimized=true）启动后仅驻
// 托盘、窗口保持隐藏；点击托盘「显示/隐藏」才弹出。对应自启注册值 "exe --minimized"。
func TestRun_StartMinimizedStaysHidden(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(100, 900, 132, 932)

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:         win,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			ConfigPath:     cfgPath,
			StartMinimized: true,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for tray.lastMenu == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tray.lastMenu == nil {
		t.Fatal("tray menu was not built")
	}
	// 启动后应保持隐藏（仅驻托盘）。
	if win.Visible() {
		t.Fatal("expected window hidden at launch with StartMinimized=true")
	}
	if win.showCalls != 0 {
		t.Errorf("showCalls = %d, want 0 (no initial show when minimized)", win.showCalls)
	}

	// 点击托盘「显示/隐藏」→ 弹出。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)
	if win.showCalls != 1 {
		t.Errorf("showCalls = %d, want 1 after toggle", win.showCalls)
	}

	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestDefaultIcon(t *testing.T) {
	b := defaultIcon()
	if len(b) == 0 {
		t.Fatal("defaultIcon returned empty")
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("defaultIcon not valid PNG: %v", err)
	}
	if s := img.Bounds().Size(); s.X != 32 || s.Y != 32 {
		t.Errorf("icon size = %v, want 32x32", s)
	}
}

// TestNextMidnight 验证 P4-4 的每日 00:00 触发点计算：始终返回严格晚于 now 的
// 次日本地零点，与当前时刻无关（含零点整、23:59:59 边界）。
func TestNextMidnight(t *testing.T) {
	loc := time.Local
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{time.Date(2026, 7, 11, 13, 40, 0, 0, loc), time.Date(2026, 7, 12, 0, 0, 0, 0, loc)},
		{time.Date(2026, 7, 11, 0, 0, 0, 0, loc), time.Date(2026, 7, 12, 0, 0, 0, 0, loc)},
		{time.Date(2026, 7, 11, 23, 59, 59, 0, loc), time.Date(2026, 7, 12, 0, 0, 0, 0, loc)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, loc), time.Date(2027, 1, 1, 0, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got := nextMidnight(c.now)
		if !got.Equal(c.want) {
			t.Errorf("nextMidnight(%v) = %v, want %v", c.now, got, c.want)
		}
		if !got.After(c.now) {
			t.Errorf("nextMidnight(%v) = %v, must be strictly after now", c.now, got)
		}
		if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 || got.Nanosecond() != 0 {
			t.Errorf("nextMidnight(%v) = %v, want exactly 00:00:00", c.now, got)
		}
		if got.Location() != c.now.Location() {
			t.Errorf("nextMidnight(%v) location = %v, want %v", c.now, got.Location(), c.now.Location())
		}
	}
}
