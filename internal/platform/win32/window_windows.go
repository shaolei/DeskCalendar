//go:build windows

package win32

import (
	"context"
	"fmt"
	"image"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/shaolei/DeskCalendar/internal/platform"
)

// ---- Win32 常量 -------------------------------------------------------------
const (
	wsPopup        = 0x80000000
	wsExTopMost    = 0x00000008
	wsExToolWindow = 0x00000080

	swShow = 5
	swHide = 0

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmPaint      = 0x000F
	wmActivate   = 0x0006
	wmKeyDown    = 0x0100
	wmDpiChanged = 0x02E0

	// 自定义消息：由控制器方法经 SendMessage 派发到窗口线程执行。
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

// ---- 懒加载 native procs（包级，仅加载一次）-------------------------------
var (
	user32   = windows.NewLazyDLL("user32.dll")
	gdi32    = windows.NewLazyDLL("gdi32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	regClassEx      = user32.NewProc("RegisterClassExW")
	createWindowEx  = user32.NewProc("CreateWindowExW")
	showWindow      = user32.NewProc("ShowWindow")
	setWindowPos    = user32.NewProc("SetWindowPos")
	setForegroundWindow       = user32.NewProc("SetForegroundWindow")
	allowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	getMsg          = user32.NewProc("GetMessageW")
	translateMsg    = user32.NewProc("TranslateMessage")
	dispatchMsg     = user32.NewProc("DispatchMessageW")
	defWndProc      = user32.NewProc("DefWindowProcW")
	postQuitMsg     = user32.NewProc("PostQuitMessage")
	destroyWindow   = user32.NewProc("DestroyWindow")
	loadCursor      = user32.NewProc("LoadCursorW")
	getModuleHandle = kernel32.NewProc("GetModuleHandleW")
	sendMessage     = user32.NewProc("SendMessageW")
	getDC           = user32.NewProc("GetDC")
	releaseDC       = user32.NewProc("ReleaseDC")
	validateRect    = user32.NewProc("ValidateRect")
	monitorFromPointProc = user32.NewProc("MonitorFromPoint")
	getMonitorInfo  = user32.NewProc("GetMonitorInfoW")

	idcArrow       = 32512
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	// deleteObject 删除 GDI 对象。定义为 func 变量以便测试注入 seam，断言「删除前对象
	// 必须已从 memDC 顶出」—— S5 的核心不变量（绝不能删除仍被选中的 GDI 对象）。
	deleteObject = gdi32.NewProc("DeleteObject").Call
	bitBlt       = gdi32.NewProc("BitBlt")
)

// classSeq 为每个窗口实例分配唯一类名序号，避免多个 win32Window 共用同一
// RegisterClassExW 类名导致 wndProc 槽被首个实例占用（S6：第二窗口消息误派发到
// 第一窗口，且其 DIB 已释放 → 崩溃）。
var classSeq int64

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
	bits  []byte // DIB 像素（BGRA），指向 bitsPtr
	dibW  int
	dibH  int

	lastBmp *image.RGBA // 最近一次 Present 的缓冲（DPI 变化时重绘用）
	visible  atomic.Int32
	// activated 标记窗口本次显示后是否确实被激活过（用户点开）。WM_ACTIVATE 收到
	// WA_INACTIVE 时，仅当 activated==1 才自隐藏——区分「SW_SHOW 后未抢到前台、首个
	// WM_ACTIVATE 即 WA_INACTIVE」与「用户点开后又点外部导致失焦」，避免「闪一下就没了」（S3）。
	activated atomic.Int32

	// 跨线程传递的数据（经 SendMessage 同步派发，原子指针避免 unsafe.Pointer 传递）。
	pendingRect atomic.Pointer[image.Rectangle]
	pendingBmp  atomic.Pointer[image.RGBA]

	// 仅窗口线程访问：最近一次锚定的托盘矩形（DPI 变化时用于重新锚定）。
	lastTray *image.Rectangle

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
// 所有窗口操作经 SendMessage 派发到该线程，满足双循环铁律。
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
	w.dibW = scaleLogical(opts.Width, dpi)
	w.dibH = scaleLogical(opts.Height, dpi)

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
		uintptr(w.dibW), uintptr(w.dibH),
		0, 0, hInst, 0,
	)
	w.hwnd = windows.Handle(hwnd)
	w.createDIB(w.dibW, w.dibH)

	ready <- nil // 此后 shell 才可安全调用 Show/Hide（happens-before 同步）

	var m msg
	for {
		ret, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 { // WM_QUIT
			break
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
	w.destroy()
	close(w.done)
}

// createDIB 创建/重建与窗口同尺寸的 DIBSection，并填充中性底色避免垃圾像素。
//
// S5 修复：CreateDIBSection 后立即 selectObject 把新位图选入 memDC——这一步会把旧位图
// 从 memDC 中「顶出」并以返回值交还。我们再用返回的旧对象决定：
//   - 若 w.hbmp!=0（resize 路径）：old 是上一轮的 DIB，此刻已不再被 memDC 选中 → 安全 deleteObject；
//   - 若 w.hbmp==0（首次）：old 是 memDC 自带的默认 1x1 位图 → 缓存为 origBmp，留作将来删除前的「安全替身」。
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
	w.dibW, w.dibH = width, height
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
	switch message {
	case wmUserShow:
		showWindow.Call(hwnd, swShow)
		w.visible.Store(1)
		// S3：WS_EX_TOPMOST 弹窗 SW_SHOW 不保证拿到前台；若原焦点窗口 reclaim，会先收到
		// WM_ACTIVATE(WA_INACTIVE) 导致刚显示就被自隐藏（点托盘闪一下就没）。这里显式抢前台
		// 并放行前台权限，使窗口成为前台、收到 WA_ACTIVE（activated=1）。
		allowSetForegroundWindow.Call(asfwAny)
		setForegroundWindow.Call(hwnd)
		w.activated.Store(0) // 本次显示尚未确认激活，等真实 WA_ACTIVE 置位
		validateRect.Call(hwnd, 0)
		return 0
	case wmUserHide:
		showWindow.Call(hwnd, swHide)
		w.visible.Store(0)
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
		hdc, _, _ := getDC.Call(hwnd)
		if w.memDC != 0 && w.dibW > 0 {
			bitBlt.Call(hdc, 0, 0, uintptr(w.dibW), uintptr(w.dibH), w.memDC, 0, 0, srccopy)
		}
		releaseDC.Call(hwnd, hdc)
		validateRect.Call(hwnd, 0)
		return 0
	case wmActivate:
		switch int(wparam) & 0xFFFF {
		case waInactive:
			// 仅当本窗口此前确实激活过（用户点开过）才自隐藏；否则（SW_SHOW 后未抢到
			// 前台，首个 WM_ACTIVATE 即 WA_INACTIVE）不隐藏，避免 S3「闪一下就没了」。
			if w.activated.Load() == 1 {
				showWindow.Call(hwnd, swHide)
				w.visible.Store(0)
			}
		default: // waActive / waClickActive：确认窗口已激活
			w.activated.Store(1)
		}
		return 0
	case wmKeyDown:
		if int(wparam) == vkEscape {
			showWindow.Call(hwnd, swHide)
			w.visible.Store(0)
		}
		return 0
	case wmClose:
		showWindow.Call(hwnd, swHide)
		w.visible.Store(0)
		return 0
	case wmDpiChanged:
		// wParam 高字 = 新的 X DPI。据新 DPI 重算尺寸后重建 DIB，再重新锚定到
		// 上次已知的托盘位置。刻意不解析 lParam 的 RECT 指针（untyped uintptr →
		// unsafe.Pointer 会被 go vet 判定为可能误用），改为自行计算，与设计一致。
		newDPI := int(wparam >> 16)
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
			validateRect.Call(uintptr(w.hwnd), 0)
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
	target := platform.AnchorAboveTray(w.dibW, w.dibH, w.margin, tray, mon)
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
	blitScaled(w.bits, w.dibW, w.dibH, bmp)
	validateRect.Call(uintptr(w.hwnd), 0)
}

// ---- WindowController 接口实现（经 SendMessage 派发到窗口线程）------------

func (w *win32Window) Show() {
	sendMessage.Call(uintptr(w.hwnd), wmUserShow, 0, 0)
}

func (w *win32Window) Hide() {
	sendMessage.Call(uintptr(w.hwnd), wmUserHide, 0, 0)
}

func (w *win32Window) Visible() bool { return w.visible.Load() == 1 }

func (w *win32Window) AnchorAboveTray(r image.Rectangle) {
	// Store 堆拷贝：SendMessage 同步派发到窗口线程后，wndProc 经 Load 取出；
	// 窗口线程内 anchor() 会把指针存为 lastTray（DPI 变化时复用），故必须堆分配
	// 保证生命周期覆盖窗口存活期——不可 Store(&r)（栈局部，函数返回后即失效）。
	rp := new(image.Rectangle)
	*rp = r
	w.pendingRect.Store(rp)
	sendMessage.Call(uintptr(w.hwnd), wmUserAnchor, 0, 0)
}

func (w *win32Window) Present(b *image.RGBA) {
	if b == nil {
		return
	}
	// Store 后同步 SendMessage，窗口线程 present() 消费前指针有效（b 由调用方持有，
	// 当前每次 ui.Render 返回全新缓冲，lastBmp 引用稳定）。
	w.pendingBmp.Store(b)
	sendMessage.Call(uintptr(w.hwnd), wmUserPresent, 0, 0)
}

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
