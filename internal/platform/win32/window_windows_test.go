//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/theme"
	"github.com/shaolei/DeskCalendar/internal/ui"
)

// TestWin32Window_AnchorAndPresentReachWindowThread 验证 AnchorAboveTray/Present 经
// SendMessage→wndProc 真正到达窗口线程（B1 回归防护）。绕过全部 fake，是 ADR-08 行动项
// #5 要求的真机烟测：先前「像素验收」走 ui.Render 直出 PNG + app 注入 fakeWindow，从未
// 经过真实 win32Window 的跨线程通道，故漏掉 B1。
//
// SendMessageW 同步——调用返回前窗口线程已处理完消息，断言无需 sleep。若本环境无法创建
// 窗口（hwnd==0，如无交互窗口站的服务会话），则 Skip，不误报。
func TestWin32Window_AnchorAndPresentReachWindowThread(t *testing.T) {
	w := newNativeWindow(Options{Width: 360, Height: 480, Margin: 8})
	if w == nil {
		t.Fatal("newNativeWindow returned nil")
	}
	wc, ok := w.(*win32Window)
	if !ok {
		t.Fatalf("expected *win32Window, got %T", w)
	}
	if wc.hwnd == 0 {
		t.Skip("window creation unavailable in this environment (no interactive window station); cannot exercise real win32 path")
	}
	defer func() {
		sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
		<-wc.done // 等窗口线程彻底退出、GDI 释放，避免与后续测试/调用竞争
	}()

	// AnchorAboveTray：同步 SendMessage 后 wndProc 应已写入 lastTray。
	tray := image.Rect(100, 800, 140, 820)
	wc.AnchorAboveTray(tray)
	if wc.lastTray == nil {
		t.Fatal("B1 regression: AnchorAboveTray did not reach window thread (lastTray nil)")
	}
	if *wc.lastTray != tray {
		t.Errorf("lastTray = %v, want %v", *wc.lastTray, tray)
	}

	// Present：同步 SendMessage 后 wndProc 应已写入 lastBmp。
	bmp := image.NewRGBA(image.Rect(0, 0, 360, 480))
	for i := 0; i < len(bmp.Pix); i += 4 {
		bmp.Pix[i], bmp.Pix[i+1], bmp.Pix[i+2], bmp.Pix[i+3] = 12, 34, 56, 255
	}
	wc.Present(bmp)
	if wc.lastBmp != bmp {
		t.Fatal("B1 regression: Present did not reach window thread (lastBmp not set)")
	}
}

// TestWin32Window_RenderAndPresentFullPipeline 端到端真机烟测：用真实 ui.Render 渲染 2026-07
// 日历（含 S2 的周一表头旋转）→ Present 到真实窗口 → 断言窗口 DIB 已被绘入（不再是中性灰底）。
// 这把 B1（跨线程推送）与 S2（渲染层）在真实窗口线程上串起来验证。
func TestWin32Window_RenderAndPresentFullPipeline(t *testing.T) {
	w := newNativeWindow(Options{Width: 360, Height: 480, Margin: 8})
	wc, ok := w.(*win32Window)
	if !ok {
		t.Fatalf("expected *win32Window, got %T", w)
	}
	if wc.hwnd == 0 {
		t.Skip("window creation unavailable in this environment (no interactive window station); cannot exercise real win32 path")
	}
	defer func() {
		sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
		<-wc.done // 等窗口线程彻底退出、GDI 释放，避免与后续测试/调用竞争
	}()

	grid := calendar.GenMonthGrid(2026, time.July, calendar.GridOptions{
		WeekStart: time.Monday,
		Today:     time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local),
		Selected:  time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local),
	})
	th := &theme.Theme{
		Name:    "test",
		Scheme:  theme.SchemeLight,
		Palette: smokePalette(),
		Alpha:   1,
	}
	bmp := ui.Render(ui.NewMonthModel(grid, true, true), ui.RenderOptions{Width: 360, Height: 480}, th)

	wc.Present(bmp)
	if wc.lastBmp != bmp {
		t.Fatal("Present did not reach window thread (lastBmp not set)")
	}
	// DIB 应已被日历位图覆盖（中性灰底 250,251,252 不再全占）。
	if dibUniformNeutral(wc) {
		t.Error("calendar bitmap was not blitted into the real window DIB (still neutral filler)")
	}
}

