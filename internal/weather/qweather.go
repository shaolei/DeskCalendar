package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// QWeatherProvider 用户填 key 后自动切换的实现（中国精度最佳）。
type QWeatherProvider struct {
	client Client
	key    string
}

// NewQWeatherProvider 用 Config 构造（key 必须非空；超时/重试经 NewClient）。
func NewQWeatherProvider(cfg Config) *QWeatherProvider {
	return &QWeatherProvider{
		client: NewClient(cfg.Timeout, cfg.Retries),
		key:    trimKey(cfg.QWeatherKey),
	}
}

func (p *QWeatherProvider) Name() string { return "qweather" }

// qwNowResp 和风实况接口（/v7/weather/now）响应。
type qwNowResp struct {
	Code string `json:"code"`
	Now  struct {
		ObsTime    string `json:"obsTime"`
		Temp       string `json:"temp"`
		FeelsLike  string `json:"feelsLike"`
		Icon       string `json:"icon"`
		Text       string `json:"text"`
		WindDir    string `json:"windDir"`
		WindScale  string `json:"windScale"`
		WindSpeed  string `json:"windSpeed"`
		Humidity   string `json:"humidity"`
		Pressure   string `json:"pressure"`
		Visibility string `json:"vis"`
	} `json:"now"`
}

// qwDailyResp 和风逐日预报接口（/v7/weather/3d 或 /7d）响应。
type qwDailyResp struct {
	Code  string `json:"code"`
	Daily []struct {
		FxDate  string `json:"fxDate"`
		TempMax string `json:"tempMax"`
		TempMin string `json:"tempMin"`
		TextDay string `json:"textDay"`
		IconDay string `json:"iconDay"`
		Precip  string `json:"precip"`
	} `json:"daily"`
}

func (p *QWeatherProvider) Current(ctx context.Context, loc Location) (*Weather, error) {
	u := BuildQWeatherURL(loc, "now", p.key)
	logger.Debug("qweather request", "url", redactURL(u)) // 脱敏日志
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, fmt.Errorf("qweather: invalid key (status %d)", resp.StatusCode) // 不重试
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("qweather: status %d", resp.StatusCode)
	}
	body, err := readBody(resp, 256<<10)
	if err != nil {
		return nil, err
	}
	var r qwNowResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("qweather: parse: %w", err)
	}
	if r.Code != "200" {
		return nil, fmt.Errorf("qweather: api code %s", r.Code)
	}
	temp, _ := strconv.ParseFloat(r.Now.Temp, 64)
	feels, _ := strconv.ParseFloat(r.Now.FeelsLike, 64)
	hum, _ := strconv.Atoi(r.Now.Humidity)
	press, _ := strconv.Atoi(r.Now.Pressure)
	spd, _ := strconv.ParseFloat(r.Now.WindSpeed, 64)
	vis, _ := strconv.ParseFloat(r.Now.Visibility, 64)
	obs, _ := time.Parse(time.RFC3339, r.Now.ObsTime)
	return &Weather{
		Source:        "qweather",
		ObservedAt:    obs,
		TempC:         temp,
		FeelsLikeC:    feels,
		ConditionCode: r.Now.Icon,
		ConditionText: r.Now.Text,
		Icon:          textToIcon(r.Now.Text),
		IsDay:         true,
		Humidity:      hum,
		WindSpeed:     spd,
		WindDir:       r.Now.WindDir + "风",
		Pressure:      press,
		Visibility:    vis,
	}, nil
}

func (p *QWeatherProvider) Forecast(ctx context.Context, loc Location, days int) ([]*Weather, error) {
	kind := "3d"
	if days > 3 {
		kind = "7d"
	}
	u := BuildQWeatherURL(loc, kind, p.key)
	logger.Debug("qweather request", "url", redactURL(u))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("qweather: status %d", resp.StatusCode)
	}
	body, err := readBody(resp, 256<<10)
	if err != nil {
		return nil, err
	}
	var r qwDailyResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("qweather: parse: %w", err)
	}
	if r.Code != "200" {
		return nil, fmt.Errorf("qweather: api code %s", r.Code)
	}
	out := make([]*Weather, 0, len(r.Daily))
	for _, d := range r.Daily {
		maxT, _ := strconv.ParseFloat(d.TempMax, 64)
		minT, _ := strconv.ParseFloat(d.TempMin, 64)
		out = append(out, &Weather{
			Source:        "qweather",
			ObservedAt:    parseQWeatherDate(d.FxDate),
			TempC:         maxT,
			LowC:          minT,
			ConditionCode: d.IconDay,
			ConditionText: d.TextDay,
			Icon:          textToIcon(d.TextDay),
		})
	}
	return out, nil
}

// parseQWeatherDate 解析和风日期（"2006-07-07"）为本地零时。
func parseQWeatherDate(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// textToIcon 由和风中文天气文字推导 CJK 单字图标（取最小可分辨单字）。
func textToIcon(text string) string {
	for _, pair := range []struct {
		sub  string
		icon string
	}{
		{"晴", "晴"}, {"雷", "雷"}, {"雪", "雪"}, {"雾", "雾"},
		{"雨", "雨"}, {"阴", "阴"}, {"云", "云"},
	} {
		if contains(text, pair.sub) {
			return pair.icon
		}
	}
	return "阴"
}

// contains 是 strings.Contains 的轻量替代（避免为单函数引入 strings 依赖的额外 import 噪音）。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
