//go:build !windows

package win32

// newNativeWindow 非 Windows 下返回内存 fake（保证跨平台可编译与单测）。
func newNativeWindow(opts Options) WindowController {
	return &fakeWindow{}
}
