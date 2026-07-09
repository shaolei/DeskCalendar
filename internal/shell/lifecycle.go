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
		if win.Visible() {
			l.state = StateHiding
			win.Hide()
		} else {
			l.state = StateShowing
			win.AnchorAboveTray(l.anchor())
			win.Show()
		}
		l.state = StateReady
	case platform.CmdShow:
		if !win.Visible() {
			l.state = StateShowing
			win.AnchorAboveTray(l.anchor())
			win.Show()
			l.state = StateReady
		}
	case platform.CmdHide:
		if win.Visible() {
			l.state = StateHiding
			win.Hide()
			l.state = StateReady
		}
	case platform.CmdQuit:
		win.Hide()
		if l.persist != nil {
			_ = l.persist()
		}
		l.state = StateQuit
		if l.quit != nil {
			l.quit()
		}
	}
}
