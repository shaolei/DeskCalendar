// Command deskcalendar 是 DeskCalendar 进程的入口，仅做装配后交给 app.Run。
//
// 路径 D：main 不依赖 gogpu/desktop.Run，只加载配置并启动装配后的双循环。
//
// 装配逻辑抽出为 buildOptions，便于在 cmd 包内做集成测试（注入 fake 后复用
// app.Options 跑通 app.Run 的「菜单退出」端到端路径，见 main_test.go）。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shaolei/DeskCalendar/build"
	"github.com/shaolei/DeskCalendar/internal/app"
	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/theme"
	"github.com/shaolei/DeskCalendar/internal/todo"
	"github.com/shaolei/DeskCalendar/internal/weather"
)

func main() {
	// --minimized：以仅驻托盘模式启动（用于开机自启，见 docs/20-Platform/Startup.md）。
	// 正常启动（不带该参数）由 app.Run 默认弹窗。
	minimized := flag.Bool("minimized", false, "以最小化（仅驻托盘）模式启动，用于开机自启")
	flag.Parse()

	info := build.Info()
	fmt.Fprintf(os.Stderr, "DeskCalendar %s (commit %s, built %s, %s/%s, cgo=%t)\n",
		info.Version, info.Commit, info.BuildTime, info.TargetOS, info.TargetArch, info.CGOEnabled)
	opts := buildOptions()
	opts.StartMinimized = *minimized
	if err := app.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar:", err)
		os.Exit(1)
	}
}

// buildOptions 装配生产环境依赖（配置/自启/主题/日历）为 app.Options。
//
// 各依赖构造失败时降级为 nil/默认，由 app.Run 的 nil 分支使用对应生产默认实现
// （窗口/托盘仍走真实实现；主题/日历 nil 时菜单开关仅改 config，不渲染）。
func buildOptions() app.Options {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		cfgPath = "config.json"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: load config failed, using default:", err)
		cfg = config.Default()
	}

	// 顶层装配：自启管理器（HKCU Run）与主题供应用（可空——非 Windows/失败时降级）。
	startup, serr := platform.NewStartupManager()
	if serr != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: startup manager unavailable:", serr)
	}
	themeProvider, terr := theme.NewProvider()
	if terr != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: theme provider unavailable:", terr)
	}

	// 日历聚合根：真实农历+节假日内嵌数据；加载失败仅日志降级，仍可运行。
	calendarSvc, cerr := calendar.NewDefaultCalendarService(nil)
	if cerr != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: calendar service unavailable:", cerr)
	}

	// 天气服务（v1.2 EPIC #149）：默认 Open-Meteo 免 key；config 填 QWeatherKey
	// 自动切和风（ADR-05b）。零 CGO、纯 stdlib。创建失败（如缓存目录不可写）
	// 仅日志降级，天气带不显示，不阻塞日历主流程。
	weatherSvc, werr := weather.NewService(weather.Config{
		QWeatherKey: cfg.Weather.QWeatherKey,
		Lat:         cfg.Weather.Lat,
		Lng:         cfg.Weather.Lng,
		Timeout:     8 * time.Second,
		Retries:     2,
	}, weatherCacheDir(), 30*time.Minute)
	if werr != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: weather service unavailable:", werr)
	}

	// 待办服务（v1.1 EPIC #148）：JSON 文件持久化（离线优先、零 CGO、轻量；
	// 后续可无缝替换为 SQLite）。与 config.json 同目录存放 todos.json。
	var todoSvc *todo.Service
	todosPath := filepath.Join(filepath.Dir(cfgPath), "todos.json")
	repo, rerr := todo.NewJSONFileRepository(todosPath)
	if rerr != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar: todo repository unavailable:", rerr)
	} else {
		todoSvc = todo.NewService(repo)
	}

	return app.Options{
		Config:     &cfg,
		ConfigPath: cfgPath,
		Startup:    startup,
		Theme:      themeProvider,
		Calendar:   calendarSvc,
		Weather:    weatherSvc,
		Todo:       todoSvc,
	}
}

// weatherCacheDir 返回天气缓存目录（%LocalAppData%/DeskCalendar/weather-cache），
// 不可用时回退系统临时目录。与 config 目录策略一致（进程重启后可即时降级）。
func weatherCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "DeskCalendar", "weather-cache")
}
