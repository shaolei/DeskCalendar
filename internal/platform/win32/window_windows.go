//go:build windows

package win32

import (
	"context"
	"fmt"
	"image"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
	"github.com/shaolei/DeskCalendar/internal/platform"
)


// ---- Win32 常量 -------------------------------------------------------------
const (
	wsPopup        = 0x80000000
	wsExTopMost    = 0x00000008
	wsExToolWindow = 0x00000080

	// CS_DROPSHADOW：窗口类样式，使 tool window（WS_EX_TOOLWINDOW）显示柔和投影。
	// 作为 DWM 默认阴影（被 WS_EX_TOOLWINDOW 抑制）的轻量替代，不引入分层窗、零 CGO（#147 可选阴影）。
	// 注意：实际投影还取决于用户的系统设置（「在窗口下显示阴影」，由 SystemParametersInfo
	// SPI_SETDROPSHADOW 控制，默认开启）。部分精简版/高对比度主题下可能不显示——属「可选阴影」
	// 的合理边界，非缺陷（N1）。
	csDropShadow = 0x00020000

	// DWM 视觉润色（#147 v1.1 Win11 系统圆角）。
	dwmwaWindowCornerPreference = 33 // DWMWA_WINDOW_CORNER_PREFERENCE
	dwmwcpRound                 = 2  // DWMWCP_ROUND

	swShow = 5
	swHide = 0

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmPaint       = 0x000F
	wmActivate    = 0x0006
	wmKeyDown     = 0x0100
	wmChar        = 0x0102 // 字符消息（TranslateMessage 由 WM_KEYDOWN 翻译而来）
	wmLButtonDown = 0x0201 // 客户区左键按下（#113 点击命中测试入口）
	wmDpiChanged  = 0x02E0

	// 虚拟键码（用于 wmKeyDown 的 wparam，驱动 OnKey 回调）。
	vkReturn = 0x0D // Enter：提交待办草稿
	vkBack   = 0x08 // Backspace：删除草稿末字符
	vkTab    = 0x09 // Tab：切换日历/待办视图
	vkDelete = 0x2E // Delete：删除选中待办

	// 自定义消息：由控制器方法派发到窗口线程执行。Show/Hide/Quit/Anchor/Present
	// 全部用 PostMessage 异步派发——主循环绝不因窗口操作而同步阻塞，这是 #151 卡死
	// 回归的根治（同步 SendMessage 在窗口线程正处理 setForegroundWindow 等会触发系统
	// 同步消息的上下文时，会形成跨线程死锁，致显示/隐藏/退出同时失效）。
	wmUserShow    = 0x0400 + 1
	wmUserHide    = 0x0400 + 2
	wmUserAnchor  = 0x0400 + 3
	wmUserPresent = 0x0400 + 4

	waInactive    = 0
	waActive      = 1
	waClickActive = 2

	asfwAny = 0xFFFFFFFF // ASFW_ANY：允许任意进程设置前台窗口（S3 抢前台用）

	vkEscape = 0x1B

	monitorDefaultToNearest = 0x00000002

	srccopy = 0x00CC0020
)

// ---- Win32 结构体（字段顺序 = ABI 布局）------------------------------------
type point struct {
	X, Y int32
}

type rect32 struct {
	Left, Top, Right, Bottom int32
}

type wndClassexW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect32
	RcWork    rect32
	DwFlags   uint32
}

// paintStruct 对应 Win32 PAINTSTRUCT（BeginPaint/EndPaint 的 in/out 参数）。
// 字段顺序与 ABI 布局严格对齐：HDC(8) | BOOL fErase(4) | RECT rcPaint(16) |
// BOOL fRestore(4) | BOOL fIncUpdate(4) | BYTE rgbReserved[32]。Go 按 8 字节
// 对齐补到 72，与真实 sizeof(PAINTSTRUCT) 一致，避免 BeginPaint 越界写入。
type paintStruct struct {
	Hdc         windows.Handle
	fErase      int32
	RcPaint     rect32
	fRestore    int32
	fIncUpdate  int32
	RgbReserved [32]byte
}

