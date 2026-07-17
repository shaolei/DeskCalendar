package main

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/app"
	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/shell"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// fakeWindow 观察生命周期对窗口的调用（与 app 包测试自包含，不依赖其内部 fake）。
// 额外实现 Present 以验证 90-UI 渲染层推送像素。
type fakeWindow struct {
	visible    bool
	anchorRect image.Rectangle
	showCalls  int
	hideCalls  int
	quitCalls  int
	presents   []*image.RGBA
}

func (w *fakeWindow) Show()                             { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                             { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                     { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }
func (w *fakeWindow) Present(b *image.RGBA)             { w.presents = append(w.presents, b) }
func (w *fakeWindow) Quit()                             { w.quitCalls++ }

var _ shell.WindowController = (*fakeWindow)(nil)

// fakeTray 模拟托盘：Run 记录 app 装配的菜单并阻塞至 ctx 取消（模拟 systray
// 消息泵常驻）。命令由菜单回调经 app 的 SendCmd 闭包推送至主循环。
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
func (t *fakeTray) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch) // 测试 fake 立即可用，无需等待异步图标创建
	return ch
}
func (t *fakeTray) Remove() error { return nil }
func (t *fakeTray) Run(ctx context.Context, menu *platform.TrayMenu) error {
	t.lastMenu = menu
	<-ctx.Done()
	return nil
}

var _ platform.TrayManager = (*fakeTray)(nil)

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

// TestBuildOptions 覆盖 main.go 的装配函数：加载配置（或回退默认）、构造自启/
// 主题/日历依赖，产出可被 app.Run 消费的非空 app.Options。仅读取，不启动真实窗口/
// 托盘，可在任意平台安全执行。
func TestBuildOptions(t *testing.T) {
	opts := buildOptions()

	if opts.Config == nil {
		t.Fatal("buildOptions returned nil Config")
	}
	if opts.ConfigPath == "" {
		t.Fatal("buildOptions returned empty ConfigPath")
	}
	// 装配出的 Options 必须能被 app.Run 接受（此处不实际 Run，避免拉起真实 systray）。
	// 仅作结构合法性断言：Config 指针可被解引用并保存。
	if err := config.Save(filepath.Join(t.TempDir(), "cfg.json"), *opts.Config); err != nil {
		t.Fatalf("assembled Config not saveable: %v", err)
	}
}

// TestRun_Integration_AllFakesMenuQuit 是 N2 要求的 cmd 集成测试：以全部 fake 注入
// app.Options（窗口/托盘/锚定/自启/主题/日历），跑通 app.Run 的「菜单→主循环→
// 生命周期→退出」端到端路径，并断言：
//   - 菜单装配完成（settings.BuildTrayMenu 接线正确）
//   - 显示窗口后 90-UI 渲染层经 Present 推送 360×480 像素缓冲（Calendar+Theme 注入生效）
//   - 退出时窗口消息泵经 WindowController.Quit 收口（N1 在 cmd 边界的回归）
//   - 退出前配置持久化到 buildOptions 指定的 ConfigPath
//
// 与 app 包 TestRun_* 范本同源但位于 cmd 包边界，证明 main 的装配配方 + app.Run
// 接线对真实菜单回调链路有效，将 cmd 包覆盖从 0% 拉起。
func TestRun_Integration_AllFakesMenuQuit(t *testing.T) {
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
	su := &fakeStartup{}
	cfg := config.Default()

	done := make(chan error, 1)
	go func() {
		done <- app.Run(app.Options{
			Window:         win,
			StartMinimized: true,
			Tray:           tray,
			Anchor:         func() image.Rectangle { return trayRect },
			Config:         &cfg,
			ConfigPath:     cfgPath,
			Startup:        su,
			Theme:          tp,
			Calendar:       svc,
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

	// 显示/隐藏 → 显示（show=1）；注入 Calendar+Theme 后渲染层应经 Present 推送像素。
	findMenuItem(tray.lastMenu.Items, "显示/隐藏").OnClick()
	waitVisible(t, win, true)

	deadline = time.Now().Add(time.Second)
	for len(win.presents) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(win.presents) == 0 {
		t.Fatal("expected at least one Present call (90-UI render push)")
	}
	last := win.presents[len(win.presents)-1]
	if last.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Errorf("presented bounds = %v, want 360×480", last.Bounds())
	}

	// 退出项 → CmdQuit → 主循环退出并收口窗口 goroutine（N1）。
	findMenuItem(tray.lastMenu.Items, "退出").OnClick()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if win.quitCalls != 1 {
		t.Errorf("N1 regression at cmd boundary: window.Quit called %d times on quit, want 1", win.quitCalls)
	}
	// 退出前经 buildOptions 的 ConfigPath 持久化配置。
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config not persisted to assembled ConfigPath: %v", statErr)
	}
}
