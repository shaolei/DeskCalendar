//go:build !windows
// +build !windows

package platform

// newPlatformRegistryBackend 非 Windows 下返回 nil（无注册表）。
func newPlatformRegistryBackend() registryBackend { return nil }
