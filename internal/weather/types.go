// Package weather 实现 DeskCalendar 的天气模块（v1.2 EPIC，issue #149）。
//
// 设计边界（70-Weather 设计文档，已按运行时现实裁剪）：
//   - 纯 Go、零 CGO（ADR-06）；仅用标准库 net/http，无第三方 HTTP 库。
//   - 默认 Open-Meteo（免 key）；config 填 QWeatherKey 自动切和风（ADR-05b）。
//   - 30min TTL 缓存（内存 + 磁盘双级），断网/超时优雅降级（→缓存→空），绝不阻塞日历。
//   - 本包是纯数据层：不依赖 ui / platform / app；对外经 Service.Snapshot() 暴露
//     不可变快照，由 app 主循环在重渲时读取并映射为 ui.Model（严守单写者/双循环铁律）。
//   - 不依赖 internal/state（设计文档假设的 weatherSignal 在运行时未被 app/ui 引入，
//     故走「app 持有 Service → 刷新后写 Model.Weather → 重渲」的真实路径）。
package weather

import (
	"context"
	"time"
)

// Location 经纬度坐标（由配置 / 定位传入，本包不负责获取）。
type Location struct {
	Lat float64
	Lng float64
}

// Weather 归一化天气值对象：两个 provider 的响应都映射到此结构，
// UI 与缓存只认这一个类型，实现「调用方零改动」。
type Weather struct {
	Source        string    // "open-meteo" | "qweather"
	ObservedAt    time.Time // 观测时间（已转本地时区）
	TempC         float64   // 当前温度 ℃
	FeelsLikeC    float64   // 体感温度 ℃
	ConditionCode string    // 归一化天气代码（WMO / 和风 code）
	ConditionText string    // 天气文字（"晴"/"多云"...）
	Icon          string    // CJK 单字图标（晴/云/雨/雪/阴/雷/雾），避免 emoji 缺字形
	IsDay         bool      // 是否白天（用于图标/主题）
	Humidity      int       // 相对湿度 %
	WindSpeed     float64   // 风速 km/h
	WindDir       string    // 风向（"北风"等）
	Pressure      int       // 气压 hPa
	Visibility    float64   // 能见度 km
	UVIndex       float64   // 紫外线指数
	Precip        float64   // 降水量 mm
	LowC          float64   // 预报最低温 ℃（实况为 0）
	Pop           float64   // 降水概率 0..1（仅 Forecast）
}

// WeatherProvider 数据源接口（ADR-05b）。
// 任何实现都必须：异步友好（首参 ctx）、不 panic、失败返回 error 而非阻塞。
type WeatherProvider interface {
	Name() string
	Current(ctx context.Context, loc Location) (*Weather, error)
	Forecast(ctx context.Context, loc Location, days int) ([]*Weather, error)
}

// Status 数据新鲜度状态（供 UI 决定显隐/降级提示）。
type Status int

const (
	StatusDisabled Status = iota // 未启用/无网络且无缓存
	StatusLoading                // 刷新中
	StatusReady                  // 新鲜数据
	StatusStale                  // 返回最后缓存（降级）
	StatusError                  // 无缓存且失败
)

// Config 工厂输入：key 决定实现切换。
type Config struct {
	QWeatherKey string  // 空 → Open-Meteo；非空 → 和风
	Lat         float64 // 默认坐标（用户未手动定位时）
	Lng         float64
	Timeout     time.Duration // 单请求超时，默认 8s
	Retries     int           // 失败重试次数，默认 2
}

// trimKey 去首尾空白；空（或纯空白）则视为未配置。
func trimKey(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
