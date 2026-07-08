package state

import "time"

// Command 是所有状态变更的归一化指令（单向数据流载体）。
//
// 任意线程都可构造 Command，但只有 Dispatcher.apply（主线程）可消费它。生产者
// （tray goroutine / 定时器 / 网络回调）只负责 Enqueue，绝不直接调用 Signal.Set。
type Command interface {
	// Name 返回命令名，用于日志/追踪。
	Name() string
}

// ---- 具体命令 ----

// CmdShow 显示面板。
type CmdShow struct{}

func (CmdShow) Name() string { return "show" }

// CmdHide 隐藏面板。
type CmdHide struct{}

func (CmdHide) Name() string { return "hide" }

// CmdToggle 切换面板显隐。
type CmdToggle struct{}

func (CmdToggle) Name() string { return "toggle" }

// CmdSelectDate 选中某日期。
type CmdSelectDate struct{ Date time.Time }

func (CmdSelectDate) Name() string { return "select_date" }

// CmdSetViewMode 设置视图模式（月/周）。
type CmdSetViewMode struct{ Mode ViewMode }

func (CmdSetViewMode) Name() string { return "set_view_mode" }

// CmdSetTheme 设置主题模式。
type CmdSetTheme struct{ Mode ThemeMode }

func (CmdSetTheme) Name() string { return "set_theme" }

// CmdSetPosition 设置面板坐标（托盘上方定位结果）。
type CmdSetPosition struct{ Pos Position }

func (CmdSetPosition) Name() string { return "set_position" }

// CmdTick 由定时器驱动，用于刷新随时间变化的状态。
type CmdTick struct{ Now time.Time }

func (CmdTick) Name() string { return "tick" }

// Stores 聚合所有领域状态容器（由 app/shell 装配时注入）。
type Stores struct {
	Calendar *CalendarState
	Theme    *ThemeState
	UI       *UIState
}
