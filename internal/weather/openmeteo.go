package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenMeteoProvider 默认实现（免 key）。
type OpenMeteoProvider struct {
	client Client
}

// NewOpenMeteoProvider 用 Config 构造（超时/重试经 NewClient）。
func NewOpenMeteoProvider(cfg Config) *OpenMeteoProvider {
	return &OpenMeteoProvider{client: NewClient(cfg.Timeout, cfg.Retries)}
}

func (p *OpenMeteoProvider) Name() string { return "open-meteo" }

// openMeteoResponse 是 Open-Meteo /v1/forecast 的归一化子集（current + daily）。
type openMeteoResponse struct {
	Current struct {
		Time                string  `json:"time"`
		Temperature2m       float64 `json:"temperature_2m"`
		RelativeHumidity2m  float64 `json:"relative_humidity_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed10m        float64 `json:"wind_speed_10m"`
		WindDirection10m    float64 `json:"wind_direction_10m"`
		IsDay               int     `json:"is_day"`
		PressureMsl         float64 `json:"pressure_msl"`
	} `json:"current"`
	Daily struct {
		Time                        []string  `json:"time"`
		WeatherCode                 []int     `json:"weather_code"`
		Temperature2mMax            []float64 `json:"temperature_2m_max"`
		Temperature2mMin            []float64 `json:"temperature_2m_min"`
		PrecipitationProbabilityMax []float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

func (p *OpenMeteoProvider) Current(ctx context.Context, loc Location) (*Weather, error) {
	u := BuildOpenMeteoURL(loc, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return nil, err // 上层 Service 转降级
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("open-meteo: status %d", resp.StatusCode)
	}
	body, err := readBody(resp, 256<<10)
	if err != nil {
		return nil, err
	}
	var r openMeteoResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("open-meteo: parse: %w", err)
	}
	text, icon := wmoToText(r.Current.WeatherCode)
	return &Weather{
		Source:        "open-meteo",
		ObservedAt:    parseOpenMeteoTime(r.Current.Time),
		TempC:         r.Current.Temperature2m,
		FeelsLikeC:    r.Current.ApparentTemperature,
		ConditionCode: fmt.Sprintf("%d", r.Current.WeatherCode),
		ConditionText: text,
		Icon:          icon,
		IsDay:         r.Current.IsDay == 1,
		Humidity:      int(r.Current.RelativeHumidity2m),
		WindSpeed:     r.Current.WindSpeed10m,
		WindDir:       windDir(r.Current.WindDirection10m),
		Pressure:      int(r.Current.PressureMsl),
	}, nil
}

func (p *OpenMeteoProvider) Forecast(ctx context.Context, loc Location, days int) ([]*Weather, error) {
	u := BuildOpenMeteoURL(loc, days)
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
		return nil, fmt.Errorf("open-meteo: status %d", resp.StatusCode)
	}
	body, err := readBody(resp, 256<<10)
	if err != nil {
		return nil, err
	}
	var r openMeteoResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("open-meteo: parse: %w", err)
	}
	out := make([]*Weather, 0, len(r.Daily.Time))
	for i := range r.Daily.Time {
		if i >= len(r.Daily.WeatherCode) || i >= len(r.Daily.Temperature2mMax) || i >= len(r.Daily.Temperature2mMin) {
			break
		}
		text, icon := wmoToText(r.Daily.WeatherCode[i])
		var pop float64
		if i < len(r.Daily.PrecipitationProbabilityMax) {
			pop = r.Daily.PrecipitationProbabilityMax[i] / 100.0
		}
		out = append(out, &Weather{
			Source:        "open-meteo",
			ObservedAt:    parseOpenMeteoTime(r.Daily.Time[i]),
			TempC:         r.Daily.Temperature2mMax[i],
			LowC:          r.Daily.Temperature2mMin[i],
			ConditionCode: fmt.Sprintf("%d", r.Daily.WeatherCode[i]),
			ConditionText: text,
			Icon:          icon,
			Pop:           pop,
		})
	}
	return out, nil
}

// parseOpenMeteoTime 解析 Open-Meteo 的本地时间字符串（timezone=auto 返回如
// "2026-07-07T10:00"），按本地时区解释（无 zone 后缀）。解析失败返回零值。
func parseOpenMeteoTime(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// wmoToText 将 WMO 天气代码映射为（中文文字, CJK 单字图标）。
// 图标取最小可分辨单字，避免 emoji 缺字形。
func wmoToText(code int) (text, icon string) {
	switch code {
	case 0:
		return "晴", "晴"
	case 1:
		return "晴间多云", "云"
	case 2:
		return "多云", "云"
	case 3:
		return "阴", "阴"
	case 45, 48:
		return "雾", "雾"
	case 51, 53, 55, 56, 57:
		return "小雨", "雨"
	case 61, 63, 65:
		return "中雨", "雨"
	case 66, 67:
		return "冻雨", "雨"
	case 71, 73, 75, 77:
		return "雪", "雪"
	case 80, 81, 82:
		return "阵雨", "雨"
	case 85, 86:
		return "阵雪", "雪"
	case 95:
		return "雷阵雨", "雷"
	case 96, 99:
		return "雷阵雨伴冰雹", "雷"
	default:
		return "未知", "阴"
	}
}

// windDir 将风向角度转换为中文方位（如 "北风"）。
func windDir(deg float64) string {
	dirs := []string{"北风", "东北风", "东风", "东南风", "南风", "西南风", "西风", "西北风"}
	// 每 45° 一档，北 = 0°。
	idx := int((deg + 22.5) / 45.0)
	idx %= 8
	return dirs[idx]
}
