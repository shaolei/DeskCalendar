package theme

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
)

// defaultFS 编译期固化默认主题（离线保证，零网络）。
//
//go:embed embedded/themes/*.json
var defaultFS embed.FS

// ShadowFile 是 JSON 中阴影的外部表示。
type ShadowFile struct {
	Blur    int     `json:"blur"`
	OffsetY int     `json:"offsetY"`
	Color   string  `json:"color"` // "#RRGGBBAA"
	Opacity float32 `json:"opacity"`
}

// ThemeFile 是 JSON 主题文件的完整 schema（用户手写友好）。
type ThemeFile struct {
	Name         string            `json:"name"`
	Scheme       string            `json:"scheme"`       // "light" | "dark"
	Colors       map[string]string `json:"colors"`       // 角色 → "#RRGGBBAA"
	CornerRadius int               `json:"cornerRadius"` // px
	Shadow       ShadowFile        `json:"shadow"`
	Alpha        float32           `json:"alpha"` // 0..1
}

// requiredColors 必填色角色（缺失即视为坏主题）。
var requiredColors = []string{
	"background", "surface", "foreground", "muted",
	"accent", "holidayRed", "todayBlue", "border",
}

// LoadEmbedded 解析编译期嵌入的默认主题集合。
func LoadEmbedded(ctx context.Context) ([]*Theme, error) {
	entries, err := defaultFS.ReadDir("embedded/themes")
	if err != nil {
		return nil, fmt.Errorf("themejson: read embed dir: %w", err)
	}
	out := make([]*Theme, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, rerr := defaultFS.ReadFile("embedded/themes/" + e.Name())
		if rerr != nil {
			return nil, fmt.Errorf("themejson: read %s: %w", e.Name(), rerr)
		}
		t, perr := ParseBytes(ctx, data)
		if perr != nil {
			return nil, fmt.Errorf("themejson: parse %s: %w", e.Name(), perr)
		}
		out = append(out, t)
	}
	return out, nil
}

// ParseBytes 将 JSON 字节解析并校验为 *Theme（内置主题，Builtin=true）。
func ParseBytes(ctx context.Context, data []byte) (*Theme, error) {
	return parseBytes(ctx, data, true)
}

// ParseFile 从磁盘读取并解析用户主题 JSON（v1.3 启用），标记 Builtin=false。
func ParseFile(ctx context.Context, path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("themejson: read %s: %w", path, err)
	}
	return parseBytes(ctx, data, false)
}

// parseBytes 解析并校验主题字节；builtin 控制 Builtin 标记（内置 true / 用户 false，S6）。
func parseBytes(ctx context.Context, data []byte, builtin bool) (*Theme, error) {
	var tf ThemeFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("themejson: unmarshal: %w", err)
	}
	t, err := buildTheme(&tf, builtin)
	if err != nil {
		return nil, err
	}
	if err := Validate(t); err != nil {
		return nil, err
	}
	return t, nil
}

// buildTheme 把 ThemeFile 映射为内部 *Theme，缺失必填色返回 error。
// builtin 标记主题来源（内置=true / 用户=false），供 UI/设置区分（S6）。
func buildTheme(tf *ThemeFile, builtin bool) (*Theme, error) {
	if tf.Name == "" {
		return nil, fmt.Errorf("themejson: name is required")
	}
	palette := ColorPalette{}
	assign := func(key string, dst *color.RGBA) error {
		hexStr, ok := tf.Colors[key]
		if !ok {
			return fmt.Errorf("themejson: missing required color %q", key)
		}
		c, err := parseColor(hexStr)
		if err != nil {
			return fmt.Errorf("themejson: color %q: %w", key, err)
		}
		*dst = c
		return nil
	}
	for _, key := range requiredColors {
		switch key {
		case "background":
			if err := assign(key, &palette.Background); err != nil {
				return nil, err
			}
		case "surface":
			if err := assign(key, &palette.Surface); err != nil {
				return nil, err
			}
		case "foreground":
			if err := assign(key, &palette.Foreground); err != nil {
				return nil, err
			}
		case "muted":
			if err := assign(key, &palette.Muted); err != nil {
				return nil, err
			}
		case "accent":
			if err := assign(key, &palette.Accent); err != nil {
				return nil, err
			}
		case "holidayRed":
			if err := assign(key, &palette.HolidayRed); err != nil {
				return nil, err
			}
		case "todayBlue":
			if err := assign(key, &palette.TodayBlue); err != nil {
				return nil, err
			}
		case "border":
			if err := assign(key, &palette.Border); err != nil {
				return nil, err
			}
		}
	}
	sc, err := parseColor(tf.Shadow.Color)
	if err != nil {
		return nil, fmt.Errorf("themejson: shadow.color: %w", err)
	}
	return &Theme{
		Name:         tf.Name,
		Builtin:      builtin,
		Scheme:       SchemeFromString(tf.Scheme),
		Palette:      palette,
		CornerRadius: tf.CornerRadius,
		Shadow: Shadow{
			Blur:    tf.Shadow.Blur,
			OffsetY: tf.Shadow.OffsetY,
			Color:   sc,
			Opacity: tf.Shadow.Opacity,
		},
		Alpha: tf.Alpha,
	}, nil
}

// parseColor 解析 "#RRGGBB" 或 "#RRGGBBAA" 为 color.RGBA（alpha 缺省 255）。
func parseColor(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 && len(h) != 8 {
		return color.RGBA{}, fmt.Errorf("invalid color %q (want #RRGGBB or #RRGGBBAA)", s)
	}
	if _, err := hex.DecodeString(h); err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex in %q: %w", s, err)
	}
	r, _ := strconvParseHex(h[0:2])
	g, _ := strconvParseHex(h[2:4])
	b, _ := strconvParseHex(h[4:6])
	a := 0xff
	if len(h) == 8 {
		a, _ = strconvParseHex(h[6:8])
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, nil
}

// strconvParseHex 解析两位十六进制（小封装，避免再引 strconv 于热点）。
func strconvParseHex(s string) (int, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	return int(v), err
}

// Validate 严格校验主题：必填色角色齐全（buildTheme 已保证）、范围合理。
func Validate(t *Theme) error {
	if t.CornerRadius < 0 || t.CornerRadius > 64 {
		return fmt.Errorf("themejson: cornerRadius %d out of [0,64]", t.CornerRadius)
	}
	if t.Alpha < 0 || t.Alpha > 1 {
		return fmt.Errorf("themejson: alpha %v out of [0,1]", t.Alpha)
	}
	if t.Shadow.Opacity < 0 || t.Shadow.Opacity > 1 {
		return fmt.Errorf("themejson: shadow.opacity %v out of [0,1]", t.Shadow.Opacity)
	}
	return nil
}
