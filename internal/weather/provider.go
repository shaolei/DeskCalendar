package weather

import "time"

// NewProvider 配置驱动工厂：热插拔核心。
// key 空 → OpenMeteoProvider；key 非空 → QWeatherProvider。
func NewProvider(cfg Config) WeatherProvider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 2
	}
	if trimKey(cfg.QWeatherKey) != "" {
		return NewQWeatherProvider(cfg)
	}
	return NewOpenMeteoProvider(cfg)
}
