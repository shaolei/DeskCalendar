package calendar

import (
	"container/list"
	"time"

	lunar "github.com/6tail/lunar-go/calendar"
)

// lunarGoService 基于 6tail/lunar-go 的实现（纯 Go · MIT · 零 CGO）。
type lunarGoService struct{}

// NewLunarService 构造 lunar-go 封装（零 CGO、纯 Go）。
func NewLunarService() LunarService { return &lunarGoService{} }

// SolarToLunar 将公历时间转换为项目自有 LunarInfo（隔离 lunar-go 类型）。
// 注：lunar-go 的 Lunar.month 对闰月用负数表示，故 LeapMonth = GetMonth()<0，
// 存入 LunarInfo 时月份取绝对值（1-12），闰月由 LeapMonth 布尔标记。
func (s *lunarGoService) SolarToLunar(date time.Time) LunarInfo {
	l := lunar.NewLunarFromDate(date)
	month := l.GetMonth()
	leap := month < 0
	absMonth := month
	if leap {
		absMonth = -month
	}
	return LunarInfo{
		LunarYear:   l.GetYear(),
		LunarMonth:  absMonth,
		LunarDay:    l.GetDay(),
		MonthStr:    l.GetMonthInChinese() + "月", // 库返回"正"/"闰二"，补"月"得"正月"/"闰二月"（冬月/腊月同理）
		DayStr:      l.GetDayInChinese(),
		LeapMonth:   leap,
		SolarTerm:   l.GetJieQi(), // 空字符串表示当日非节气
		GanZhiYear:  l.GetYearInGanZhi(),
		GanZhiMonth: l.GetMonthInGanZhi(),
		GanZhiDay:   l.GetDayInGanZhi(),
		Zodiac:      l.GetYearShengXiao(),
		Festival:    firstOrEmpty(listToStrings(l.GetFestivals())),
		Yi:          listToStrings(l.GetDayYi()),
		Ji:          listToStrings(l.GetDayJi()),
	}
}

// listToStrings 将 lunar-go 的 *list.List 转为 []string（跳过非字符串元素）。
func listToStrings(l *list.List) []string {
	if l == nil {
		return nil
	}
	out := make([]string, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		if s, ok := e.Value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// firstOrEmpty 取切片首元素，空切片返回空串。
func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
