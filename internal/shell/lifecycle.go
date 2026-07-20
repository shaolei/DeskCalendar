// Package shell 维护应用 UI 显隐生命周期状态机（路径 D / ADR-08）。
//
// 消费来自托盘的命令（platform.TrayCommand，由 platform.TrayManager.Run 经 channel
// 写入主线程），调用 WindowController 切换显隐/锚定，并在退出前持久化配置。仅主线程
// 驱动 Handle（ADR-02 双循环铁律：tray goroutine 只发命令，绝不直调窗口）。
package shell

import (
	"image"
	"sync"

	"github.com/shaolei/DeskCalendar/internal/platform"
)

// WindowController 是 Lifecycle 驱动窗口所需的窄接口。
// 由 win32.WindowController 结构化满足（其多出的 Present 不影响），本包不 import win32，
// 保持可独立单测。
type WindowController interface {
	Show()
	Hide()
	AnchorAboveTray(rect image.Rectangle)
	Visible() bool
	// Quit 请求窗口退出其消息泵 goroutine（销毁窗口 + 释放 GDI）。app.Run 退出路径
	// 调用，确保窗口线程随进程退出被收口而非泄漏（代码审查 N1）。
	Quit()
}

// State 是应用 UI 生命周期状态。
type State int

const (
	StateBoot State = iota // 构造后、尚未消费任何命令
	StateReady             // 就绪，可显可隐（稳态）
	StateShowing           // 正在显示（瞬态，Handle 内设置后即回到 Ready）
	StateHiding            // 正在隐藏（瞬态）
	StateQuit              // 已退出（幂等）
)

// Lifecycle 维护 UI 显隐状态机并分发托盘命令。仅主线程驱动 Handle。
type Lifecycle struct {
	mu      sync.RWMutex
	state   State
	// desiredVisible 是「期望可见态」的唯一真相源（由主循环同步维护），显隐决策
	// 一律基于它而非 win.Visible()。#151 卡死修复后，窗口操作经 PostMessage 异步派发，
	// win.Visible() 的 caller 乐观原子与窗口线程实际处理存在时序差，且窗口自身「点击外部
	// 自动隐藏」(waInactive) 会改实际可见态却不通知生命周期——若决策依赖 win.Visible()，
	// 异步时序下二者会错位，致「显示/隐藏」切换发出错误命令（窗口卡在显示或隐藏）。
	// 以 desiredVisible 为真相源彻底消除该错位；窗口自动隐藏经 NotifyAutoHidden 回写。
	desiredVisible bool
	anchor  func() image.Rectangle // 取托盘包围盒（物理像素），由 app 接线 tray.Bounds
	persist func() error           // 退出前持久化配置，由 app 接线 config.Save
	quit    func()                 // 触发进程退出，由 app 接线（取消 ctx / os.Exit）
}

// NewLifecycle 构造状态机。anchor 提供托盘矩形；persist/quit 提供退出钩子
// （nil 时对应步骤被安全跳过，便于单测）。
func NewLifecycle(anchor func() image.Rectangle, persist func() error, quit func()) *Lifecycle {
	return &Lifecycle{
		state:   StateBoot,
		anchor:  anchor,
		persist: persist,
		quit:    quit,
	}
}

// State 返回当前状态（线程安全读）。
func (l *Lifecycle) State() State {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

// Handle 必须在主线程（消费 tray 命令的循环）中调用，依据命令驱动状态机与窗口。
// cmd 来自 platform.TrayManager 经 channel 推送的 platform.TrayCommand。
//
// 路径 D 要点：显示时先 AnchorAboveTray(托盘矩形) 再 Show（替代旧 SetPosition+Show）；
// 窗口固定后不再移动，DPI/多屏变化下的重锚由窗口自身在 WM_DPICHANGED 中处理（见 #9）。
func (l *Lifecycle) Handle(cmd platform.TrayCommand, win WindowController) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// 已进入退出则忽略后续所有命令（退出幂等）。
	if l.state == StateQuit {
		return
	}
	switch cmd {
	case platform.CmdToggle:
		// 基于 desiredVisible（真相源）翻转，不读 win.Visible()——避免异步时序下
		// win.Visible() 与窗口实际态错位而发出错误命令（#151 显示/隐藏卡死）。
		if l.desiredVisible {
			l.desiredVisible = false
			l.state = StateHiding
			win.Hide()
		} else {
			l.desiredVisible = true
			l.state = StateShowing
			win.AnchorAboveTray(l.anchor())
			win.Show()
		}
		l.state = StateReady
	case platform.CmdShow:
		// 幂等：已在期望可见态则不重复操作（菜单「显示」项重复点击安全）。
		if !l.desiredVisible {
			l.desiredVisible = true
			l.state = StateShowing
			win.AnchorAboveTray(l.anchor())
			win.Show()
			l.state = StateReady
		}
	case platform.CmdHide:
		// 幂等：已隐藏则不重复操作。
		if l.desiredVisible {
			l.desiredVisible = false
			l.state = StateHiding
			win.Hide()
			l.state = StateReady
		}
	case platform.CmdQuit:
		if l.desiredVisible {
			win.Hide()
		}
		l.desiredVisible = false
		if l.persist != nil {
			_ = l.persist()
		}
		l.state = StateQuit
		if l.quit != nil {
			l.quit()
		}
	}
}

// NotifyAutoHidden 由窗口在「点击外部自动隐藏」(waInactive) 后回调，将期望可见态
// 同步为 false。窗口线程的自动隐藏改的是实际可见态，若不回写 desiredVisible，后续
// 托盘「显示/隐藏」切换会基于过时的意图而吞掉一次切换（#151 显示/隐藏卡死修复：
// 异步 PostMessage 下 win.Visible() 不再可信为决策依据，故以 desiredVisible 为准，
// 此处保证自动隐藏也被状态机感知）。仅主线程消费 dismissedCh 后调用，满足单写者。
func (l *Lifecycle) NotifyAutoHidden() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.desiredVisible = false
}