// TestWin32Window_S3_ActivateGuard 回归 S3：窗口显示后若未真正抢到前台，系统会先投递
// WM_ACTIVATE(WA_INACTIVE)，旧实现会立即把自己隐藏（点托盘「闪一下就没了」）。修复后：
// wmUserShow 显式 SetForegroundWindow 抢前台；且仅当窗口此前确实被激活过（activated==1），
// 收到 WA_INACTIVE 才自隐藏。本测试直接用 wndProc 模拟消息序列，验证：
//  1. Show 后若立刻收到 WA_INACTIVE（未激活）不隐藏 —— S3 核心防护；
//  2. 用户点开（WA_ACTIVE）后失焦（WA_INACTIVE）才隐藏 —— 点击外部关闭仍可用；
//  3. 再次 Show 重置 activated，重复 1)。
func TestWin32Window_S3_ActivateGuard(t *testing.T) {
	w := newNativeWindow(Options{Width: 360, Height: 480, Margin: 8})
	wc, ok := w.(*win32Window)
	if !ok {
		t.Fatalf("expected *win32Window, got %T", w)
	}
	if wc.hwnd == 0 {
		t.Skip("window creation unavailable in this environment (no interactive window station); cannot exercise real win32 path")
	}
	defer func() {
		sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
		<-wc.done
	}()

	// 1) 显示后立即收到 WA_INACTIVE（未激活）—— 不得隐藏。
	wc.wndProc(uintptr(wc.hwnd), wmUserShow, 0, 0)
	if !wc.Visible() {
		t.Fatal("S3: window not visible right after Show")
	}
	wc.wndProc(uintptr(wc.hwnd), wmActivate, waInactive, 0)
	if !wc.Visible() {
		t.Error("S3 regression: window hidden by premature WA_INACTIVE before any activation (flash-and-gone)")
	}

	// 2) 用户点开（WA_ACTIVE）→ 再点外部失焦（WA_INACTIVE）→ 应隐藏（点击外部关闭）。
	wc.wndProc(uintptr(wc.hwnd), wmActivate, waActive, 0)
	if wc.activated.Load() != 1 {
		t.Fatal("S3: activated flag not set after WA_ACTIVE")
	}
	wc.wndProc(uintptr(wc.hwnd), wmActivate, waInactive, 0)
	if wc.Visible() {
		t.Error("S3: window should hide on focus loss after being activated (click-away dismiss broken)")
	}

	// 3) 再次显示：重置 activated，且未激活前的 WA_INACTIVE 仍不隐藏。
	wc.wndProc(uintptr(wc.hwnd), wmUserShow, 0, 0)
	if !wc.Visible() {
		t.Fatal("S3: window not visible after re-Show")
	}
	if wc.activated.Load() != 0 {
		t.Error("S3: activated flag not reset on re-Show")
	}
	wc.wndProc(uintptr(wc.hwnd), wmActivate, waInactive, 0)
	if !wc.Visible() {
		t.Error("S3 regression: re-Show window hidden by premature WA_INACTIVE")
	}
}

