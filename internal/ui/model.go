// Package ui 是 DeskCalendar 的渲染层（路径 D / ADR-08 / #90-UI）。
//
// 职责一句话：把日历领域数据（calendar.MonthGrid）与主题（theme.Theme）光栅化为
// 一张实心不透明的 *image.RGBA，经 internal/platform/win32 的 WindowController.Present
// 推送至弹窗。MVP 为不透明方角面板（圆角/阴影推 v1.1/v1.3），渲染后端为
// github.com/gogpu/gg（纯 Go·零 CGO CPU 光栅）。
//
// 依赖方向（ADR-07a）：本包 import internal/calendar（仅取 MonthGrid 值对象）、
// internal/theme（取 *Theme 值对象）、image/color、gg。绝不 import platform/win32/
// app/shell —— 像素推送由调用方（app）负责，保持渲染层可独立单测。
package ui

import (
	"fmt"
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
)

// Cell 是单格的展示视图模型（已扁平化自 calendar.Cell，剔除 UI 不需要的域细节）。
type Cell struct {
	Day        int    // 公历日
	InMonth    bool   // 是否属于当前月（补位格=false）
	IsToday    bool   // 是否今天（★ 高亮）
	IsSelected bool   // 是否选中
	Lunar      string // 农历小字（初一/节气/节日）；ShowLunar=false 时为空
	Holiday    string // 节假日名（元旦…）；ShowHoliday=false 时为空
	IsHoliday  bool   // 法定节假日（ShowHoliday 生效且 IsHoliday）
	IsWorkday  bool   // 调休补班日（ShowHoliday 生效且 IsWorkday）
}

// Model 是 Render 所需的完整展示模型（与 calendar 域解耦的视图模型）。
type Model struct {
	Year        int
	Month       time.Month
	MonthLabel  string       // "2026年7月"
	Weekdays    [7]string    // 表头，按 grid.WeekStart 旋转为「第 0 列 = 周首」（如周一→一二三四五六日），与网格列对齐（S2）
	Weeks       [6][7]Cell   // 6 行 7 列网格
	ShowLunar   bool         // 显示农历小字
	ShowHoliday bool         // 高亮节假日
	Weather     *WeatherCard // 顶部天气卡片；nil 时不显示天气带（不挤压日历）

	// 以下字段为 v1.1 待办视图（#148）引入，由 app 在重渲时填充；
	// 不依赖 internal/todo（ui 保持不反向依赖域包，ADR-07a）。
	ViewMode ViewMode    // 当前视图：日历 / 待办
	Draft    string      // 待办输入框草稿（键盘录入经 app 主循环写入）
	Editing  bool        // 输入框是否处于编辑态（聚焦）
	Todos    []*TodoItem // 待办列表（由 app 从 todo.Service 映射而来，已含 Overdue/DueSoon 计算值）
}

// ViewMode 当前面板视图（Tab 切换）。
type ViewMode int

const (
	// ViewCalendar 日历视图（默认）。
	ViewCalendar ViewMode = iota
	// ViewTodo 待办视图（v1.1 #148）。
	ViewTodo
)

// TodoItem 待办列表的展示视图模型（由 app 从 todo.Todo 映射，ui 不反向依赖
// internal/todo，避免依赖方向破坏，ADR-07a）。Overdue/DueSoon 由 app 在映射时
// 用 todo.Todo 的领域方法结合当前时刻算出，TodoView 仅消费。
type TodoItem struct {
	ID         string     // 待办 ID（点击命中后 app 据此定位领域对象）
	Title      string     // 标题
	Due        *time.Time // 截止时间，nil 表示无期限
	Status     string     // "active" / "done"（与 todo.Status 字符串一致）
	Tags       []string   // 标签
	ReminderAt *time.Time // 提醒时间，nil 表示不提醒
	Overdue    bool       // 是否已延期（active 且 Due 已过）
	DueSoon    bool       // 是否即将到期/已到提醒（用于高亮，非必须）
}

// WeatherStatus 天气加载状态（驱动降级态）。
type WeatherStatus int

const (
	WeatherLoading WeatherStatus = iota // 刷新中
	WeatherReady                        // 有数据（新鲜或降级旧数据）
	WeatherError                        // 无网络且无缓存：整块降级
)

// WeatherItem 单条天气展示（当前实况或某日预报），CJK 图标避免 emoji 缺字形。
type WeatherItem struct {
	TempC         float64 // 当前温度℃（预报时为该日最高温）
	LowC          float64 // 预报最低温℃（实况为 0）
	ConditionText string  // 天气文字：晴/多云/雨…
	Icon          string  // CJK 单字图标：晴/云/雨/雪/阴/雷/雾
	IsDay         bool    // 是否白天（实况）
	Pop           float64 // 降水概率 0..1（预报）
	HasRange      bool    // 是否含 LowC（预报项）
}

// WeatherCard 面板顶部天气卡片（含降级态）。由 app 从 weather.Service.Snapshot()
// 映射而来，保持 ui 不反向依赖 internal/weather（依赖方向约束 ADR-07a）。
type WeatherCard struct {
	Status   WeatherStatus
	Current  *WeatherItem
	Forecast []*WeatherItem
	Source   string // "open-meteo" / "qweather"
	Stale    bool   // 降级旧数据（显示「·旧数据」角标）
}

// WeekdayLabels 中文星期表头（以周日为第 0 列，按 time.Weekday 索引：日=0…六=6）。
var WeekdayLabels = [7]string{"日", "一", "二", "三", "四", "五", "六"}

// rotateWeekdays 以 weekStart 为第 0 列重排中文星期表头，使表头与网格列对齐（S2）。
// WeekdayLabels 按 time.Weekday 索引，故列 i 的星期 = (weekStart + i) % 7。
func rotateWeekdays(weekStart time.Weekday) [7]string {
	var out [7]string
	for i := 0; i < 7; i++ {
		out[i] = WeekdayLabels[(int(weekStart)+i)%7]
	}
	return out
}

// NewMonthModel 由 calendar.MonthGrid 构建展示模型。showLunar/showHoliday 控制
// 农历小字与节假日高亮的显隐（来自 config.Display）。纯函数，易单测。
func NewMonthModel(grid calendar.MonthGrid, showLunar, showHoliday bool) Model {
	m := Model{
		Year:        grid.Year,
		Month:       grid.Month,
		MonthLabel:  fmt.Sprintf("%d年%d月", grid.Year, int(grid.Month)),
		Weekdays:    rotateWeekdays(grid.WeekStart),
		ShowLunar:   showLunar,
		ShowHoliday: showHoliday,
	}
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			src := grid.Weeks[r][c]
			cell := Cell{
				Day:        src.Date.Day(),
				InMonth:    src.InCurrentMonth,
				IsToday:    src.IsToday,
				IsSelected: src.IsSelected,
			}
			if showLunar {
				cell.Lunar = lunarText(src.Lunar)
			}
			if showHoliday {
				cell.Holiday = src.Holiday.Name
				cell.IsHoliday = src.Holiday.IsHoliday
				cell.IsWorkday = src.Holiday.IsWorkday
			}
			m.Weeks[r][c] = cell
		}
	}
	return m
}

// lunarText 选择格内农历小字优先级：节气 > 农历节日 > 农历日（初一/十五…）。
func lunarText(l calendar.LunarInfo) string {
	if l.SolarTerm != "" {
		return l.SolarTerm
	}
	if l.Festival != "" {
		return l.Festival
	}
	return l.DayStr
}
