// Command deskcalendar 是 DeskCalendar 进程的入口，仅做装配后交给 app.Run。
//
// 路径 D：main 不依赖 gogpu/desktop.Run，只加载配置并启动装配后的双循环。
package main

import (
	"fmt"
	"os"

	"github.com/shaolei/DeskCalendar/internal/app"
	"github.com/shaolei/DeskCalendar/internal/infra/config"
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
	if err := app.Run(app.Options{Config: cfg, ConfigPath: cfgPath}); err != nil {
		fmt.Fprintln(os.Stderr, "DeskCalendar:", err)
		os.Exit(1)
	}
}
