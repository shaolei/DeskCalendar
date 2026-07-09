// Package theme 定义主题模型与「当前主题」解析/供给（MVP 基础主题）。
//
// 依赖方向（ADR-07a）：本包仅依赖 internal/theme 内部子模块（themejson 同包）、
// image/color、golang.org/x/sys/windows（仅 Windows 编译，纯 Go 零 CGO）。
// 不依赖 ui/plugin/platform，UI 只读本包产出的 *Theme 值对象。
package theme

import (
	"context"
	"fmt"
	"image/color"
	"sync"
)

// Scheme 表示明暗色彩方案（跟随系统或用户指定）。
type Scheme int

const (
	SchemeLight Scheme = iota
	SchemeDark
)

func (s Scheme) String() string {
	if s == SchemeDark {
		return "dark"
	}
	return "light"
}

// SchemeFromString 解析 "light"/"dark" 字符串（非法值回退 light）。
func SchemeFromString(s string) Scheme {
	if s == "dark" {
		return SchemeDark
	}
	return SchemeLight
}

// ColorPalette 主题调色板。规格要求 5 色 + 3 个渲染必需扩展色。
type ColorPalette struct {
	Background color.RGBA // 面板背景
	Surface    color.RGBA // 卡片/单元格底色
	Foreground color.RGBA // 主文字
	Muted      color.RGBA // 次要文字（表头/非本月）
	Accent     color.RGBA // 强调（标题/交互）
	HolidayRed color.RGBA // 节假日红
	TodayBlue  color.RGBA // 今日蓝
	Border     color.RGBA // 网格线
}

// Shadow 面板阴影参数（对接 ADR-03 DWM 阴影 + 自绘）。
type Shadow struct {
	Blur    int
	OffsetY int
	Color   color.RGBA
	Opacity float32
}

// Theme 是主题聚合根（值对象，不可变语义：切换即整体替换）。
type Theme struct {
	Name         string
	Builtin      bool
	Scheme       Scheme
	Palette      ColorPalette
	CornerRadius int
	Shadow       Shadow
	Alpha        float32 // 面板整体透明度 0..1（每像素 alpha 合成）
}

// ThemeProvider 解析并缓存「当前主题」，是 UI 取色的唯一入口。
type ThemeProvider struct {
	mu           sync.RWMutex
	current      *Theme
	light        *Theme
	dark         *Theme
	override     *Theme
	systemScheme Scheme // 最近一次探测到的系统方案（无覆盖时 current 据此解析）
}

// ProviderOption 构造期可选配置。
type ProviderOption func(*ThemeProvider)

// WithInitialScheme 指定初始系统方案（测试用；默认经 systemScheme 探测）。
// 同时写入 systemScheme，使 ClearOverride 能正确回退到该方案。
func WithInitialScheme(s Scheme) ProviderOption {
	return func(p *ThemeProvider) {
		p.systemScheme = s
		p.current = p.resolveLocked(s)
	}
}

// NewProvider 创建 Provider，从内嵌默认主题初始化 Light/Dark 两套。
func NewProvider(opts ...ProviderOption) (*ThemeProvider, error) {
	themes, err := LoadEmbedded(context.Background())
	if err != nil {
		return nil, fmt.Errorf("theme: load embedded themes: %w", err)
	}
	light, dark := pickByScheme(themes)
	if light == nil || dark == nil {
		return nil, fmt.Errorf("theme: embedded themes must contain light and dark (got %d)", len(themes))
	}
	p := &ThemeProvider{light: light, dark: dark}
	for _, o := range opts {
		o(p)
	}
	// 未显式指定初始方案时，探测系统方案（失败回退 light，离线安全）。
	p.mu.Lock()
	if p.current == nil {
		s, serr := systemScheme()
		if serr != nil {
			s = SchemeLight
		}
		p.systemScheme = s
		p.current = p.resolveLocked(s)
	}
	p.mu.Unlock()
	return p, nil
}