// ---- 懒加载 native procs（包级，仅加载一次）-------------------------------
var (
	user32   = windows.NewLazyDLL("user32.dll")
	gdi32    = windows.NewLazyDLL("gdi32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")
	dwmapi   = windows.NewLazyDLL("dwmapi.dll") // #147：Win11 DWM 圆角（零 CGO）

	regClassEx               = user32.NewProc("RegisterClassExW")
	createWindowEx           = user32.NewProc("CreateWindowExW")
	showWindow               = user32.NewProc("ShowWindow")
	setWindowPos             = user32.NewProc("SetWindowPos")
	setForegroundWindow      = user32.NewProc("SetForegroundWindow")
	allowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	getMsg                   = user32.NewProc("GetMessageW")
	translateMsg             = user32.NewProc("TranslateMessage")
	dispatchMsg              = user32.NewProc("DispatchMessageW")
	defWndProc               = user32.NewProc("DefWindowProcW")
	postQuitMsg              = user32.NewProc("PostQuitMessage")
	destroyWindow            = user32.NewProc("DestroyWindow")
	loadCursor               = user32.NewProc("LoadCursorW")
	getModuleHandle          = kernel32.NewProc("GetModuleHandleW")
	getCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	sendMessage              = user32.NewProc("SendMessageW")
	postMessage              = user32.NewProc("PostMessageW")
	beginPaint              = user32.NewProc("BeginPaint")
	endPaint                = user32.NewProc("EndPaint")
	invalidateRect          = user32.NewProc("InvalidateRect")
	monitorFromPointProc     = user32.NewProc("MonitorFromPoint")
	getMonitorInfo           = user32.NewProc("GetMonitorInfoW")
	// 可靠抢前台（#151 显示/隐藏卡死修复后的「点击外部关不掉」根因修复）：
	// 后台线程直接 SetForegroundWindow 常被系统拒绝（仅显示不激活），需经
	// AttachThreadInput 把本线程输入队列临时挂到前台窗口线程才能生效。
	getForegroundWindow      = user32.NewProc("GetForegroundWindow")
	getWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	attachThreadInput        = user32.NewProc("AttachThreadInput")

	idcArrow           = 32512
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	// deleteObject 删除 GDI 对象。定义为 func 变量以便测试注入 seam，断言「删除前对象
	// 必须已从 memDC 顶出」—— S5 的核心不变量（绝不能删除仍被选中的 GDI 对象）。
	deleteObject = gdi32.NewProc("DeleteObject").Call
	bitBlt       = gdi32.NewProc("BitBlt")

	// dwmSetWindowAttribute 设置为 func 变量（同 deleteObject 的 seam 手法），便于测试注入。
	// 签名：func(a ...uintptr) (r1, r2 uintptr, lastErr error)（来自 LazyProc.Call 的方法值）。
	// 注意：dwmSetWindowAttribute 为包级变量，注入 seam（测试）期间禁止并行测试（t.Parallel），
	// 否则对包级 var 的读写会成数据竞争（与既有的 deleteObject seam 约束一致）。
	dwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute").Call
)

// classSeq 为每个窗口实例分配唯一类名序号，避免多个 win32Window 共用同一
// RegisterClassExW 类名导致 wndProc 槽被首个实例占用（S6：第二窗口消息误派发到
// 第一窗口，且其 DIB 已释放 → 崩溃）。
var classSeq int64

// logger 用于视觉润色（#147）的可观测诊断。DWM 调用失败时记录 Debug 级日志，
// 便于一线排查；成功路径（err==nil）不输出。
var logger = log.New()

