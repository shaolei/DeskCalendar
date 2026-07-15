// Package config 提供配置持久化（JSON 文件，落 %AppData%/DeskCalendar/config.json）。
//
// 纯 stdlib、零 CGO。Phase 0 仅定义可序列化配置结构与 Load/Save/Default；
// 与 Store 快照的映射由 shell（Phase 3）负责。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 是应用配置根结构。
type Config struct {
	Version int           `json:"version"`
	Theme   ThemeConfig   `json:"theme"`
	Window  WindowConfig  `json:"window"`
	Startup StartupConfig `json:"startup"`
	Display DisplayConfig `json:"display"`
	Weather WeatherConfig `json:"weather"`
}

// ThemeConfig 主题相关配置。
type ThemeConfig struct {
	// Mode: "system" | "light" | "dark"
	Mode string `json:"mode"`
	// Accent: 十六进制颜色 "#RRGGBB"
	Accent string `json:"accent"`
}

// WindowConfig 窗口相关配置。
type WindowConfig struct {
	CornerRadius int  `json:"corner_radius"`
	Shadow       bool `json:"shadow"`
	// PositionMode: "tray" | "cursor"
	PositionMode string `json:"position_mode"`
}

// StartupConfig 开机自启相关配置。
type StartupConfig struct {
	AutoStart bool `json:"auto_start"`
}

// DisplayConfig 日历显示开关（托盘菜单直接切换）。
type DisplayConfig struct {
	// ShowLunar 是否在日期格显示农历。
	ShowLunar bool `json:"show_lunar"`
	// ShowHoliday 是否高亮节假日。
	ShowHoliday bool `json:"show_holiday"`
}

// WeatherConfig 天气数据源配置（v1.2 EPIC #149）。
//
// 默认 Open-Meteo（免 key，ADR-05b）。填 QWeatherKey 后自动切和风（中国精度最佳）。
// Lat/Lng 为默认坐标（北京），用户未手动定位时使用。
type WeatherConfig struct {
	QWeatherKey string  `json:"qweather_key"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

// Default 返回 MVP 默认配置。
func Default() Config {
	return Config{
		Version: 1,
		Theme: ThemeConfig{
			Mode:   "system",
			Accent: "#4C8DFF",
		},
		Window: WindowConfig{
			CornerRadius: 16,
			Shadow:       true,
			PositionMode: "tray",
		},
		Startup: StartupConfig{
			AutoStart: false,
		},
		Display: DisplayConfig{
			ShowLunar:   true,
			ShowHoliday: true,
		},
		Weather: WeatherConfig{
			Lat: 39.9042, // 北京
			Lng: 116.4074,
		},
	}
}

// DefaultPath 返回配置文件默认路径：<UserConfigDir>/DeskCalendar/config.json。
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: get user config dir: %w", err)
	}
	return filepath.Join(dir, "DeskCalendar", "config.json"), nil
}

// Load 读取并解析配置文件；文件不存在时返回 Default()（不报错）。
//
// 解析策略：以 Default() 为基底，仅用 JSON 中出现的字段覆盖；未出现的字段
// 保留默认值，从而修复「部分 JSON（如旧版本缺字段、用户手动删节）静默零值」的问题。
// 枚举字段（Theme.Mode / Window.PositionMode）若取值非法，回落到默认值，避免脏配置
// 破坏启动。
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	c := Default()
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if c.Version == 0 {
		c.Version = 1
	}
	c.Theme.Mode = normalizeThemeMode(c.Theme.Mode)
	c.Theme.Accent = normalizeAccent(c.Theme.Accent)
	c.Window.PositionMode = normalizePositionMode(c.Window.PositionMode)
	return c, nil
}

// normalizeAccent 校验强调色格式，非法值（非 "#RRGGBB"）回落 defaultAccent。
// 防止脏配置让 UI 拿到空字符串/非法色值导致渲染异常。
func normalizeAccent(v string) string {
	if len(v) != 7 || v[0] != '#' {
		return defaultAccent
	}
	for _, c := range v[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return defaultAccent
		}
	}
	return v
}

// defaultAccent 是 Accent 的兜底值（与 Default() 保持一致）。
const defaultAccent = "#4C8DFF"

// normalizeThemeMode 校验主题模式，非法值回落 "system"。
func normalizeThemeMode(v string) string {
	switch v {
	case "system", "light", "dark":
		return v
	default:
		return "system"
	}
}

// normalizePositionMode 校验窗口定位模式，非法值回落 "tray"。
func normalizePositionMode(v string) string {
	switch v {
	case "tray", "cursor":
		return v
	default:
		return "tray"
	}
}

// Save 将配置写入文件（自动创建父目录）。
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
