//go:build windows

// Native layered-window half of the ADR-03 POC (路径 D: gg + 原生分层窗口).
//
// gogpu/gg only rasterises; it is NOT a window library. So we own the window
// here, in project code, with zero dependency patches. We render a panel to a
// PREMULTIPLIED RGBA buffer (via RenderPanel — supplied by draw_stdlib.go or
// draw_gg.go) and push it into a WS_EX_LAYERED window through UpdateLayeredWindow
// with per-pixel alpha (AC_SRC_ALPHA). That gives real rounded corners + real
// per-pixel transparency + free Show/Hide + free position (SetWindowPos) — the
// three things upstream gogpu could not provide natively (see ADR-03).
//
// NOTE: golang.org/x/sys/windows in our sandbox does NOT ship named wrappers
// for CreateWindowEx/UpdateLayeredWindow/etc., so we call user32/gdi32 through
// windows.LazyDLL + manual structs. Core types (Handle, Errno, NewCallback,
// UTF16PtrFromString) ARE present and stable.
package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- Win32 constants --------------------------------------------------------
const (
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000
	wsExToolWindow = 0x00000080
	wsPopup        = 0x80000000

	swShow       = 5
	swHide       = 0
	ulwAlpha     = 2
	acSrcOver    = 0
	acSrcAlpha   = 1
	idiArrow     = 32512
	dibRgbColors = 0

	swpNoSize     = 0x0001
	swpNoActivate = 0x0002
	swpShowWindow = 0x0040

	wmDestroy = 0x0010
	wmKeyDown = 0x0100
	wmTimer   = 0x0113
	wmLButtonUp = 0x0202
	vkEscape  = 0x1B
)

// ---- minimal Win32 structs (field order = ABI layout) -----------------------
type point struct {
	X, Y int32
}

type size struct {
	CX, CY int32
}

type blendfunc struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
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

type rgbquad struct {
	Blue, Green, Red, Reserved byte
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]rgbquad
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