// win32Window 是 WindowController 的真实实现（自拥普通弹窗）。
type win32Window struct {
	opts   Options
	margin int

	hwnd  windows.Handle // 仅由窗口线程（run）写入，shell 经 SendMessage 读取
	memDC uintptr
	hbmp  uintptr
	// origBmp 是 memDC 创建时默认选中的 1x1 位图。删除 hbmp 前必须先 selectObject 它
	// 回来把 DIB 从 memDC 中「顶出」，否则「删除仍被选中的 GDI 对象」在 Win32 下行为
	// 未定义（S5 修复的核心）。
	origBmp uintptr
	bits    []byte // DIB 像素（BGRA），指向 bitsPtr
	// dibW/dibH 当前 DIB 物理像素尺寸。窗口线程在创建/换屏(WM_DPICHANGED)时写入；
	// app 主循环经 PhysicalSize() 读取以反算渲染 Scale（#41）。跨 goroutine 读写，
	// 故用 atomic 规避数据竞争（本仓禁用 -race，靠原子化读写兜底，S1 范畴）。
	dibW atomic.Int64
	dibH atomic.Int64

	lastBmp *image.RGBA // 最近一次 Present 的缓冲（DPI 变化时重绘用）
	visible atomic.Int32
	// activated 标记窗口本次显示后是否确实被激活过（用户点开）。WM_ACTIVATE 收到
	// WA_INACTIVE 时，仅当 activated==1 才自隐藏——区分「SW_SHOW 后未抢到前台、首个
	// WM_ACTIVATE 即 WA_INACTIVE」与「用户点开后又点外部导致失焦」，避免「闪一下就没了」（S3）。
	activated atomic.Int32
	// shownAt 是最近一次 wmUserShow(显示) 的单调时刻。与 activated 配合实现「点击外部
	// 关闭」：显示后 250ms 内的 WA_INACTIVE 视为 SW_SHOW 后原前台 reclaim 的假失焦（不隐藏，
	// 防 S3）；稳定显示超过 250ms 后失焦则隐藏——覆盖「SetForegroundWindow 抢前台失败、
	// 窗口从未激活（activated 恒 0）」的托盘程序常见情形（#151）。
	shownAt time.Time

	// 跨线程传递的数据（经 SendMessage 同步派发，原子指针避免 unsafe.Pointer 传递）。
	pendingRect atomic.Pointer[image.Rectangle]
	pendingBmp  atomic.Pointer[image.RGBA]

	// 仅窗口线程访问：最近一次锚定的托盘矩形（DPI 变化时用于重新锚定）。
	lastTray *image.Rectangle

	// dpi 窗口创建时的有效 DPI（创建期由 DPIScaler 取得并固化）。用于将
	// WM_LBUTTONDOWN 的物理像素坐标反算为 ui 的逻辑坐标（#113 命中测试）。
	dpi int
	// onClick 左键点击回调（app 主循环注册），在 wndProc(WM_LBUTTONDOWN) 内调用；
	// 仅经 channel 向主循环投递坐标，不在此直改业务状态（ADR-02 双循环铁律）。
	// 用 atomic.Pointer 持有：OnClick 在主 goroutine 注册（晚于 go w.run()），
	// wndProc 在窗口线程读取；本仓禁用 -race，靠原子指针规避跨 goroutine 数据竞争（S1）。
	onClick atomic.Pointer[func(int, int)]

	// onChar 字符输入回调（#148 待办输入框）：wndProc(WM_CHAR) 内调用，把录入的
	// rune 经主循环投递（不直接改业务状态，S1 单写者）。同样用 atomic.Pointer 持有。
	onChar atomic.Pointer[func(rune)]
	// onKey 功能键回调（#148）：Enter/Backspace/Tab/Delete 等非字符键经此投递到
	// 主循环处理（切视图 / 提交草稿 / 删除待办）。atomic.Pointer 持有规避竞争。
	onKey atomic.Pointer[func(int)]

	// onDPIChanged DPI 变更回调（#41 高 DPI 重渲）。由 app 主循环注册，仅经 channel
	// 向主循环投递「需重渲」信号（不在此直改业务状态，S1 单写者）；窗口线程在
	// WM_DPICHANGED 重建 DIB 后调用。atomic.Pointer 持有规避竞争。
	onDPIChanged atomic.Pointer[func()]
	// onDismissed 窗口「点击外部自动隐藏」(waInactive) 后的回调（由 app 注册，仅经
	// channel 向主循环投递信号，不在此直改业务状态，S1 单写者）。窗口线程调用时只做
	// 非阻塞 channel 发送，与 onClick/onChar/onKey 同模式。用于把自动隐藏同步回生命周期
	// 的期望可见态（#151 显示/隐藏卡死修复：异步 PostMessage 下 win.Visible() 不再可信
	// 为决策依据，生命周期改以 desiredVisible 为准，此处保证自动隐藏也被状态机感知）。
	onDismissed atomic.Pointer[func()]

	// done 在 run() 退出（destroy 后）关闭，供调用方等待消息泵 goroutine 完全结束，
	// 避免窗口/ GDI 资源在测试或退出路径上被并发复用（N1 范畴的清理兜底）。
	done chan struct{}
}

// compile-time 接口满足性校验（仅 Windows 编译单元，win32Window 于此定义）。
var _ WindowController = (*win32Window)(nil)

// scaleLogical 逻辑坐标(96 DPI 基准)→物理像素（四舍五入）。
func scaleLogical(logical, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return int(float64(logical*dpi)/96.0 + 0.5)
}

// newNativeWindow 构造真实弹窗。窗口创建与其消息泵运行在专属 goroutine（窗口线程），
// 所有窗口操作经消息派发到该线程（Show/Hide/Quit/Anchor/Present 一律 PostMessage 异步，
// 主循环绝不因窗口操作同步阻塞，根治 #151 跨线程死锁），满足双循环铁律
// （主 goroutine 发起，窗口线程执行）。
func newNativeWindow(opts Options) WindowController {
	if opts.Width <= 0 {
		opts.Width = 360
	}
	if opts.Height <= 0 {
		opts.Height = 480
	}
	if opts.Margin <= 0 {
		opts.Margin = 8
	}
	w := &win32Window{opts: opts, margin: opts.Margin, done: make(chan struct{})}

	// 进程早期声明 DPI 感知（PerMonitorV2）。
	scaler := platform.NewDPIScaler()
	_ = scaler.SetAwareness(context.Background(), platform.DefaultAwareness())
	dpi, _, _ := scaler.EffectiveDPI()
	w.dpi = dpi
	w.dibW.Store(int64(scaleLogical(opts.Width, dpi)))
	w.dibH.Store(int64(scaleLogical(opts.Height, dpi)))

	ready := make(chan error, 1)
	go w.run(ready)
	<-ready
	return w
}

