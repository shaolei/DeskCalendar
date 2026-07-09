package theme

import (
	"embed"
	"fmt"
	"strings"
)

// Resolution 图标分辨率档位。
type Resolution int

const (
	R16 Resolution = iota
	R32
	R48
	R256
)

// resName 分辨率 → 文件名后缀。
func (r Resolution) resName() string {
	switch r {
	case R16:
		return "16"
	case R32:
		return "32"
	case R48:
		return "48"
	case R256:
		return "256"
	default:
		return "16"
	}
}

// parseResolution 把文件名中的分辨率字符串解析为 Resolution。
func parseResolution(s string) (Resolution, error) {
	switch s {
	case "16":
		return R16, nil
	case "32":
		return R32, nil
	case "48":
		return R48, nil
	case "256":
		return R256, nil
	default:
		return 0, fmt.Errorf("icon: unknown resolution %q", s)
	}
}

// IconSet 持有 Light/Dark 两套各分辨率 PNG 字节。
type IconSet struct {
	Light map[Resolution][]byte
	Dark  map[Resolution][]byte
}

// IconProvider 按 Scheme 提供图标字节，屏蔽资源来源（embed）。
type IconProvider struct {
	fs  embed.FS
	set *IconSet
}

//go:embed embedded/icons/*.png
var iconFS embed.FS

// NewIconProvider 加载 embed 资源到内存 IconSet。
func NewIconProvider() (*IconProvider, error) {
	set, err := loadIconSet(iconFS)
	if err != nil {
		return nil, err
	}
	return &IconProvider{fs: iconFS, set: set}, nil
}

// loadIconSet 解析 embedded/icons 下 `<scheme>_<res>.png` 命名。
func loadIconSet(fs embed.FS) (*IconSet, error) {
	entries, err := fs.ReadDir("embedded/icons")
	if err != nil {
		return nil, fmt.Errorf("icon: read embed dir: %w", err)
	}
	set := &IconSet{Light: map[Resolution][]byte{}, Dark: map[Resolution][]byte{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".png")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("icon: bad filename %q (want <scheme>_<res>.png)", e.Name())
		}
		scheme := parts[0]
		res, rerr := parseResolution(parts[1])
		if rerr != nil {
			return nil, fmt.Errorf("icon: %s: %w", e.Name(), rerr)
		}
		data, rerr := fs.ReadFile("embedded/icons/" + e.Name())
		if rerr != nil {
			return nil, fmt.Errorf("icon: read %s: %w", e.Name(), rerr)
		}
		switch scheme {
		case "light":
			set.Light[res] = data
		case "dark":
			set.Dark[res] = data
		default:
			return nil, fmt.Errorf("icon: unknown scheme %q in %s", scheme, e.Name())
		}
	}
	if len(set.Light) == 0 || len(set.Dark) == 0 {
		return nil, fmt.Errorf("icon: missing light/dark png assets")
	}
	return set, nil
}

// ForScheme 返回对应明暗的图标字节映射（按分辨率索引）。
func (p *IconProvider) ForScheme(s Scheme) map[Resolution][]byte {
	if s == SchemeDark {
		return p.set.Dark
	}
	return p.set.Light
}

// TrayIcon 返回托盘通知区图标字节（默认 16px，按 Scheme 选明暗）。
// 缺失 16px 时回退到任一可用分辨率，避免返回 nil 致托盘无入口。
func (p *IconProvider) TrayIcon(s Scheme) ([]byte, error) {
	return p.iconAt(s, R16)
}

// WinIcon 返回窗口/任务栏图标字节（默认 48px，高分屏由 platform 选 256）。
func (p *IconProvider) WinIcon(s Scheme, res Resolution) ([]byte, error) {
	return p.iconAt(s, res)
}

// iconAt 取指定 scheme+分辨率字节，缺失时回退到同 scheme 任一分辨率。
func (p *IconProvider) iconAt(s Scheme, res Resolution) ([]byte, error) {
	m := p.ForScheme(s)
	if b, ok := m[res]; ok {
		return b, nil
	}
	// 回退：任意可用分辨率。
	for _, try := range []Resolution{R16, R32, R48, R256} {
		if b, ok := m[try]; ok {
			return b, nil
		}
	}
	return nil, fmt.Errorf("icon: no png for scheme=%s res=%s", s, res.resName())
}
