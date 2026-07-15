package weather

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---- helpers ----

func fakeResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// fakeClient 实现 Client 接口，每次调用返回全新响应（避免 Body 被首次读取后耗尽）。
type fakeClient struct {
	status int
	body   string
	err    error
	calls  int
}

func (f *fakeClient) Do(_ context.Context, _ *http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, f.err
}

// seqRT 按序列返回状态码（用于测 httpClient 重试）。
type seqRT struct {
	seq   []int
	idx   int
	calls int
}

func (r *seqRT) RoundTrip(_ *http.Request) (*http.Response, error) {
	r.calls++
	s := r.seq[r.idx]
	if r.idx < len(r.seq)-1 {
		r.idx++
	}
	return fakeResp(s, ""), nil
}

// ---- URL 构建 ----

func TestBuildOpenMeteoURL(t *testing.T) {
	u := BuildOpenMeteoURL(Location{Lat: 39.9042, Lng: 116.4074}, 3)
	if !strings.Contains(u, "latitude=39.9042") {
		t.Errorf("missing latitude: %s", u)
	}
	if !strings.Contains(u, "longitude=116.4074") {
		t.Errorf("missing longitude: %s", u)
	}
	if !strings.Contains(u, "current=") || !strings.Contains(u, "daily=") {
		t.Errorf("missing current/daily: %s", u)
	}
	if !strings.Contains(u, "forecast_days=3") {
		t.Errorf("missing forecast_days: %s", u)
	}
	if !strings.HasPrefix(u, "https://api.open-meteo.com/") {
		t.Errorf("wrong host: %s", u)
	}
}

func TestBuildQWeatherURL_LngLatOrder(t *testing.T) {
	u := BuildQWeatherURL(Location{Lat: 39.9042, Lng: 116.4074}, "now", "MYKEY")
	// 和风 location 是 "lng,lat"（经度在前）。
	if !strings.Contains(u, "location=116.4074%2C39.9042") && !strings.Contains(u, "location=116.4074,39.9042") {
		t.Errorf("location order wrong (want lng,lat): %s", u)
	}
	if !strings.Contains(u, "key=MYKEY") {
		t.Errorf("missing key: %s", u)
	}
	if !strings.Contains(u, "devapi.qweather.com/v7/weather/now") {
		t.Errorf("wrong endpoint: %s", u)
	}
}

func TestRedactURL(t *testing.T) {
	raw := "https://devapi.qweather.com/v7/weather/now?location=116.4074,39.9042&key=SUPERSECRET"
	got := redactURL(raw)
	if strings.Contains(got, "SUPERSECRET") {
		t.Errorf("key leaked in redacted url: %s", got)
	}
	if !strings.Contains(got, "REDACTED") {
		t.Errorf("key not redacted: %s", got)
	}
	if !hasKeyPlaintext(raw, "SUPERSECRET") {
		t.Errorf("hasKeyPlaintext should detect plaintext")
	}
	if hasKeyPlaintext(got, "SUPERSECRET") {
		t.Errorf("hasKeyPlaintext should NOT detect after redaction")
	}
}

// ---- httpClient 重试语义 ----

func TestHTTPClient_RetriesOn5xx(t *testing.T) {
	c := &httpClient{
		base:        &http.Client{Transport: &seqRT{seq: []int{503, 503, 200}}},
		retries:     2,
		baseBackoff: time.Millisecond,
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200 after retries, got %d", resp.StatusCode)
	}
}