// run 在窗口线程：创建窗口 + DIB，随后进入消息泵。仅在进程退出（DestroyWindow）
// 或 WM_DESTROY 时退出循环。
func (w *win32Window) run(ready chan<- error) {
	// 唯一类名（S6）：每实例独立类名，确保 wndProc 槽归属本实例，多窗口不串。
	className, _ := windows.UTF16PtrFromString(fmt.Sprintf("DeskCalendarWin32_%d", atomic.AddInt64(&classSeq, 1)))
	hInst, _, _ := getModuleHandle.Call(0)
	hCursor, _, _ := loadCursor.Call(0, uintptr(idcArrow))

	wcex := wndClassexW{
		Size:      uint32(unsafe.Sizeof(wndClassexW{})),
		Style:     csDropShadow, // #147 可选阴影：类样式投影（tool window 才显示）
		WndProc:   windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr { return w.wndProc(hwnd, msg, wparam, lparam) }),
		Instance:  windows.Handle(hInst),
		Cursor:    windows.Handle(hCursor),
		ClassName: className,
	}
	regClassEx.Call(uintptr(unsafe.Pointer(&wcex)))

	hwnd, _, _ := createWindowEx.Call(
		wsExTopMost|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		0, // lpWindowName
		wsPopup,
		0, 0,
		uintptr(w.dibW.Load()), uintptr(w.dibH.Load()),
		0, 0, hInst, 0,
	)
	w.hwnd = windows.Handle(hwnd)
	// #147 视觉润色：Win11 DWM 系统圆角（纯 DWM 合成、零 CGO、不引入分层窗）。
	applyVisualPolish(uintptr(hwnd))
	w.createDIB(int(w.dibW.Load()), int(w.dibH.Load()))

	ready <- nil // 此后 shell 才可安全调用 Show/Hide（happens-before 同步）

	var m msg
	for {
		ret, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 { // WM_QUIT
			break
		}
		if m.Message != wmPaint {
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
	w.destroy()
	close(w.done)
}

// applyVisualPolish 应用 v1.1 视觉润色（#147）：Win11 DWM 系统圆角 + 轻量阴影。
//
//   - 圆角：DwmSetWindowAttribute(DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND)。纯 DWM 合成，
//     零 CGO、不引入 WS_EX_LAYERED / 每像素 alpha（与 ADR-08 一致）。Win10 下该 attribute 未被
//     识别（返回 E_INVALIDARG）调用被忽略，方角窗口无回归；DPI Per-Monitor V2 下圆角由 DWM
//     自动随缩放合成，无需额外处理。
//   - 阴影：由窗口类 CS_DROPSHADOW 实现（见 run() 的 wcex.Style）。CS_DROPSHADOW 仅在
//     WS_EX_TOOLWINDOW 窗口上显示投影，本弹窗恰为 tool window，故得柔和阴影且保留 Alt-Tab 隐藏，
//     不引入分层窗。
//
// 无副作用失败路径：DWM 不可用（LazyDLL 解析失败 / 返回错误）时静默忽略，窗口仍是可用方角弹窗。
func applyVisualPolish(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	pref := uint32(dwmwcpRound)
	// 保留「忽略失败」语义：DWM 不可用（LazyDLL 解析失败 / Win10 返回 E_INVALIDARG）时
	// 静默降级为方角弹窗。仅在 err != nil 时记 Debug 日志，便于一线排查（S1）。
	_, _, err := dwmSetWindowAttribute(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&pref)), 4)
	if err != nil {
		logger.Debug("DwmSetWindowAttribute(corner preference) failed, falling back to square corners: %v", err)
	}
}

// createDIB 创建/重建与窗口同尺寸的 DIBSection，并填充中性底色避免垃圾像素。
//
// S5 修复：CreateDIBSection 后立即 selectObject 把新位图选入 memDC——这一步会把旧位图
// 从 memDC 中「顶出」并以返回值交还。我们再用返回的旧对象决定：
//   - 若 w.hbmp!=0（resize 路径）：old 是上一轮的 DIB，此刻已不再被 memDC 选中 → 安全 deleteObject；
//   - 若 w.hbmp==0（首次）：old 是 memDC 自带的默认 1x1 位图 → 缓存为 origBmp，留作将来删除前的「安全替身」。
//
// 绝不再「删除一个仍被 memDC 选中的位图」——该操作在 Win32 下行为未定义（多数实现延后删除，
// 但跨 DPI 反复重建会累积泄漏/损坏）。
func (w *win32Window) createDIB(width, height int) {
	if w.memDC == 0 {
		dc, _, _ := createCompatibleDC.Call(0)
		w.memDC = dc
	}
	bmi := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(width),
		Height:      int32(-height), // 负高 = 自上而下（首行即顶部）
		Planes:      1,
		BitCount:    32,
		Compression: 0, // BI_RGB
	}
	var bitsPtr unsafe.Pointer
	hbmp, _, _ := createDIBSection.Call(w.memDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bitsPtr)), 0, 0)

	// 关键：先把新位图选入 memDC，旧位图被「顶出」并作为旧选择返回。
	old, _, _ := selectObject.Call(w.memDC, hbmp)
	if w.hbmp != 0 {
		// resize 路径：old 是上一轮的 DIB，此刻已不再被 memDC 选中 → 安全删除。
		deleteObject(old)
	} else if w.origBmp == 0 {
		// 首次：old 是 memDC 自带默认 1x1 位图 → 缓存为删除 hbmp 前的安全替身。
		w.origBmp = old
	}
	w.hbmp = hbmp
	w.dibW.Store(int64(width))
	w.dibH.Store(int64(height))
	n := width * height * 4
	w.bits = (*[1 << 30]byte)(bitsPtr)[:n:n]
	for i := 0; i < n; i += 4 {
		w.bits[i], w.bits[i+1], w.bits[i+2], w.bits[i+3] = 250, 251, 252, 255
	}
}

