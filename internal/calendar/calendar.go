// Package calendar 是日历领域 feature（MVP 骨架）。
//
// 依赖方向（ADR-07a）：本包仅 import internal/state 来 emit 领域事件，
// 绝不 import internal/plugin。这保证 feature 不反向编译依赖插件层。
package calendar

import (
	"context"
	"time"

	"github.com/shaolei/DeskCalendar/internal/state"
)

// ViewMode 视图模式。
type ViewMode int

const (
	ViewMonth ViewMode = iota // 月视图
	ViewWeek                  // 周视图（可选，见 Week.md；MVP 未启用）
)

// SolarDay 公历日值对象。
type SolarDay struct {
	Date    time.Time
	Year    int
	Month   time.Month
	Day     int
	Weekday time.Weekday
}

// DayInfo 聚合根产出的某日完整信息。
type DayInfo struct {
	Solar     SolarDay
	Lunar     LunarInfo
	Holiday   HolidayInfo
	IsToday   bool
	IsSelected bool
}

// CalendarService 聚合根服务接口（依赖倒置，可 mock）。
type CalendarService interface {
	// GetDayInfo 获取某日完整信息（公历+农历+节气+节假日）。
	GetDayInfo(date time.Time) DayInfo
	// SelectedDate 当前选中日期。
	SelectedDate() time.Time
	// SetSelectedDate 设置选中日期，触发 TopicDateChanged（IsMonth=false）。
	SetSelectedDate(date time.Time)
	// CurrentView 当前视图模式。
	CurrentView() ViewMode
	// SetView 切换视图模式（MVP 周视图未启用，无订阅者；Phase 3 UI 接入时再经 state 广播 ViewModeChanged）。
	SetView(mode ViewMode)
	// VisibleRange 当前视图可见日期范围 [start, end]（闭区间）。
	VisibleRange() (start, end time.Time)
}

// calendarService 默认实现。
type calendarService struct {
	bus      state.EventBus
	selected time.Time
	view     ViewMode
	today    time.Time
	lunar    LunarService
	holiday  HolidayRepository
}

// Option 构造期可选配置。
type Option func(*calendarService)

// WithSelected 指定初始选中日期（默认 time.Now）。
func WithSelected(t time.Time) Option { return func(s *calendarService) { s.selected = t } }

// WithView 指定初始视图模式（默认 ViewMonth）。
func WithView(v ViewMode) Option { return func(s *calendarService) { s.view = v } }

// WithToday 指定“今天”基准（测试用，默认 time.Now）。
func WithToday(t time.Time) Option { return func(s *calendarService) { s.today = t } }

// NewCalendarService 构造聚合根；bus 用于广播日期变更事件（ADR-07a feature→state）。
func NewCalendarService(bus state.EventBus, lunar LunarService, holiday HolidayRepository, opts ...Option) CalendarService {
	now := time.Now()
	s := &calendarService{
		bus:      bus,
		selected: now,
		view:     ViewMonth,
		today:    now,
		lunar:    lunar,
		holiday:  holiday,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// GetDayInfo 组合某日完整信息。
func (s *calendarService) GetDayInfo(date time.Time) DayInfo {
	info := DayInfo{
		Solar:      toSolarDay(date),
		IsToday:    isSameDay(date, s.today),
		IsSelected: isSameDay(date, s.selected),
	}
	if s.lunar != nil {
		info.Lunar = s.lunar.SolarToLunar(date)
	}
	info.Holiday = dayInfo(s.holiday, date)
	return info
}

// SelectedDate 当前选中日期。
func (s *calendarService) SelectedDate() time.Time { return s.selected }

// SetSelectedDate 设置选中日期并经 state 广播（feature → state 方向实体证据）。
func (s *calendarService) SetSelectedDate(date time.Time) {
	s.selected = date
	if s.bus != nil {
		s.bus.Publish(context.Background(), state.Event{
			Topic: state.TopicDateChanged,
			Payload: state.DateChangedPayload{
				Year:    date.Year(),
				Month:   int(date.Month()),
				Day:     date.Day(),
				IsMonth: false,
			},
			At: time.Now(),
		})
	}
}

// CurrentView 当前视图模式。
func (s *calendarService) CurrentView() ViewMode { return s.view }

// SetView 切换视图模式。
func (s *calendarService) SetView(mode ViewMode) { s.view = mode }

// VisibleRange 当前视图可见日期范围。
func (s *calendarService) VisibleRange() (time.Time, time.Time) {
	if s.view == ViewWeek {
		start := weekStart(s.selected, time.Monday)
		return start, start.AddDate(0, 0, 6)
	}
	y, m, _ := s.selected.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	return first, last
}

// toSolarDay 从 time.Time 提取公历日值对象。
func toSolarDay(t time.Time) SolarDay {
	return SolarDay{
		Date:    t,
		Year:    t.Year(),
		Month:   t.Month(),
		Day:     t.Day(),
		Weekday: t.Weekday(),
	}
}