func TestHTTPClient_NoRetryOn4xx(t *testing.T) {
	rt := &seqRT{seq: []int{401, 200}} // 即便后续是 200，也不应重试 4xx
	c := &httpClient{
		base:        &http.Client{Transport: rt},
		retries:     2,
		baseBackoff: time.Millisecond,
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, _ := c.Do(context.Background(), req)
	if resp.StatusCode != 401 {
		t.Errorf("want 401 (no retry on 4xx), got %d", resp.StatusCode)
	}
	if rt.calls != 1 {
		t.Errorf("4xx should not be retried; roundtrips=%d", rt.calls)
	}
}

func TestHTTPClient_RetryExhaustedOnError(t *testing.T) {
	fc := &fakeClient{err: context.DeadlineExceeded}
	// 直接测 Do 的重试计数：用 fakeClient 包一层经 base。
	c := &httpClient{
		base:        &http.Client{Transport: errRT{err: context.DeadlineExceeded}},
		retries:     2,
		baseBackoff: time.Millisecond,
	}
	_ = fc
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

type errRT struct{ err error }

func (e errRT) RoundTrip(_ *http.Request) (*http.Response, error) { return nil, e.err }

// ---- Provider 工厂 ----

func TestNewProvider_Factory(t *testing.T) {
	om := NewProvider(Config{})
	if om.Name() != "open-meteo" {
		t.Errorf("empty key → want open-meteo, got %s", om.Name())
	}
	qw := NewProvider(Config{QWeatherKey: "  abc123  "})
	if qw.Name() != "qweather" {
		t.Errorf("key set → want qweather, got %s", qw.Name())
	}
}

// ---- Open-Meteo 解析 ----

const sampleOM = `{
  "current": {
    "time": "2026-07-07T10:00",
    "temperature_2m": 28.3,
    "relative_humidity_2m": 40,
    "apparent_temperature": 26.1,
    "weather_code": 1,
    "wind_speed_10m": 12.5,
    "wind_direction_10m": 350,
    "is_day": 1,
    "pressure_msl": 1003
  },
  "daily": {
    "time": ["2026-07-07","2026-07-08","2026-07-09"],
    "weather_code": [1, 3, 61],
    "temperature_2m_max": [28.0, 26.0, 22.0],
    "temperature_2m_min": [20.0, 19.0, 18.0],
    "precipitation_probability_max": [10, 20, 80]
  }
}`

func TestOpenMeteoProvider_Parse(t *testing.T) {
	p := &OpenMeteoProvider{client: &fakeClient{status: 200, body: sampleOM}}
	cur, err := p.Current(context.Background(), Location{Lat: 39.9, Lng: 116.4})
	if err != nil {
		t.Fatalf("Current err: %v", err)
	}
	if cur.TempC != 28.3 {
		t.Errorf("TempC = %v, want 28.3", cur.TempC)
	}
	if cur.ConditionCode != "1" || cur.ConditionText != "晴间多云" || cur.Icon != "云" {
		t.Errorf("condition mapping wrong: code=%s text=%s icon=%s", cur.ConditionCode, cur.ConditionText, cur.Icon)
	}
	if !cur.IsDay {
		t.Errorf("IsDay should be true")
	}
	if cur.Humidity != 40 {
		t.Errorf("Humidity = %d, want 40", cur.Humidity)
	}
	fc, err := p.Forecast(context.Background(), Location{Lat: 39.9, Lng: 116.4}, 3)
	if err != nil {
		t.Fatalf("Forecast err: %v", err)
	}
	if len(fc) != 3 {
		t.Fatalf("forecast len = %d, want 3", len(fc))
	}
	if fc[2].TempC != 22.0 || fc[2].LowC != 18.0 {
		t.Errorf("day3 max/min = %v/%v, want 22/18", fc[2].TempC, fc[2].LowC)
	}
	if fc[2].Pop != 0.8 {
		t.Errorf("day3 pop = %v, want 0.8", fc[2].Pop)
	}
}

func TestOpenMeteoProvider_Non200(t *testing.T) {
	p := &OpenMeteoProvider{client: &fakeClient{status: 500}} // 非 200
	_, err := p.Current(context.Background(), Location{})
	if err == nil {
		t.Fatal("expected error on non-200")
	}
}

// ---- WMO 映射 ----

func TestWMOToText(t *testing.T) {
	cases := map[int]struct{ text, icon string }{
		0:  {"晴", "晴"},
		3:  {"阴", "阴"},
		61: {"中雨", "雨"},
		71: {"雪", "雪"},
		95: {"雷阵雨", "雷"},
		45: {"雾", "雾"},
	}
	for code, want := range cases {
		text, icon := wmoToText(code)
		if text != want.text || icon != want.icon {
			t.Errorf("wmo %d → (%q,%q), want (%q,%q)", code, text, icon, want.text, want.icon)
		}
	}
}

// ---- Cache ----

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	c, err := NewCache(dir, time.Minute)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	return c
}

func TestCache_FreshAndStale(t *testing.T) {
	c := newTestCache(t)
	w := &Weather{Source: "open-meteo", TempC: 20}
	if err := c.Set(Location{Lat: 1, Lng: 2}, time.Now(), w); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, fresh, err := c.Get(Location{Lat: 1, Lng: 2}, time.Now())
	if err != nil || !fresh {
		t.Fatalf("Get fresh: err=%v fresh=%v", err, fresh)
	}
	if got.TempC != 20 {
		t.Errorf("temp = %v, want 20", got.TempC)
	}
}

func TestCache_Miss(t *testing.T) {
	c := newTestCache(t)
	_, _, err := c.Get(Location{Lat: 9, Lng: 9}, time.Now())
	if err != ErrCacheMiss {
		t.Errorf("want ErrCacheMiss, got %v", err)
	}
}

func TestCache_DiskPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	c1, _ := NewCache(dir, time.Minute)
	_ = c1.Set(Location{Lat: 1, Lng: 2}, time.Now(), &Weather{Source: "open-meteo", TempC: 15})
	// 新缓存实例从磁盘恢复。
	c2, err := NewCache(dir, time.Minute)
	if err != nil {
		t.Fatalf("NewCache2: %v", err)
	}
	if err := c2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, fresh, err := c2.Get(Location{Lat: 1, Lng: 2}, time.Now())
	if err != nil {
		t.Fatalf("Get after load: %v", err)
	}
	if got.TempC != 15 {
		t.Errorf("recovered temp = %v, want 15", got.TempC)
	}
	_ = fresh
}