// destroy 释放 GDI 资源（进程退出或窗口销毁时调用）。
//
// S5 修复：删除 hbmp 前先 selectObject(memDC, origBmp) 把 DIB 从 memDC 中顶出，避免
// 「删除仍被选中的 GDI 对象」这一未定义行为；随后再 deleteDC。
func (w *win32Window) destroy() {
	if w.hbmp != 0 {
		if w.origBmp != 0 {
			selectObject.Call(w.memDC, w.origBmp) // 先把 DIB 顶出 memDC
		}
		deleteObject(w.hbmp)
		w.hbmp = 0
	}
	if w.memDC != 0 {
		deleteDC.Call(w.memDC)
		w.memDC = 0
	}
	w.origBmp = 0
}

// wndProc 窗口过程（运行于窗口线程）。
func (w *win32Window) wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if message != wmPaint {
	}
	switch message {
	case wmUserShow:
		showWindow.Call(hwnd, swShow)
		w.visible.Store(1)
		// 可靠抢前台：窗口线程（PostMessage 派发上下文）直接 SetForegroundWindow 常被系统
		// 拒绝（仅显示不激活，返回 0），窗口从未成为前台 → 点击外部不收 WM_ACTIVATE(WA_INACTIVE)
		// → 自动隐藏（点击外部关闭）永不触发，表现为「窗口一直显示、点外部关不掉」（#151 卡死
		// 修复后暴露的隐患）。用 AttachThreadInput 把本线程输入队列临时挂到当前前台窗口线程，
		// SetForegroundWindow 即生效、窗口拿到焦点并收 WA_ACTIVE；操作后立即解除挂载，
		// 避免输入队列长期耦合。激活成功后 activated 经 WA_ACTIVE 置 1，点击外部 → WA_INACTIVE
		// → 自动隐藏可靠生效。
		if fg, _, _ := getForegroundWindow.Call(); fg != 0 {
			fgThread, _, _ := getWindowThreadProcessId.Call(fg, 0)
			selfThread, _, _ := getCurrentThreadId.Call()
			if fgThread != 0 && fgThread != selfThread {
				attachThreadInput.Call(fgThread, selfThread, 1)
				allowSetForegroundWindow.Call(asfwAny)
				setForegroundWindow.Call(hwnd)
				attachThreadInput.Call(fgThread, selfThread, 0)
			}
		}
		w.activated.Store(0)     // 本次显示尚未确认激活，等真实 WA_ACTIVE 置位
		w.shownAt = time.Now()   // 记录显示时刻，供 waInactive 判定「稳定显示后失焦」(#151)
		// 请求整客户区重绘（InvalidateRect 触发 WM_PAINT，与 present 路径一致）。
		// 注意：此处须用 InvalidateRect（请求重绘），误用 ValidateRect 会清除更新区、
		// 导致 WM_PAINT 永不派发、窗口透明无内容（真机首跑暴露）。
		invalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmUserHide:
		// 先置不可见，使随后 showWindow 触发的重入 WM_ACTIVATE(WA_INACTIVE) 在 waInactive
		// 分支被 visible==1 短路，杜绝同步重入死锁（#151 回归修复）。
		w.visible.Store(0)
		showWindow.Call(hwnd, swHide)
		return 0
	case wmUserAnchor:
		if r := w.pendingRect.Load(); r != nil {
			w.anchor(r)
		}
		return 0
	case wmUserPresent:
		if b := w.pendingBmp.Load(); b != nil {
			w.present(b)
		}
		return 0
	case wmPaint:
		// 必须用 BeginPaint/EndPaint 配对处理 WM_PAINT：BeginPaint 返回可用于绘制的
		// 客户区 HDC 并校验（清除）更新区；EndPaint 收尾。仅用 GetDC+ValidateRect
		// 不合规，会导致窗口永不重绘/透明（真机首跑暴露的"透明无内容"根因之一）。
		var ps paintStruct
		hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if w.memDC != 0 && w.dibW.Load() > 0 {
			bitBlt.Call(hdc, 0, 0, uintptr(w.dibW.Load()), uintptr(w.dibH.Load()), w.memDC, 0, 0, srccopy)
		}
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmActivate:
		switch int(wparam) & 0xFFFF {
		case waInactive:
			// 失焦自隐藏（点击窗口外部关闭，#151）。任一条件满足即隐藏：
			//  (1) 窗口此前确实被激活过（用户点开过，activated==1）；
			//  (2) 已稳定显示超过 250ms（shownAt 距现在 > 250ms）——覆盖「SetForegroundWindow
			//      抢前台失败、窗口从未激活（activated 恒 0）」的托盘程序常见情形，此时用户
			//      点击外部也应关闭。
			// 显示后 250ms 内的 WA_INACTIVE 视为「SW_SHOW 后原前台 reclaim 的假失焦」，
			// 两个条件都不满足则不隐藏，避免 S3「闪一下就没了」。仅对可见窗口响应。
			if w.visible.Load() == 1 &&
				(w.activated.Load() == 1 || time.Since(w.shownAt) > 250*time.Millisecond) {
				// 禁止在此直接 ShowWindow：会同步重入触发 WM_ACTIVATE→消息泵死锁。
				// 改异步投递，由 wmUserHide 在下一轮消息泵执行真正隐藏（#151 回归修复）。
				postMessage.Call(hwnd, wmUserHide, 0, 0)
				// 通知生命周期：窗口已自行隐藏，把期望可见态回写为 false，使后续托盘
				// 「显示/隐藏」切换基于正确意图（#151 显示/隐藏卡死修复）。回调仅做非阻塞
				// channel 发送（与 onClick/onChar 同模式），不在此直改业务状态（S1 单写者）。
				if fn := w.onDismissed.Load(); fn != nil {
					(*fn)()
				}
			}
		default: // waActive / waClickActive：确认窗口已激活
			w.activated.Store(1)
		}
		return 0
	case wmKeyDown:
		switch int(wparam) {
		case vkEscape:
			// 异步隐藏，避免 wndProc 内同步 ShowWindow 重入死锁（同 #151 回归修复）。
			postMessage.Call(hwnd, wmUserHide, 0, 0)
		case vkReturn, vkBack, vkTab, vkDelete:
			// 功能键（#148）：经 OnKey 回调投递到主循环处理（切视图/提交草稿/
			// 删除待办）。窗口线程只发键码，绝不直改业务状态（S1 单写者）。
			if fn := w.onKey.Load(); fn != nil {
				(*fn)(int(wparam))
			}
		}
		return 0
	case wmChar:
		// 字符输入（#148 待办输入框）：TranslateMessage 由 WM_KEYDOWN 翻译出 WM_CHAR，
		// wparam 低 16 位为 UTF-16 码元；我们取为 rune（BMP 内字符足够，复合表情/代理对
		// 不在待办输入场景使用）。经 OnChar 回调投递到主循环（S1 单写者）。
		if fn := w.onChar.Load(); fn != nil {
			(*fn)(rune(wparam & 0xFFFF))
		}
		return 0
	case wmLButtonDown:
		// 客户区左键按下：lParam 低 16 位 = x、次 16 位 = y（物理像素，相对窗口左上）。
		// 反算为 ui 的逻辑坐标（96-DPI 基准）：logical = physical × 96 / dpi。
		// 转换在主线程完成，结果经 onClick 回调投递给 app 主循环做命中测试与导航
		// （ADR-02：窗口线程不直改业务状态，仅发坐标）。
		if fn := w.onClick.Load(); fn != nil {
			px := int(int16(lparam & 0xFFFF))
			py := int(int16((lparam >> 16) & 0xFFFF))
			dpi := w.dpi
			if dpi <= 0 {
				dpi = 96
			}
			lx := int(float64(px)*96.0/float64(dpi) + 0.5)
			ly := int(float64(py)*96.0/float64(dpi) + 0.5)
			(*fn)(lx, ly)
		}
		return 0
	case wmClose:
		// 异步隐藏，避免 wndProc 内同步 ShowWindow 重入死锁（同 #151 回归修复）。
		postMessage.Call(hwnd, wmUserHide, 0, 0)
		return 0
	case wmDpiChanged:
		// wParam 高字 = 新的 X DPI。据新 DPI 重算尺寸后重建 DIB，再重新锚定到
		// 上次已知的托盘位置。刻意不解析 lParam 的 RECT 指针（untyped uintptr →
		// unsafe.Pointer 会被 go vet 判定为可能误用），改为自行计算，与设计一致。
		newDPI := int(wparam >> 16)
		w.dpi = newDPI // N1：DPI 变更后回写窗口线程局部 dpi，避免换屏后用旧 DPI 反算点击坐标偏移
		nw := scaleLogical(w.opts.Width, newDPI)
		nh := scaleLogical(w.opts.Height, newDPI)
		if nw > 0 && nh > 0 {
			w.createDIB(nw, nh)
			if w.lastBmp != nil {
				blitScaled(w.bits, nw, nh, w.lastBmp)
			}
			if w.lastTray != nil {
				w.anchor(w.lastTray)
			} else {
				setWindowPos.Call(uintptr(w.hwnd), 0, 0, 0, uintptr(nw), uintptr(nh), swpNoZOrder|swpNoActivate)
			}
			// DPI 变化重建 DIB 后请求重绘，使新分辨率位图上屏。
			invalidateRect.Call(uintptr(w.hwnd), 0, 0)
		}
		// #41 高 DPI：DIB 已按新 DPI 重建，发信号让主循环以新分辨率重渲
		// （回调仅经 channel 投递，不在此直改业务状态，S1 单写者）。
		if fn := w.onDPIChanged.Load(); fn != nil {
			(*fn)()
		}
		return 0
	case wmDestroy:
		postQuitMsg.Call(0)
		return 0
	}
	r, _, _ := defWndProc.Call(hwnd, message, wparam, lparam)
	return r
}

