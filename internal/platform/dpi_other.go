//go:build !windows
// +build !windows

package platform

// newPlatformDPIBackend 非 Windows 下返回 nil：SetAwareness 为 no-op，
// EffectiveDPI 回落系统默认 96（见 dpi.go）。
func newPlatformDPIBackend() dpiBackend { return nil }
