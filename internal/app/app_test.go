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
	presents   []*image.RGBA
}

func (w *fakeWindow) Show()                              { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                              { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                     { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }
func (w *fakeWindow) Present(b *image.RGBA)             { w.presents = append(w.presents, b) }

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
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return trayRect },
			Config:     &cfg,
			ConfigPath: cfgPath,
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
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return trayRect },
			Config:     &cfg,
			ConfigPath: cfgPath,
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
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return image.Rect(10, 20, 34, 44) },
			Config:     &cfg,
			ConfigPath: cfgPath,
			Startup:    su,
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
			Window:   win,
			Tray:     tray,
			Anchor:   func() image.Rectangle { return trayRect },
			Config:   &cfg,
			Calendar: svc,
			Theme:    tp,
			ConfigPath: cfgPath,
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
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return trayRect },
			Config:     &cfg,
			Calendar:   svc,
			Theme:      tp,
			Startup:    su,
			ConfigPath: cfgPath,
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
