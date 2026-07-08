//go:build windows
// +build windows

package platform

import (
	"golang.org/x/sys/windows/registry"
)

// winRegistryBackend 基于 HKCU 注册表的真实实现（零 CGO）。
type winRegistryBackend struct{}

func openRunKey() (registry.Key, error) {
	// 不存在则创建（用户首次设置自启时注册表 Run 项可能尚不存在）。
	k, _, err := registry.CreateKey(registry.CURRENT_USER, RegistryKey,
		registry.SET_VALUE|registry.QUERY_VALUE|registry.READ)
	if err != nil {
		return 0, err
	}
	return k, nil
}

func (b *winRegistryBackend) setString(key, valueName, value string) error {
	k, err := openRunKey()
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(valueName, value)
}

func (b *winRegistryBackend) deleteValue(key, valueName string) error {
	k, err := openRunKey()
	if err != nil {
		return err
	}
	defer k.Close()
	// 值不存在时 DeleteValue 返回 ERROR_FILE_NOT_FOUND，视为成功（幂等）。
	if err := k.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func (b *winRegistryBackend) queryString(key, valueName string) (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, RegistryKey, registry.QUERY_VALUE|registry.READ)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", false, nil
		}
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueName)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func newPlatformRegistryBackend() registryBackend { return &winRegistryBackend{} }
