//go:build windows
// +build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// winDPIBackend 封装零 CGO 的 Win32 DPI API（user32.dll）。
type winDPIBackend struct{}

// dpiAwarenessContextValue 将 DPIAwareness 映射为
// DPI_AWARENESS_CONTEXT 特殊句柄值（见 winuser.h）：
//
//	UNAWARE            (HANDLE)-5
//	SYSTEM_AWARE      (HANDLE)-4
//	PER_MONITOR_AWARE (HANDLE)-3
//	PER_MONITOR_AWARE_V2 (HANDLE)-2
func dpiAwarenessContextValue(a DPIAwareness) uintptr {
	switch a {
	case DPIUnaware:
		return ^uintptr(0) - 4 // -5
	case DPISystemAware:
		return ^uintptr(0) - 3 // -4
	case DPIPerMonitorAware:
		return ^uintptr(0) - 2 // -3
	case DPIPerMonitorAwareV2:
		return ^uintptr(0) - 1 // -2
	default:
		return ^uintptr(0) - 1 // 默认 V2
	}
}

func (w *winDPIBackend) setAwareness(a DPIAwareness) error {
	proc := windows.NewLazyDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")
	r, _, err := proc.Call(dpiAwarenessContextValue(a))
	if r == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return fmt.Errorf("SetProcessDpiAwarenessContext(%d): %w", a, err)
		}
		return fmt.Errorf("SetProcessDpiAwarenessContext(%d) failed", a)
	}
	return nil
}

func (w *winDPIBackend) effectiveDPI() (int, int, error) {
	proc := windows.NewLazyDLL("user32.dll").NewProc("GetDpiForSystem")
	r, _, err := proc.Call()
	if r == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return 0, 0, fmt.Errorf("GetDpiForSystem: %w", err)
		}
		return 0, 0, fmt.Errorf("GetDpiForSystem returned 0")
	}
	d := int(r)
	return d, d, nil
}

func newPlatformDPIBackend() dpiBackend { return &winDPIBackend{} }
