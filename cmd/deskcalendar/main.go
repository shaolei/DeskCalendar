// Command deskcalendar 是 DeskCalendar 进程的入口，仅做装配后交给 app.Run。
//
// 路径 D：main 不依赖 gogpu/desktop.Run，只加载配置并启动装配后的双循环。
//
// 装配逻辑抽出为 buildOptions，便于在 cmd 包内做集成测试（注入 fake 后复用
// app.Options 跑通 app.Run 的「菜单退出」端到端路径，见 main_test.go）。
package main

import (
	"fmt"
	"os"

	"github.com/shaolei/DeskCalendar/build"
	"github.com/shaolei/DeskCalendar/internal/app"
	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

func main() {
	info := build.Info()
	fmt.Fprintf(os.Stderr, "DeskCalendar %s (commit %s, built %s, %s/%s, cgo=%t)\n",
		info.Version, info.Commit, info.BuildTime, info.TargetOS, info.TargetArch, info.CGOEnabled)
	if err := app.Run(buildOptions()); err != nil {
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

	return app.Options{
		Config:     &cfg,
		ConfigPath: cfgPath,
		Startup:    startup,
		Theme:      themeProvider,
		Calendar:   calendarSvc,
	}
}