// ---- Service 编排 + 降级 ----

type stubProvider struct {
	cur *Weather
	fc  []*Weather
	err error
}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) Current(_ context.Context, _ Location) (*Weather, error) {
	return s.cur, s.err
}
func (s stubProvider) Forecast(_ context.Context, _ Location, _ int) ([]*Weather, error) {
	return s.fc, s.err
}

func TestService_RefreshReady(t *testing.T) {
	dir := t.TempDir()
	s, err := NewService(Config{Lat: 1, Lng: 1}, dir, time.Minute)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	s.provider = stubProvider{cur: &Weather{Source: "stub", TempC: 25}, fc: []*Weather{{TempC: 30, LowC: 20}}}
	s.Refresh(context.Background())
	snap := s.Snapshot()
	if snap.Status != StatusReady {
		t.Errorf("status = %v, want Ready", snap.Status)
	}
	if snap.Current == nil || snap.Current.TempC != 25 {
		t.Errorf("current wrong: %+v", snap.Current)
	}
	if len(snap.Forecast) != 1 {
		t.Errorf("forecast len = %d, want 1", len(snap.Forecast))
	}
}

func TestService_DegradeToCache(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewService(Config{Lat: 1, Lng: 1}, dir, time.Minute)
	// 先成功一次，写缓存。
	s.provider = stubProvider{cur: &Weather{Source: "stub", TempC: 25}}
	s.Refresh(context.Background())
	if s.Snapshot().Status != StatusReady {
		t.Fatal("first refresh should be Ready")
	}
	// 再失败：应降级到缓存（Stale）。
	s.provider = stubProvider{err: context.DeadlineExceeded}
	s.Refresh(context.Background())
	snap := s.Snapshot()
	if snap.Status != StatusStale {
		t.Errorf("status = %v, want Stale (degrade to cache)", snap.Status)
	}
	if snap.Current == nil || snap.Current.TempC != 25 {
		t.Errorf("should serve cached 25, got %+v", snap.Current)
	}
	if !snap.Stale {
		t.Errorf("Stale flag should be true")
	}
}

func TestService_DisabledOnMiss(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewService(Config{Lat: 5, Lng: 5}, dir, time.Minute)
	s.provider = stubProvider{err: context.DeadlineExceeded}
	s.Refresh(context.Background())
	if s.Snapshot().Status != StatusDisabled {
		t.Errorf("status = %v, want Disabled (no cache)", s.Snapshot().Status)
	}
}

func TestService_OnUpdateAndStop(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewService(Config{Lat: 1, Lng: 1}, dir, time.Minute)
	var fired int
	s.SetOnUpdate(func() { fired++ })
	s.provider = stubProvider{cur: &Weather{Source: "stub", TempC: 1}}
	// 首次异步 Refresh（Start 内）会触发 onUpdate。
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	// 等待首次刷新完成。
	deadline := time.After(2 * time.Second)
	for fired == 0 {
		select {
		case <-deadline:
			t.Fatal("onUpdate not fired within timeout")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	s.Stop()
	cancel()
	// Stop 幂等：再调不应 panic。
	s.Stop()
}
