// Package calendar 是日历领域 feature（MVP）。
//
// 依赖方向（ADR-07a）：本包仅 import internal/state 来 emit 领域事件，
// 绝不 import internal/plugin / internal/platform / internal/theme。
// 农历换算经 LunarService 接口依赖倒置（真实实现见 lunar_lunargolang.go，零 CGO）。
package calendar

import "time"

// LunarInfo 农历信息值对象（项目自有，隔离第三方库类型）。
// 真实映射见 lunar_lunargolang.go 的 lunarGoService.SolarToLunar。
type LunarInfo struct {
	LunarYear   int
	LunarMonth  int
	LunarDay    int
	MonthStr    string   // "正月"
	DayStr      string   // "初一"
	LeapMonth   bool     // 是否闰月
	SolarTerm   string   // 节气，如 "清明"，无则空
	GanZhiYear  string   // "甲辰"
	GanZhiMonth string   // "庚午"
	GanZhiDay   string   // "癸酉"
	Zodiac      string   // 生肖 "龙"
	Festival    string   // 农历节日 "中秋节"，无则空
	Yi          []string // 宜
	Ji          []string // 忌
}

// LunarService 农历服务接口（依赖倒置，可 mock）。
type LunarService interface {
	// SolarToLunar 将公历时间转换为农历信息。
	SolarToLunar(date time.Time) LunarInfo
}
