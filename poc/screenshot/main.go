//go:build windows

// Headless-safe full-screen capturer for the ADR-03 POC (路径 D).
//
// Why this exists: the sandbox blocks `powershell Add-Type` (runtime .NET
// compile), so we capture the desktop the same way the POC draws it — through
// gdi32/user32 via windows.LazyDLL, zero CGO. We BitBlt the primary screen
// into a 32bpp DIB section and write shot.png (un-premultiplied RGBA).
package main

import (
	"image"
	"image/png"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	srcCopy        = 0x00CC0020
	smCxScreen     = 0
	smCyScreen     = 1
	dibRgbColors   = 0
	biRgb          = 0
	bitsPerPixel   = 32
)

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

var (
	user32 = windows.NewLazyDLL("user32.dll")
	gdi32  = windows.NewLazyDLL("gdi32.dll")

	getDC           = user32.NewProc("GetDC")
	releaseDC       = user32.NewProc("ReleaseDC")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	selectObject       = gdi32.NewProc("SelectObject")
	bitBlt             = gdi32.NewProc("BitBlt")
	deleteObject       = gdi32.NewProc("DeleteObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
)

func main() {
	w := int32(0)
	h := int32(0)
	if r, _, _ := getSystemMetrics.Call(smCxScreen); r != 0 {
		w = int32(r)
	}
	if r, _, _ := getSystemMetrics.Call(smCyScreen); r != 0 {
		h = int32(r)
	}
	if w == 0 || h == 0 {
		println("GetSystemMetrics failed")
		os.Exit(1)
	}
	println("screen:", w, "x", h)

	hdcScreen, _, _ := getDC.Call(0)
	if hdcScreen == 0 {
		println("GetDC(0) failed")
		os.Exit(1)
	}
	defer releaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := createCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		println("CreateCompatibleDC failed")
		os.Exit(1)
	}
	defer deleteDC.Call(hdcMem)

	bmi := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       w,
			Height:      -h, // negative => top-down
			Planes:      1,
			BitCount:    bitsPerPixel,
			Compression: biRgb,
		},
	}
	var bits unsafe.Pointer
	hbmp, _, _ := createDIBSection.Call(
		hdcScreen,
		uintptr(unsafe.Pointer(&bmi)),
		dibRgbColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbmp == 0 || bits == nil {
		println("CreateDIBSection failed")
		os.Exit(1)
	}
	defer deleteObject.Call(hbmp)
	selectObject.Call(hdcMem, hbmp)

	ok, _, _ := bitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen, 0, 0, srcCopy)
	if ok == 0 {
		println("BitBlt failed")
		os.Exit(1)
	}

	// bits are BGRA, top-down, 4 bytes/pixel
	px := (*[1 << 30]byte)(bits)[: w*h*4 : w*h*4]
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < int(w*h); i++ {
		b := px[i*4+0]
		g := px[i*4+1]
		r := px[i*4+2]
		img.Pix[i*4+0] = r
		img.Pix[i*4+1] = g
		img.Pix[i*4+2] = b
		img.Pix[i*4+3] = 255
	}

	f, err := os.Create("shot.png")
	if err != nil {
		println("create shot.png:", err.Error())
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		println("encode png:", err.Error())
		os.Exit(1)
	}
	println("shot.png written")
}
