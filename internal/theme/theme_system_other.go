//go:build !windows

package theme

import "context"

// systemScheme 非 Windows（CI/headless）无系统主题概念，回退 light，不致命。
func systemScheme() (Scheme, error) {
	return SchemeLight, nil
}

// watchSystem 非 Windows 无系统主题事件，初值由 Watch 直接推送，此处立即返回。
func watchSystem(ctx context.Context, onScheme func(Scheme)) error {
	return nil
}