// TestApplyVisualPolish_DWMRoundCorner 验证 #147 圆角 plumbing：applyVisualPolish 以正确的
// 参数调用 DwmSetWindowAttribute（DWMWA_WINDOW_CORNER_PREFERENCE=33, 值=DWMWCP_ROUND, cb=4），
// 且仅在 hwnd 非零时触发。通过注入 dwmSetWindowAttribute seam（同 S5 的 deleteObject 手法）
// 记录调用，无需真实窗口即可在任意平台确定性跑通。
//
// 注：DWM pref 指针经 uintptr 传入，测试内不反向重建 *uint32（vet 的 unsafeptr 规则会报
// "possible misuse of unsafe.Pointer"）。pref 值由生产代码的常量 dwmwcpRound 保证（下面直接
// 断言该常量 == DWMWCP_ROUND），seam 侧仅校验「指针非空 + 属性/尺寸正确」。
func TestApplyVisualPolish_DWMRoundCorner(t *testing.T) {
	var gotHWND, gotAttr, gotSize, gotPrefPtr uintptr
	var calls int
	real := dwmSetWindowAttribute
	dwmSetWindowAttribute = func(args ...uintptr) (uintptr, uintptr, error) {
		calls++
		gotHWND = args[0]
		gotAttr = args[1]
		gotPrefPtr = args[2]
		gotSize = args[3]
		return 0, 0, nil
	}
	defer func() { dwmSetWindowAttribute = real }()
	// S2：dwmSetWindowAttribute 为包级变量，本注入期间禁止本包其他测试以 t.Parallel() 并行执行，
	// 否则对包级 var 的读写会成数据竞争（与既有的 deleteObject seam 约束一致）。

	// 生产代码常量自检：确为 DWMWCP_ROUND / DWMWA_WINDOW_CORNER_PREFERENCE。
	if dwmwcpRound != 2 {
		t.Fatalf("dwmwcpRound = %d, want 2 (DWMWCP_ROUND)", dwmwcpRound)
	}
	if dwmwaWindowCornerPreference != 33 {
		t.Fatalf("dwmwaWindowCornerPreference = %d, want 33", dwmwaWindowCornerPreference)
	}

	const hwnd = 0xABCDEF
	applyVisualPolish(hwnd)

	if calls != 1 {
		t.Fatalf("applyVisualPolish called DwmSetWindowAttribute %d times, want 1", calls)
	}
	if gotHWND != hwnd {
		t.Errorf("DWM hwnd = %#x, want %#x", gotHWND, hwnd)
	}
	if gotAttr != dwmwaWindowCornerPreference {
		t.Errorf("DWM attribute = %d, want %d (DWMWA_WINDOW_CORNER_PREFERENCE)", gotAttr, dwmwaWindowCornerPreference)
	}
	if gotPrefPtr == 0 {
		t.Error("DWM pref pointer is nil; expected a valid *uint32 (=DWMWCP_ROUND) pointer")
	}
	if gotSize != 4 {
		t.Errorf("DWM cbAttribute = %d, want 4 (sizeof DWORD)", gotSize)
	}

	// hwnd==0 时不触发（防御性守卫）。
	calls = 0
	applyVisualPolish(0)
	if calls != 0 {
		t.Errorf("applyVisualPolish(0) should not call DwmSetWindowAttribute, got %d calls", calls)
	}
}

// smokePalette 一套已知浅色板，便于渲染稳定的日历位图。
func smokePalette() theme.ColorPalette {
	return theme.ColorPalette{
		Background: color.RGBA{247, 247, 247, 255},
		Surface:    color.RGBA{255, 255, 255, 255},
		Foreground: color.RGBA{26, 26, 26, 255},
		Muted:      color.RGBA{154, 160, 166, 255},
		Accent:     color.RGBA{45, 127, 249, 255},
		HolidayRed: color.RGBA{229, 57, 53, 255},
		TodayBlue:  color.RGBA{45, 127, 249, 255},
		Border:     color.RGBA{224, 224, 224, 255},
	}
}

// dibUniformNeutral 判断 DIB 像素是否仍全是中性灰底（250,251,252），即 createDIB 的初始填充。
func dibUniformNeutral(wc *win32Window) bool {
	b := wc.bits
	for i := 0; i < len(b); i += 4 {
		if b[i] != 250 || b[i+1] != 251 || b[i+2] != 252 {
			return false
		}
	}
	return true
}