// anchor 将窗口锚定到托盘图标正上方居中（经 platform.AnchorAboveTray 计算并钳制）。
func (w *win32Window) anchor(r *image.Rectangle) {
	w.lastTray = r
	tray := platform.Rect{X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy()}
	mon := monitorFromPoint(tray.X+tray.W/2, tray.Y+tray.H/2)
	target := platform.AnchorAboveTray(int(w.dibW.Load()), int(w.dibH.Load()), w.margin, tray, mon)
	setWindowPos.Call(
		uintptr(w.hwnd), 0,
		uintptr(target.X), uintptr(target.Y),
		uintptr(target.W), uintptr(target.H),
		swpNoZOrder|swpNoActivate,
	)
}

// present 接收最新像素缓冲并触发重绘。
func (w *win32Window) present(bmp *image.RGBA) {
	if bmp == nil {
		return
	}
	w.lastBmp = bmp
	blitScaled(w.bits, int(w.dibW.Load()), int(w.dibH.Load()), bmp)
	// 请求重绘（InvalidateRect）：把整客户区加入更新区，触发 WM_PAINT 经 BitBlt
	// 把最新 DIB 上屏。误用 ValidateRect 等于告知系统"无需重绘" → 透明窗口无内容。
	invalidateRect.Call(uintptr(w.hwnd), 0, 0)
}