// pickByScheme 从加载的主题中按 scheme 选出 light/dark（first match）。
func pickByScheme(themes []*Theme) (light, dark *Theme) {
	for _, t := range themes {
		if t.Scheme == SchemeDark {
			if dark == nil {
				dark = t
			}
		} else {
			if light == nil {
				light = t
			}
		}
	}
	return light, dark
}

// resolveLocked 根据 Scheme 返回对应内置主题；override 非空时优先返回 override。
// 调用方须持锁（或构造期）。
func (p *ThemeProvider) resolveLocked(scheme Scheme) *Theme {
	if p.override != nil {
		return p.override
	}
	if scheme == SchemeDark {
		return p.dark
	}
	return p.light
}

// Resolve 根据 Scheme 返回对应主题（override 优先）。
func (p *ThemeProvider) Resolve(scheme Scheme) *Theme {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resolveLocked(scheme)
}

// Current 返回当前生效主题（线程安全读）。
func (p *ThemeProvider) Current() *Theme {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}

// SetOverride 设置用户覆盖主题（v1.3 Skin 调用）；传 nil 等同 ClearOverride。
func (p *ThemeProvider) SetOverride(t *Theme) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.override = t
	p.current = p.resolveLocked(p.currentScheme())
}

// ClearOverride 清除覆盖，恢复系统跟随。
func (p *ThemeProvider) ClearOverride() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.override = nil
	p.current = p.resolveLocked(p.currentScheme())
}

// currentScheme 推断 current 当前对应 scheme：override 优先看其 Scheme，
// 否则返回最近探测到的系统方案（不再做指针比较，避免 ClearOverride 错误回退）。
// 调用方须持锁。
func (p *ThemeProvider) currentScheme() Scheme {
	if p.override != nil {
		return p.override.Scheme
	}
	return p.systemScheme
}

// Watch 返回只读 channel，在初始与系统浅/深或 override 变化时推送 Scheme。
// 系统方案变化时（Windows 轮询注册表）会同步更新 Current()，使其保持实时跟随。
// 调用方须在 ctx 取消时停止接收以释放 goroutine。
func (p *ThemeProvider) Watch(ctx context.Context) <-chan Scheme {
	ch := make(chan Scheme, 1)
	go func() {
		// 立即推送当前方案，供订阅者获得初值。
		p.mu.RLock()
		cur := p.current
		p.mu.RUnlock()
		if cur != nil {
			sendScheme(ch, cur.Scheme)
		}
		// Windows 下轮询注册表变化；非 Windows 无系统主题事件，仅初值。
		if err := watchSystem(ctx, func(s Scheme) { p.onSystemSchemeChanged(ch, s) }); err != nil {
			return
		}
	}()
	return ch
}

// onSystemSchemeChanged 处理系统方案切换：记录 systemScheme，若无用户覆盖则
// 重建 current 使 Current() 实时跟随，并把新方案推送给 Watch 订阅者。
// 跨平台可单测（不依赖真实系统主题事件）。
func (p *ThemeProvider) onSystemSchemeChanged(ch chan<- Scheme, s Scheme) {
	p.mu.Lock()
	p.systemScheme = s
	if p.override == nil {
		p.current = p.resolveLocked(s)
	}
	p.mu.Unlock()
	sendScheme(ch, s)
}

// sendScheme 非阻塞推送（缓冲 1，溢出即丢弃，避免阻塞主流程）。
func sendScheme(ch chan<- Scheme, s Scheme) {
	select {
	case ch <- s:
	default:
	}
}

// ApplyMode 按字符串模式应用主题（供设置菜单调用）：
//   - "system" → 清除覆盖，跟随系统浅/深
//   - "light"  → 固定浅色覆盖
//   - "dark"   → 固定深色覆盖
//   - 其它值   → 返回错误（非法枚举）
//
// 同时写入 Current()，使订阅 Current() 的渲染层实时更新。
func (p *ThemeProvider) ApplyMode(mode string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch mode {
	case "system":
		p.override = nil
	case "light":
		p.override = p.light
	case "dark":
		p.override = p.dark
	default:
		return fmt.Errorf("theme: invalid mode %q (want system|light|dark)", mode)
	}
	p.current = p.resolveLocked(p.currentScheme())
	return nil
}