// ---- lazily-loaded native procs (package-level: loaded once) ----------------
var (
	user32   = windows.NewLazyDLL("user32.dll")
	gdi32    = windows.NewLazyDLL("gdi32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	regClassEx      = user32.NewProc("RegisterClassExW")
	createWindowEx  = user32.NewProc("CreateWindowExW")
	showWindow      = user32.NewProc("ShowWindow")
	updateLayered   = user32.NewProc("UpdateLayeredWindow")
	setWindowPos    = user32.NewProc("SetWindowPos")
	getMsg          = user32.NewProc("GetMessageW")
	translateMsg    = user32.NewProc("TranslateMessage")
	dispatchMsg     = user32.NewProc("DispatchMessageW")
	defWndProc      = user32.NewProc("DefWindowProcW")
	postQuitMsg     = user32.NewProc("PostQuitMessage")
	destroyWindow   = user32.NewProc("DestroyWindow")
	loadCursor      = user32.NewProc("LoadCursorW")
	setTimer        = user32.NewProc("SetTimer")
	getModuleHandle = kernel32.NewProc("GetModuleHandleW")

	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
)

// wndProc: minimal handler — close on Escape / left-click / timer.
func wndProc(hwnd windows.Handle, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmDestroy:
		postQuitMsg.Call(0)
		return 0
	case wmKeyDown:
		if wparam == vkEscape {
			destroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case wmLButtonUp:
		destroyWindow.Call(uintptr(hwnd))
		return 0
	case wmTimer:
		destroyWindow.Call(uintptr(hwnd))
		return 0
	}
	r, _, _ := defWndProc.Call(uintptr(hwnd), uintptr(message), wparam, lparam)
	return r
}

// runLayeredWindow: build the panel bitmap and show it in a real layered window.
func runLayeredWindow() {
	premul, w, h := RenderPanel()

	// --- 1. DIB section holding the premultiplied pixel data (BGRA order) ---
	bmi := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       int32(w),
			Height:      -int32(h), // negative => top-down (first row = top)
			Planes:      1,
			BitCount:    32,
			Compression: 0, // BI_RGB
		},
	}
	var bitsPtr unsafe.Pointer
	memDC, _, _ := createCompatibleDC.Call(0)
	if memDC == 0 {
		println("CreateCompatibleDC failed")
		return
	}
	hbmp, _, _ := createDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		dibRgbColors,
		uintptr(unsafe.Pointer(&bitsPtr)),
		0,
		0,
	)
	if hbmp == 0 || bitsPtr == nil {
		println("CreateDIBSection failed")
		return
	}
	// copy RGBA(premul) -> BGRA(bits): only R/B swap, alpha stays put
	bits := (*[1 << 30]byte)(bitsPtr)[: w*h*4 : w*h*4]
	for i := 0; i < w*h; i++ {
		bits[i*4+0] = premul[i*4+2] // B
		bits[i*4+1] = premul[i*4+1] // G
		bits[i*4+2] = premul[i*4+0] // R
		bits[i*4+3] = premul[i*4+3] // A
	}
	selectObject.Call(memDC, hbmp)

	// --- 2. window class ---
	hInst, _, _ := getModuleHandle.Call(0)
	hCursor, _, _ := loadCursor.Call(0, idiArrow)
	className, _ := windows.UTF16PtrFromString("DeskCalendarLayeredPOC")
	wcex := wndClassexW{
		Size:      uint32(unsafe.Sizeof(wndClassexW{})),
		WndProc:   windows.NewCallback(wndProc),
		Instance:  windows.Handle(hInst),
		Cursor:    windows.Handle(hCursor),
		ClassName: className,
	}
	regClassEx.Call(uintptr(unsafe.Pointer(&wcex)))

	// --- 3. create layered + no-activate + tool popup (no caption/taskbar) ---
	posX, posY := int32(300), int32(300)
	hwnd, _, _ := createWindowEx.Call(
		wsExLayered|wsExNoActivate|wsExToolWindow,
		uintptr(unsafe.Pointer(className)), // lpClassName
		0,                                  // lpWindowName (NULL)
		wsPopup,
		uintptr(posX), uintptr(posY),
		uintptr(w), uintptr(h),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		println("CreateWindowExW failed")
		deleteDC.Call(memDC)
		return
	}

	// --- 4. push the bitmap with per-pixel alpha + initial position ---
	ptDst := point{X: posX, Y: posY}
	ptSrc := point{}
	sz := size{CX: int32(w), CY: int32(h)}
	blend := blendfunc{
		BlendOp:             acSrcOver,
		SourceConstantAlpha: 255,
		AlphaFormat:         acSrcAlpha,
	}
	showWindow.Call(hwnd, swShow)
	ulw, _, _ := updateLayered.Call(
		hwnd,
		0, // hdcDst = NULL (desktop)
		uintptr(unsafe.Pointer(&ptDst)),
		uintptr(unsafe.Pointer(&sz)),
		memDC,
		uintptr(unsafe.Pointer(&ptSrc)),
		0, // crKey (unused, alpha mode)
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
	if ulw == 0 {
		println("UpdateLayeredWindow failed")
		deleteDC.Call(memDC)
		return
	}

	// --- 5. demonstrate runtime SetPosition (move down-right, keep bitmap) ---
	setWindowPos.Call(
		hwnd, 0,
		uintptr(posX+60), uintptr(posY+60),
		0, 0,
		swpNoSize|swpNoActivate|swpShowWindow,
	)

	// --- 6. auto-close after 8s (plus ESC / click) ---
	setTimer.Call(hwnd, 1, 8000, 0)

	println("Layered window shown (moved to", posX+60, posY+60, "size", w, "x", h, ")")
	println("Press ESC or click to close; auto-closes in 8s")

	// --- 7. message loop ---
	var m msg
	for {
		ret, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 { // WM_QUIT
			break
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}

	deleteDC.Call(memDC)
	println("Layered window closed")
}
