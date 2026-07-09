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

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/shell"
)

// fakeWindow 观察生命周期对窗口的调用（与 win32.fakeWindow 类似，但本包测试自包含）。
type fakeWindow struct {
	visible    bool
	anchorRect image.Rectangle
	showCalls  int
	hideCalls  int
}

func (w *fakeWindow) Show()                              { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                              { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                     { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }

var _ shell.WindowController = (*fakeWindow)(nil)

// fakeTray 模拟托盘：Run 时按序把注入的命令推入 cmdCh，随后阻塞至 ctx 取消
// （模拟 systray 消息泵常驻）；若 quitCh 被关闭则追加一次 CmdQuit 以便测试可控退出。
type fakeTray struct {
	cmds   []platform.TrayCommand
	click  func()
	bounds image.Rectangle
	quitCh chan struct{}
}

func (t *fakeTray) SetIcon([]byte) error { return nil }
func (t *fakeTray) SetTooltip(string)    {}
func (t *fakeTray) OnClick(fn func())    { t.click = fn }
func (t *fakeTray) Bounds() (int, int, int, int) {
	return t.bounds.Min.X, t.bounds.Min.Y, t.bounds.Dx(), t.bounds.Dy()
}
func (t *fakeTray) Remove() error { return nil }
func (t *fakeTray) Run(ctx context.Context, cmdCh chan<- platform.TrayCommand) error {
	for _, c := range t.cmds {
		select {
		case cmdCh <- c:
		case <-ctx.Done():
			return nil
		}
	}
	select {
	case <-t.quitCh:
		select {
		case cmdCh <- platform.CmdQuit:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
	return nil
}

var _ platform.TrayManager = (*fakeTray)(nil)

// TestRun_ToggleThenQuit 验证双循环装配：托盘命令经 channel → 主循环 →
// lifecycle.Handle → 窗口；退出前持久化配置。
func TestRun_ToggleThenQuit(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	trayRect := image.Rect(100, 900, 132, 932) // 托盘图标矩形（物理像素）
	win := &fakeWindow{}
	tray := &fakeTray{
		cmds:   []platform.TrayCommand{platform.CmdToggle, platform.CmdToggle, platform.CmdQuit},
		bounds: trayRect,
	}

	err := Run(Options{
		Window:     win,
		Tray:       tray,
		Anchor:     func() image.Rectangle { return trayRect },
		Config:     config.Default(),
		ConfigPath: cfgPath,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// 两次切换：显示(show=1) → 隐藏(hide=1)；退出再隐藏(hide=2)。
	if win.showCalls != 1 {
		t.Errorf("showCalls = %d, want 1", win.showCalls)
	}
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

// TestRun_LeftClickToggles 验证左键 OnClick 回调向 cmdCh 推送 CmdToggle，
// 使主循环驱动窗口显示。测试以 goroutine 运行 Run，手动模拟单击后再受控退出。
func TestRun_LeftClickToggles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	trayRect := image.Rect(50, 800, 82, 832)

	win := &fakeWindow{}
	tray := &fakeTray{bounds: trayRect, quitCh: make(chan struct{})}

	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Window:     win,
			Tray:       tray,
			Anchor:     func() image.Rectangle { return trayRect },
			Config:     config.Default(),
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
	for !win.Visible() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !win.Visible() {
		t.Errorf("after left click, window visible = %v, want true", win.Visible())
	}

	// 受控退出。
	close(tray.quitCh)
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestDefaultIcon 验证内置图标生成合法 32×32 PNG。
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