// TestWin32Window_S5_DIBLifecycle 回归 S5：删除 GDI 位图前必须先把它从 memDC 中顶出，
// 绝不能「删除一个仍被选中的 GDI 对象」——该操作在 Win32 下行为未定义，跨 DPI 反复重建
// 会累积 GDI 句柄泄漏/损坏。
//
// 做法：给 deleteObject 注入 seam，每次删除时断言「被删位图当前不是 memDC 中选中的位图」
// （经 GetCurrentObject(memDC, OBJ_BITMAP) 校验）。随后真实地走一遍生命周期：
//  1. 窗口创建时 run() 已创建首张 DIB（hbmp 非空）；
//  2. 投递 WM_DPICHANGED 触发 createDIB 重建（resize 路径）——旧 DIB 在此被删；
//  3. 投递 WM_DESTROY 让窗口线程退出并 destroy() —— DIB + memDC 在此被删并释放。
//
// 任一处出现「删除仍被选中的位图」seam 立即报错。
func TestWin32Window_S5_DIBLifecycle(t *testing.T) {
	w := newNativeWindow(Options{Width: 360, Height: 480, Margin: 8})
	wc, ok := w.(*win32Window)
	if !ok {
		t.Fatalf("expected *win32Window, got %T", w)
	}
	if wc.hwnd == 0 {
		t.Skip("window creation unavailable in this environment (no interactive window station); cannot exercise real win32 path")
	}
	if wc.hbmp == 0 || wc.memDC == 0 {
		t.Fatal("S5 precondition: initial DIB not created on window creation")
	}
	defer func() {
		sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
		<-wc.done // 等窗口线程彻底退出、GDI 释放
	}()

	// 安装 seam：断言每次 deleteObject 的对象都「已不在 memDC 中被选中」。
	getCurrentObject := gdi32.NewProc("GetCurrentObject")
	const objBitmap = 7 // OBJ_BITMAP
	realDelete := deleteObject
	deleteObject = func(args ...uintptr) (uintptr, uintptr, error) {
		hgdiobj := args[0]
		cur, _, _ := getCurrentObject.Call(wc.memDC, objBitmap)
		if cur != 0 && cur == hgdiobj {
			t.Errorf("S5 footgun: deleteObject called on a bitmap STILL selected into memDC (undefined behavior)")
		}
		return realDelete(hgdiobj)
	}
	defer func() { deleteObject = realDelete }()

	// 2) 触发 DPI 变化 → createDIB 重建 DIB（resize 路径，旧位图在此被删）。
	//    直接驱动 wndProc（与 S3 测试同风格），避免与窗口线程消息泵竞争；createDIB
	//    在 wmDpiChanged 分支内同步执行，dibW 立即更新。
	const newDPI = 144
	wc.wndProc(uintptr(wc.hwnd), wmDpiChanged, uintptr(newDPI<<16), 0)
	if wc.hbmp == 0 {
		t.Fatal("S5: DIB not recreated after DPI change")
	}
	wantW := scaleLogical(360, newDPI) // 360*144/96 = 540
	if wc.dibW != wantW {
		t.Errorf("S5: dibW after DPI change = %d, want %d", wc.dibW, wantW)
	}

	// 3) 销毁：窗口线程退出后 destroy() 删除 hbmp + memDC（seam 同样覆盖）。
	sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
	<-wc.done
	if wc.hbmp != 0 || wc.memDC != 0 {
		t.Error("S5: GDI resources not released after destroy (hbmp/memDC still set)")
	}
}

// TestWin32Window_N1_QuitStopsMessagePump 回归 N1：窗口消息泵 goroutine 必须在 Quit
// 后彻底退出——否则 app.Run 退出路径只 tray.Remove()+return 时，窗口线程泄漏至进程退出
// （作为库重复使用时累积）。修复后 win32Window.Quit() 经 WM_DESTROY→postQuitMessage 使
// run 的 getMsg 返回 0 而退出循环→destroy()+close(done)，且 Quit() 阻塞至 done 关闭。
// 本测试驱动 Quit() 并断言 done 在时限内关闭（窗口线程确已结束）。
func TestWin32Window_N1_QuitStopsMessagePump(t *testing.T) {
	w := newNativeWindow(Options{Width: 360, Height: 480, Margin: 8})
	wc, ok := w.(*win32Window)
	if !ok {
		t.Fatalf("expected *win32Window, got %T", w)
	}
	if wc.hwnd == 0 {
		t.Skip("window creation unavailable in this environment (no interactive window station); cannot exercise real win32 path")
	}
	// 保险清理：Quit 已收口则 sendMessage 落到无效 hwnd（立即返回）+ done 已关闭（立即收到），
	// 不会死锁；若测试在 Quit 前失败，则正常销毁并等待线程退出。
	defer func() {
		sendMessage.Call(uintptr(wc.hwnd), wmDestroy, 0, 0)
		<-wc.done
	}()

	// 先显示，模拟真实使用路径。
	wc.Show()
	if !wc.Visible() {
		t.Fatal("N1: window not visible after Show")
	}

	// 退出：Quit() 内部 sendMessage(WM_DESTROY) + 阻塞 <-done。用超时包裹，证明窗口线程
	// 确实退出——若仍泄漏（N1 复发），Quit 永不返回，select 超时触发 Fatal。
	quitDone := make(chan struct{})
	go func() { wc.Quit(); close(quitDone) }()
	select {
	case <-quitDone:
		// 窗口线程已退出。
	case <-time.After(3 * time.Second):
		t.Fatal("N1 regression: window message-pump goroutine did not exit after Quit (leak)")
	}

	// done 必须已关闭（Quit 阻塞语义的副产品，亦直接证明 goroutine 退出）。
	select {
	case <-wc.done:
	default:
		t.Error("N1: window done channel not closed after Quit")
	}
}
