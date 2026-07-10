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
	lc.Handle(platform.CmdQuit, win) // 首次：隐藏窗口 + 持久化 + 退出
	lc.Handle(platform.CmdQuit, win) // 第二次应被忽略
	lc.Handle(platform.CmdToggle, win) // 退出后任何命令都应被忽略

	if *persistCalls != 1 || *quitCalls != 1 {
		t.Fatalf("after idempotent quit: persist=%d quit=%d, want 1/1", *persistCalls, *quitCalls)
	}
	// 首次退出隐藏了窗口（hideCalls==1）；后续命令不再触碰窗口。
	if win.hideCalls != 1 {
		t.Fatalf("window hideCalls after quit = %d, want 1", win.hideCalls)
	}
	if win.showCalls != 0 {
		t.Fatalf("window showCalls after quit = %d, want 0 (toggle ignored)", win.showCalls)
	}
}
