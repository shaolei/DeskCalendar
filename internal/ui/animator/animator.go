// Package animator 提供零依赖、可单测的动画原语（路径 D / ADR-08 / #120-Animation）。
//
// 职责：把「一段缓动」建模为纯函数 + 一个主线程推进的实例。MVP（v1.0）的
// Show/Hide 走瞬时显隐（无动画，见 #121），本包仅作为**类型与预设占位**，预留给
// v1.1+ 的视觉润色路线（#123）与 Animator 接入显隐——即「暂不接入显隐」（#122 原文）。
//
// 设计约束：
//   - 全程纯 Go、零 CGO、零外部依赖，便于 go test 覆盖缓动曲线数学。
//   - 不依赖 platform/win32/app——动画推进由调用方（未来 shell/win32 显隐路径）
//     在主线程以 Tick(now) 驱动，避免跨 goroutine 触碰窗口（ADR-02 双循环铁律）。
package animator

import "time"

// Easing 缓动函数：输入归一化进度 t∈[0,1]，输出缓动后的进度（通常仍∈[0,1]，
// 但回弹类可短暂超出，形成「过冲」手感）。
type Easing func(t float64) float64

// 预设缓动曲线（纯函数）。命名遵循 Robert Penner 缓动惯例。
var (
	// Linear 线性（无缓动）。
	Linear Easing = func(t float64) float64 { return t }
	// EaseInQuad 慢入（起步缓、收尾快）。
	EaseInQuad Easing = func(t float64) float64 { return t * t }
	// EaseOutQuad 快出（起步快、收尾缓）。
	EaseOutQuad Easing = func(t float64) float64 { return t * (2 - t) }
	// EaseInOutQuad 两端皆缓。
	EaseInOutQuad Easing = func(t float64) float64 {
		if t < 0.5 {
			return 2 * t * t
		}
		return -1 + (4-2*t)*t
	}
	// EaseOutCubic 立方快出，收尾更柔。
	EaseOutCubic Easing = func(t float64) float64 { return 1 - pow3(1-t) }
	// EaseInOutCubic 立方两端皆缓，平滑感更强。
	EaseInOutCubic Easing = func(t float64) float64 {
		if t < 0.5 {
			return 4 * t * t * t
		}
		return 1 - pow3(-2*t+2)/2
	}
	// EaseOutBack 回弹过冲（末段轻微回弹），适合「弹出」质感。
	EaseOutBack Easing = func(t float64) float64 {
		const c1 = 1.70158
		const c3 = c1 + 1
		// 标准公式：1 + c3·(t-1)³ + c1·(t-1)²（二次项提供过冲）。
		d := t - 1
		return 1 + c3*d*d*d + c1*d*d
	}
)

// pow3 立方（内部小工具）。
func pow3(x float64) float64 { return x * x * x }

// Kind 动画类型（v1.0 MVP 未接入显隐，预留给 v1.1+ 润色路线，见 #123）。
type Kind int

const (
	// KindNone 无动画（MVP 瞬时显隐的等价表述）。
	KindNone Kind = iota
	// KindFadeSlideIn 淡入 + 自托盘上方位移（#120 T2 目标）。
	KindFadeSlideIn
	// KindFadeOut 淡出。
	KindFadeOut
)

// Spec 一段动画的参数。
type Spec struct {
	// Duration 动画时长；≤0 时 Animator 视为瞬时（Tick 立即 done）。
	Duration time.Duration
	// Easing 缓动曲线；nil 时退化为 Linear。
	Easing Easing
	// Kind 动画类型（仅作语义标注，推进逻辑不依赖）。
	Kind Kind
}

// Animator 主线程推进的动画实例（MVP 仅保留类型，暂不接入显隐；#122）。
// 零值不可用：必须经 New 构造（设定默认 Duration/Easing）。
type Animator struct {
	spec   Spec
	start  time.Time
	active bool
}

// New 构造动画器并归一化参数（Easing nil→Linear；Duration≤0→200ms 兜底）。
func New(spec Spec) *Animator {
	if spec.Easing == nil {
		spec.Easing = Linear
	}
	if spec.Duration <= 0 {
		spec.Duration = 200 * time.Millisecond
	}
	return &Animator{spec: spec}
}

// Start 以 now 为起点开始动画（重置 active）。
func (a *Animator) Start(now time.Time) {
	a.start = now
	a.active = true
}

// Tick 依据 now 计算缓动后的进度。
//   - 返回 (进度, done=false) 表示进行中；
//   - 返回 (1, done=true) 表示已结束（进度钳制为 1）；
//   - 未 Start 时返回 (0, done=true)（视作已完成，避免调用方空转等待）。
//
// 进度已被 Easing 映射，故可超出 [0,1]（如 EaseOutBack 过冲），调用方应自行钳制。
func (a *Animator) Tick(now time.Time) (progress float64, done bool) {
	if !a.active {
		return 0, true
	}
	if a.spec.Duration <= 0 {
		a.active = false
		return 1, true
	}
	raw := float64(now.Sub(a.start)) / float64(a.spec.Duration)
	if raw >= 1 {
		a.active = false
		return 1, true
	}
	if raw < 0 {
		raw = 0
	}
	return a.spec.Easing(raw), false
}

// Active 报告动画是否仍在进行中。
func (a *Animator) Active() bool { return a.active }