// ---- WindowController 接口实现（经消息派发到窗口线程）----------------------

// Show 经 PostMessage 异步派发到窗口线程。改用 PostMessage（而非 SendMessage）是 #151
// 卡死回归的根治：SendMessage 为跨线程同步调用，会阻塞调用方（主循环）直至窗口线程处理
// 完并返回；而 wmUserShow 内会调用 setForegroundWindow——该调用在某些焦点场景下会让系统
// 向其它线程同步发消息，与「主循环阻塞在 SendMessage」形成跨线程死锁，使整个消息泵冻结
// （显示/隐藏/退出同时失效，完全吻合用户反馈）。PostMessage 仅入队、立即返回，窗口线程在
// 自己的消息泵上下文（非 SendMessage 派发上下文）内处理 wmUserShow，彻底断开该死锁环。
// visible 在调用方立即置位，保证 life.Handle 的后续 win.Visible() 判断与渲染不受异步延迟影响。
func (w *win32Window) Show() {
	w.visible.Store(1)
	postMessage.Call(uintptr(w.hwnd), wmUserShow, 0, 0)
}

// Hide 同 Show，改用 PostMessage 异步派发，避免跨线程同步阻塞导致的消息泵死锁。
// visible 立即清零，使 life.Handle(CmdToggle) 的可见性判断即时正确。
func (w *win32Window) Hide() {
	w.visible.Store(0)
	postMessage.Call(uintptr(w.hwnd), wmUserHide, 0, 0)
}

func (w *win32Window) Visible() bool { return w.visible.Load() == 1 }

// Quit 经 PostMessage 异步派发 WM_DESTROY，随后阻塞至 done 关闭（窗口线程 postQuitMsg
// → 消息泵退出 → destroy → close(done)）。同 Show/Hide，用 PostMessage 避免同步死锁；
// 加 3s 超时兜底——若窗口线程仍冻结（done 不关闭），记录告警后返回，使主循环得以退出、
// 进程最终结束，而非永久卡死。hwnd 尚未就绪则直接返回。
func (w *win32Window) Quit() {
	if w.hwnd == 0 {
		return
	}
	postMessage.Call(uintptr(w.hwnd), wmDestroy, 0, 0)
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		logger.Warn("win32Window.Quit: 窗口线程 3s 内未退出，疑似消息泵死锁（冻结）")
	}
}

func (w *win32Window) AnchorAboveTray(r image.Rectangle) {
	// Store 堆拷贝：PostMessage 异步派发到窗口线程后，wndProc 经 Load 取出；
	// 窗口线程内 anchor() 会把指针存为 lastTray（DPI 变化时复用），故必须堆分配
	// 保证生命周期覆盖窗口存活期——不可 Store(&r)（栈局部，函数返回后即失效）。
	// 用 PostMessage（而非 SendMessage）是 #151 卡死回归的根治：主循环经 SendMessage
	// 同步阻塞等待窗口线程时，若窗口线程正忙于 setForegroundWindow 等会触发系统同步消息
	// 的调用，会形成跨线程死锁（显示/隐藏/退出同时失效，用户反馈的「点几次后全失效」）。
	// PostMessage 仅入队即返回，窗口线程在自己的消息泵上下文处理 wmUserAnchor，彻底断开死锁环。
	rp := new(image.Rectangle)
	*rp = r
	w.pendingRect.Store(rp)
	postMessage.Call(uintptr(w.hwnd), wmUserAnchor, 0, 0)
}

