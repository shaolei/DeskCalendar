package state

import (
	"image/color"
	"time"
)

// ---------------------------------------------------------------------------
// 值对象（value objects）
// ---------------------------------------------------------------------------

// ViewMode 日历视图模式。
type ViewMode int

const (
	ViewMonth ViewMode = iota
	ViewWeek
)

// ThemeMode 主题模式。
type ThemeMode int

const (
	ThemeSystem ThemeMode = iota
	ThemeLight
	ThemeDark
)

// Position 屏幕坐标（托盘上方定位结果）。
type Position struct {
	X, Y int
}

// Size 弹窗尺寸。
type Size struct {
	W, H int
}

// ---------------------------------------------------------------------------
// 接口
// ---------------------------------------------------------------------------

// State 是所有领域状态容器的公共接口（接口隔离，便于 mock/测试）。
type State interface {
	// Snapshot 返回当前状态只读快照，供持久化/调试。
	Snapshot() any
}

// Store 是状态容器的统一接口，可被 Dispatcher 应用命令。
type Store interface {
	State
	// Apply 在主线程执行，把命令结果落到内部 Signal。
	// 非主线程调用会破坏「主线程唯一写入」约定（见 Signal.md §6），
	// 由 DataFlow 保证只在 OnUpdate 内调用。
	Apply(cmd Command)
}

// ---------------------------------------------------------------------------
// CalendarState
// ---------------------------------------------------------------------------

// CalendarState 当前选中日期与视图模式。
type CalendarState struct {
	selectedDate   Signal[time.Time]
	viewMode       Signal[ViewMode]
	displayedMonth Signal[time.Time]
}

// NewCalendarState 构造日历状态（以 now 为初始选中/显示月份）。
func NewCalendarState(now time.Time) *CalendarState {
	return &CalendarState{
		selectedDate: NewSignal(now),
		viewMode:     NewSignal(ViewMonth),
		displayedMonth: NewSignal(time.Date(
			now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())),
	}
}

// SelectedDate 返回选中日期 Signal（主线程唯一写入）。
func (s *CalendarState) SelectedDate() Signal[time.Time] { return s.selectedDate }

// ViewMode 返回视图模式 Signal。
func (s *CalendarState) ViewMode() Signal[ViewMode] { return s.viewMode }

// DisplayedMonth 返回当前显示月份 Signal。
func (s *CalendarState) DisplayedMonth() Signal[time.Time] { return s.displayedMonth }

// Snapshot 返回只读快照。
func (s *CalendarState) Snapshot() any {
	return struct {
		SelectedDate   time.Time
		ViewMode       ViewMode
		DisplayedMonth time.Time
	}{s.selectedDate.Get(), s.viewMode.Get(), s.displayedMonth.Get()}
}

// Apply 在主线程消费与本状态相关的命令。
func (s *CalendarState) Apply(cmd Command) {
	switch c := cmd.(type) {
	case CmdSelectDate:
		s.applySelectDate(c.Date)
	case CmdSetViewMode:
		s.viewMode.Set(c.Mode)
	case CmdTick:
		s.applyTick(c.Now)
	}
}

// applySelectDate 主线程唯一写入点：设置选中日期并联动显示月份。
func (s *CalendarState) applySelectDate(d time.Time) {
	s.selectedDate.Set(d)
	s.displayedMonth.Update(func(old time.Time) time.Time {
		return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
	})
}

// applyTick 由定时器驱动（CmdTick）。MVP 阶段：时间相关的高亮（如"今天"）由视图层
// 在渲染时基于 time.Now() 派生，无需回写 Store；本方法保留作未来钩子（如显式回到
// 当前月、跨午夜刷新等），避免在未导航时擅自覆盖用户当前浏览的月份。
func (s *CalendarState) applyTick(now time.Time) {
	_ = now // Phase 0 占位：时间派生交给视图层。
}

// ---------------------------------------------------------------------------
// ThemeState
// ---------------------------------------------------------------------------

// ThemeState 主题与强调色。
type ThemeState struct {
	mode   Signal[ThemeMode]
	accent Signal[color.RGBA]
}

// NewThemeState 构造主题状态（默认跟随系统、强调色 #4C8DFF）。
func NewThemeState() *ThemeState {
	return &ThemeState{
		mode:   NewSignal(ThemeSystem),
		accent: NewSignal(color.RGBA{R: 0x4C, G: 0x8D, B: 0xFF, A: 0xFF}),
	}
}

// Mode 返回主题模式 Signal。
func (s *ThemeState) Mode() Signal[ThemeMode] { return s.mode }

// Accent 返回强调色 Signal。
func (s *ThemeState) Accent() Signal[color.RGBA] { return s.accent }

// Snapshot 返回只读快照。
func (s *ThemeState) Snapshot() any {
	return struct {
		Mode   ThemeMode
		Accent color.RGBA
	}{s.mode.Get(), s.accent.Get()}
}

// Apply 在主线程消费与本状态相关的命令。
func (s *ThemeState) Apply(cmd Command) {
	if c, ok := cmd.(CmdSetTheme); ok {
		s.mode.Set(c.Mode)
	}
}

// ---------------------------------------------------------------------------
// UIState
// ---------------------------------------------------------------------------

// UIState 弹窗可见性与几何。
type UIState struct {
	visible  Signal[bool]
	position Signal[Position]
	size     Signal[Size]
}

// NewUIState 构造 UI 状态（默认隐藏、320x420）。
func NewUIState() *UIState {
	return &UIState{
		visible:  NewSignal(false),
		position: NewSignal(Position{}),
		size:     NewSignal(Size{W: 320, H: 420}),
	}
}

// Visible 返回可见性 Signal。
func (s *UIState) Visible() Signal[bool] { return s.visible }

// Position 返回坐标 Signal。
func (s *UIState) Position() Signal[Position] { return s.position }

// Size 返回尺寸 Signal。
func (s *UIState) Size() Signal[Size] { return s.size }

// Snapshot 返回只读快照。
func (s *UIState) Snapshot() any {
	return struct {
		Visible  bool
		Position Position
		Size     Size
	}{s.visible.Get(), s.position.Get(), s.size.Get()}
}

// Apply 在主线程消费与本状态相关的命令。
func (s *UIState) Apply(cmd Command) {
	switch c := cmd.(type) {
	case CmdShow:
		s.visible.Set(true)
	case CmdHide:
		s.visible.Set(false)
	case CmdToggle:
		s.visible.Update(func(v bool) bool { return !v })
	case CmdSetPosition:
		s.position.Set(c.Pos)
	}
}
