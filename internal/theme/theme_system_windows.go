//go:build windows

package theme

import (
	"context"
	"time"

	"golang.org/x/sys/windows/registry" // 纯 Go，零 CGO
)

// systemScheme 通过纯 Go 读取 Windows 个性化注册表判断当前系统方案。
// 路径：HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize\AppsUseLightTheme
// 0 = 深色, 1 = 浅色。失败时回退 SchemeLight（不致命，离线安全）。
func systemScheme() (Scheme, error) {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return SchemeLight, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return SchemeLight, err
	}
	if v == 0 {
		return SchemeDark, nil
	}
	return SchemeLight, nil
}

// watchSystem 在 Windows 下轮询注册表浅/深变化，变化时回调 onScheme；
// ctx 取消时退出。轮询间隔 2s，满足合理开销折中。
func watchSystem(ctx context.Context, onScheme func(Scheme)) error {
	last, _ := systemScheme()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s, err := systemScheme()
			if err != nil {
				continue
			}
			if s != last {
				last = s
				onScheme(s)
			}
		}
	}
}
