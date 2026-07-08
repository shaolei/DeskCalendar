package state

import "github.com/coregx/signals"

// Signal 是 DeskCalendar 全局复用的响应式原语。
//
// 它是 github.com/coregx/signals.Signal[T] 的类型别名——与 gogpu/ui/state.Signal[T]
// 指向完全相同的底层类型（gogpu/ui 仅做了 re-export）。因此 UI 层（gogpu/ui）在
// Phase 4 可直接观察本包产出的 Signal，无需任何转换。
//
// 设计约束（ADR-01）：项目直接复用 gogpu/ui 的 Signal 原语、不做二次封装。这里直接
// 依赖 coregx/signals（而非更重的 gogpu/ui GPU 栈），是为了让基础层保持精简、可离线
// 零 CGO 构建；类型仍与 gogpu/ui 完全一致（同一版本 v0.1.0）。
//
// 线程安全铁律（见 docs/30-State/Signal.md §6）：
//   - Set / Update 仅允许在主线程（desktop.Run 所在、已 LockOSThread 的线程）调用。
//   - Get 为只读，任意线程安全。
//   - Subscribe / SubscribeForever 注册的回调在主线程派发（由上层 OnUpdate 保证）。
type Signal[T any] = signals.Signal[T]

// ReadonlySignal 是 Signal 的只读视图（Get + Subscribe），用于封装——对外暴露只读、
// 内部保留可写，防止外部绕过命令通道直改状态。
type ReadonlySignal[T any] = signals.ReadonlySignal[T]

// NewSignal 创建一个带初始值的可写 Signal。
func NewSignal[T any](initial T) Signal[T] {
	return signals.New[T](initial)
}

// NewSignalWithEqual 创建带自定义相等判定的 Signal：仅当新值与旧值不等时才通知订阅者。
// 适合切片/结构体等需要按内容比较的类型。
func NewSignalWithEqual[T any](initial T, equal func(a, b T) bool) Signal[T] {
	return signals.NewWithOptions(initial, signals.Options[T]{Equal: equal})
}
