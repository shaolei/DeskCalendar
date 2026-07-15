package weather

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
)

// logger 天气包全局日志（默认 Nop，app 可经 SetLogger 开启，用于 key 脱敏审计）。
var logger = log.Nop()

// SetLogger 配置天气包日志输出。nil 被忽略。
func SetLogger(l log.Logger) {
	if l != nil {
		logger = l
	}
}

// Client 网络请求接口（便于单测注入 fake）。
type Client interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// NewClient 创建带超时与有限重试的 Client。
// timeout 作用于每个请求的 context；retries 为额外重试次数（默认 2）。
func NewClient(timeout time.Duration, retries int) Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if retries < 0 {
		retries = 0
	}
	return &httpClient{
		base:        &http.Client{Timeout: timeout},
		retries:     retries,
		baseBackoff: 300 * time.Millisecond,
	}
}

type httpClient struct {
	base        *http.Client
	retries     int
	baseBackoff time.Duration
}

// Do 发送请求：超时由 req 自带的 ctx 控制；遇可重试错误重试 retries 次，
// 指数退避（300ms, 600ms, ...）。4xx 不重试，直接返回 error。
func (c *httpClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		// 每个重试都用独立的、带超时的 ctx 克隆，避免复用已取消的 ctx。
		reqClone := req.Clone(ctx)
		resp, err := c.base.Do(reqClone)
		if err == nil {
			if resp.StatusCode >= 500 {
				resp.Body.Close()
				lastErr = fmt.Errorf("weather: upstream 5xx %d", resp.StatusCode)
				// 落入重试
			} else {
				return resp, nil // 2xx/3xx/4xx 都交上层（4xx 不重试）
			}
		} else {
			lastErr = err
		}
		if attempt < c.retries {
			backoff := c.baseBackoff * time.Duration(attempt+1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, fmt.Errorf("weather: request failed after %d retries: %w", c.retries, lastErr)
}

// BuildOpenMeteoURL 拼 Open-Meteo 请求（免 key）。
// 实况 + 每日预报；timezone=auto 由 API 按坐标推断。
func BuildOpenMeteoURL(loc Location, days int) string {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", loc.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", loc.Lng))
	q.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m,wind_direction_10m,is_day,pressure_msl")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max")
	q.Set("timezone", "auto")
	if days > 0 {
		q.Set("forecast_days", fmt.Sprintf("%d", days))
	}
	return "https://api.open-meteo.com/v1/forecast?" + q.Encode()
}

// BuildQWeatherURL 拼和风请求（带 key，注意 location 是 "lng,lat" 顺序）。
// kind=now 实况；kind=3d/7d 预报。key 必须来自配置，不得硬编码。
func BuildQWeatherURL(loc Location, kind string, key string) string {
	base := "https://devapi.qweather.com/v7/weather/" + kind
	q := url.Values{}
	q.Set("location", fmt.Sprintf("%.4f,%.4f", loc.Lng, loc.Lat)) // 注意：经度在前
	q.Set("key", key)
	return base + "?" + q.Encode()
}

// redactURL 用于日志：把 query 中的 key 值替换为 ***，避免 key 泄露到 stdout/日志。
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	q := u.Query()
	if q.Get("key") != "" {
		q.Set("key", "***REDACTED***")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// readBody 便捷拉取并限制响应体大小（防异常大响应）。
func readBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// hasKeyPlaintext 仅用于测试/审计：判断 raw 中是否仍含明文 key（脱敏是否生效）。
func hasKeyPlaintext(raw, key string) bool {
	if key == "" {
		return false
	}
	return strings.Contains(raw, key)
}