func (w *win32Window) Present(b *image.RGBA) {
	if b == nil {
		return
	}
	// Store 后异步 PostMessage（同 AnchorAboveTray 的 #151 死锁根治）：主循环 render()
	// 经此上屏，若用 SendMessage 同步阻塞，窗口线程在 setForegroundWindow/重绘等会触发
	// 系统同步消息的上下文里无法即时排空该消息 → 主循环永久卡死（连同退出命令一并吞掉）。
	// 改 PostMessage 后主循环永不阻塞于窗口线程；b 由 ui.Render 每次返回全新缓冲、
	// lastBmp 引用稳定，窗口线程稍后处理 wmUserPresent 时数据仍有效。
	w.pendingBmp.Store(b)
	postMessage.Call(uintptr(w.hwnd), wmUserPresent, 0, 0)
}

// OnClick 注册左键点击回调（#113）。回调在窗口线程的 WM_LBUTTONDOWN 处理中调用，
// 仅负责把逻辑坐标投递给主循环，不在本线程触碰业务状态（ADR-02）。
func (w *win32Window) OnClick(fn func(int, int)) { w.onClick.Store(&fn) }

// OnChar 注册字符输入回调（#148 待办输入框）。回调在窗口线程的 WM_CHAR 处理中调用，
// 仅负责把录入的 rune 投递给主循环，不在本线程触碰业务状态（S1 单写者）。
func (w *win32Window) OnChar(fn func(rune)) { w.onChar.Store(&fn) }

// OnKey 注册功能键回调（#148：Enter/Backspace/Tab/Delete）。回调在窗口线程的
// WM_KEYDOWN 处理中调用，仅把键码投递给主循环处理，不触碰业务状态（S1 单写者）。
func (w *win32Window) OnKey(fn func(int)) { w.onKey.Store(&fn) }

// PhysicalSize 返回当前 DIB 物理像素尺寸（#41 高 DPI 重渲用）。主循环经此反算
// 渲染 Scale = 物理宽 / 逻辑宽，使 ui.Render 产出与 DIB 1:1 的位图。原子读取，
// 因换屏(WM_DPICHANGED)时窗口线程会并发写入（S1 范畴的跨 goroutine 同步）。
// 注意：该方法是 *win32Window 的额外能力，不纳入 WindowController 接口，以免
// 破坏仅实现基础接口的测试 fake（接口隔离，与 clicker/keyboarder 同一手法）。
func (w *win32Window) PhysicalSize() (int, int) {
	return int(w.dibW.Load()), int(w.dibH.Load())
}

// OnDPIChanged 注册 DPI 变更回调（#41）。仅 app 主循环注册，回调在窗口线程的
// WM_DPICHANGED 处理末尾调用；回调实现【只】经 channel 向主循环投递「需重渲」信号，
// 绝不在此直改业务状态或调用渲染（S1 单写者）。不纳入 WindowController 接口。
func (w *win32Window) OnDPIChanged(fn func()) { w.onDPIChanged.Store(&fn) }

// OnDismissed 注册「点击外部自动隐藏」回调（#151 显示/隐藏一致性）。窗口线程在
// waInactive 决定隐藏后调用，仅负责把信号投递给主循环（非阻塞 channel 发送，与
// OnClick/OnChar/OnKey 同模式），不直接触碰业务状态（S1 单写者）。不纳入
// WindowController 接口，由 app 经局部 dismisser 接口断言调用。
func (w *win32Window) OnDismissed(fn func()) { w.onDismissed.Store(&fn) }

// ---- 显示器查询（锚定用）--------------------------------------------------

// winMonitor 实现 platform.Monitor，返回指定点的工作区矩形。
type winMonitor struct{ work platform.Rect }

func (m winMonitor) Bounds() platform.Rect { return m.work }
func (m winMonitor) DPI() int              { return 96 }

// monitorFromPoint 返回包含给定点的显示器工作区（MONITORINFO.rcWork）。
func monitorFromPoint(x, y int) platform.Monitor {
	hmon, _, _ := monitorFromPointProc.Call(uintptr(x), uintptr(y), uintptr(monitorDefaultToNearest))
	if hmon == 0 {
		return winMonitor{work: platform.Rect{X: 0, Y: 0, W: 1920, H: 1080}}
	}
	var mi monitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(monitorInfo{}))
	getMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	r := mi.RcWork
	return winMonitor{work: platform.Rect{
		X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top),
	}}
}

// wmDpiChanged 已在 wndProc 中处理：依据 lParam 建议矩形重建 DIB 并重新锚定。
