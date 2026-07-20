package shell

import (
	"image"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/platform"
)

// fakeWindow 记录 Lifecycle 对窗口的调用，用于断言。
type fakeWindow struct {
	visible    bool
	anchorRect image.Rectangle
	showCalls  int
	hideCalls  int
}

func (w *fakeWindow) Show()                        { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                        { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }
func (w *fakeWindow) Quit()                        {}

// trayRect 是注入的托盘包围盒（物理像素），模拟 tray.Bounds() 的转换结果。
var trayRect = image.Rect(100, 200, 130, 230) // x=100,y=200,w=30,h=30

func newTestLifecycle() (*Lifecycle, *fakeWindow, *int, *int) {
	persistCalls := 0
	quitCalls := 0
	lc := NewLifecycle(
		func() image.Rectangle { return trayRect },
		func() error { persistCalls++; return nil },
		func() { quitCalls++ },
	)
	return lc, &fakeWindow{}, &persistCalls, &quitCalls
}

func TestLifecycle_InitialState(t *testing.T) {
	lc, _, _, _ := newTestLifecycle()
	if got := lc.State(); got != StateBoot {
		t.Fatalf("initial state = %v, want StateBoot", got)
	}
}

func TestLifecycle_CmdToggleShowsWhenHidden(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	lc.Handle(platform.CmdToggle, win)

	if !win.Visible() {
		t.Fatal("after CmdToggle from hidden, window should be visible")
	}
	if win.showCalls != 1 {
		t.Fatalf("Show called %d times, want 1", win.showCalls)
	}
	if win.anchorRect != trayRect {
		t.Fatalf("AnchorAboveTray rect = %+v, want %+v", win.anchorRect, trayRect)
	}
	if lc.State() != StateReady {
		t.Fatalf("state = %v, want StateReady", lc.State())
	}
}

func TestLifecycle_CmdToggleHidesWhenVisible(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	lc.Handle(platform.CmdToggle, win) // show
	lc.Handle(platform.CmdToggle, win) // hide

	if win.Visible() {
		t.Fatal("after second CmdToggle, window should be hidden")
	}
	if win.hideCalls != 1 {
		t.Fatalf("Hide called %d times, want 1", win.hideCalls)
	}
	if lc.State() != StateReady {
		t.Fatalf("state = %v, want StateReady", lc.State())
	}
}

func TestLifecycle_CmdToggleTwiceReturnsToHidden(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	for i := 0; i < 2; i++ {
		lc.Handle(platform.CmdToggle, win)
	}
	if win.Visible() {
		t.Fatal("after two toggles, window should be hidden again")
	}
	if win.showCalls != 1 || win.hideCalls != 1 {
		t.Fatalf("Show=%d Hide=%d, want 1/1", win.showCalls, win.hideCalls)
	}
}

func TestLifecycle_CmdShowAnchorsAndShows(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	lc.Handle(platform.CmdShow, win)

	if !win.Visible() {
		t.Fatal("CmdShow should make window visible")
	}
	if win.anchorRect != trayRect {
		t.Fatalf("AnchorAboveTray rect = %+v, want %+v", win.anchorRect, trayRect)
	}
}

func TestLifecycle_CmdHideWhenVisible(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	lc.Handle(platform.CmdToggle, win) // show
	lc.Handle(platform.CmdHide, win)   // hide

	if win.Visible() {
		t.Fatal("CmdHide should hide the window")
	}
	if lc.State() != StateReady {
		t.Fatalf("state = %v, want StateReady", lc.State())
	}
}

func TestLifecycle_CmdQuitPersistsAndQuits(t *testing.T) {
	lc, win, persistCalls, quitCalls := newTestLifecycle()
	lc.Handle(platform.CmdToggle, win) // visible
	lc.Handle(platform.CmdQuit, win)

	if *persistCalls != 1 {
		t.Fatalf("persist called %d times, want 1", *persistCalls)
	}
	if *quitCalls != 1 {
		t.Fatalf("quit called %d times, want 1", *quitCalls)
	}
	if lc.State() != StateQuit {
		t.Fatalf("state = %v, want StateQuit", lc.State())
	}
}

func TestLifecycle_CmdQuitIsIdempotent(t *testing.T) {
	lc, win, persistCalls, quitCalls := newTestLifecycle()
	lc.Handle(platform.CmdShow, win) // 显示，desiredVisible=true（未显示的窗口退出无需隐藏）
	lc.Handle(platform.CmdQuit, win) // 首次：隐藏窗口 + 持久化 + 退出
	lc.Handle(platform.CmdQuit, win) // 第二次应被忽略
	lc.Handle(platform.CmdToggle, win) // 退出后任何命令都应被忽略

	if *persistCalls != 1 || *quitCalls != 1 {
		t.Fatalf("after idempotent quit: persist=%d quit=%d, want 1/1", *persistCalls, *quitCalls)
	}
	// 首次退出隐藏了已显示的窗口（hideCalls==1）；后续命令不再触碰窗口。
	if win.hideCalls != 1 {
		t.Fatalf("window hideCalls after quit = %d, want 1", win.hideCalls)
	}
	if win.showCalls != 1 {
		t.Fatalf("window showCalls after quit = %d, want 1 (CmdShow 显示一次，退出后 toggle 被忽略)", win.showCalls)
	}
}

// TestLifecycle_ToggleStormConverges 锁定 #151 卡死修复后的「显示/隐藏」一致性：
// 切换决策基于内部 desiredVisible（唯一真相源），与窗口实际态的异步处理无关，快速连续
// 切换必收敛到正确终态，不出现「窗口卡在显示或隐藏」。旧实现读 win.Visible()（caller 乐观
// 原子）做决策，在 PostMessage 异步派发下易与窗口线程实际态错位，导致切换发出错误命令。
func TestLifecycle_ToggleStormConverges(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	for i := 0; i < 20; i++ {
		lc.Handle(platform.CmdToggle, win)
	}
	if win.visible {
		t.Errorf("20 toggles: expected hidden, got visible")
	}
	if win.showCalls != 10 || win.hideCalls != 10 {
		t.Errorf("20 toggles: showCalls=%d hideCalls=%d, want 10/10 (no duplicated same-direction commands)",
			win.showCalls, win.hideCalls)
	}
	// 再切一次 → 显示
	lc.Handle(platform.CmdToggle, win)
	if !win.visible {
		t.Errorf("21 toggles: expected visible, got hidden")
	}
}

// TestLifecycle_AutoHideResync 锁定自动隐藏后生命周期期望可见态的回写：窗口自身「点击外部」
// 关闭（仅改实际态，不经生命周期，模拟 waInactive→wmUserHide）后，须经 NotifyAutoHidden 把
// desiredVisible 回写为 false，使后续托盘「显示/隐藏」切换方向正确。若决策仍依赖 win.Visible()，
// 自动隐藏（仅改实际态）会让下一次切换误判为「已隐藏→再 Hide」，吞掉一次切换（#151 表现）。
func TestLifecycle_AutoHideResync(t *testing.T) {
	lc, win, _, _ := newTestLifecycle()
	lc.Handle(platform.CmdShow, win) // 期望可见态=true，窗口显示
	if !win.visible {
		t.Fatal("CmdShow should show window")
	}
	// 窗口自身「点击外部」隐藏（模拟 waInactive→wmUserHide，不经生命周期）。
	win.Hide()
	// 窗口通知生命周期：我已隐藏。
	lc.NotifyAutoHidden()
	// 托盘「显示/隐藏」切换：基于 desiredVisible=false → 应 Show（而非再 Hide）。
	lc.Handle(platform.CmdToggle, win)
	if !win.visible {
		t.Errorf("after auto-hide + NotifyAutoHidden + toggle: expected shown, got hidden")
	}
}
