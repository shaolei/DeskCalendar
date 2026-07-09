// Command deskcalendar 是 DeskCalendar 进程的入口，仅做装配后交给 app.Run。
//
// 路径 D：main 不依赖 gogpu/desktop.Run，只加载配置并启动装配后的双循环。
package main

import (
	"fmt"
	"os"

	"github.com/shaolei/DeskCalendar/internal/app"
	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

func main() {
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

	if err := app.Run(app.Options{
		Config:     &cfg,
		ConfigPath: cfgPath,
		Startup:    startup,
		Theme:      themeProvider,
		Calendar:   calendarSvc,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar:", err)
		os.Exit(1)
	}
}
