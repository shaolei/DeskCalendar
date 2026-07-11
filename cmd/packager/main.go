// Command packager 是发布期打包工具：把 Build 产出的单个 exe 封装为
// NSIS 安装器与便携版 zip，供 scripts/package.sh / CI release job 调用。
//
// 用法示例：
//
//	go run ./cmd/packager -exe dist/deskcalendar-amd64.exe -arch amd64 -version v1.0.0
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/shaolei/DeskCalendar/build/packaging"
)

func main() {
	var (
		exe       = flag.String("exe", "", "源 exe 路径（必填）")
		arch      = flag.String("arch", "amd64", "架构 amd64/arm64，用于产物命名")
		version   = flag.String("version", "dev", "版本号")
		outdir    = flag.String("outdir", "dist", "产物输出目录")
		icon      = flag.String("icon", "", "安装器/快捷方式图标 .ico（可空）")
		desktop   = flag.Bool("desktop", true, "创建桌面快捷方式")
		startmenu = flag.Bool("startmenu", true, "创建开始菜单项")
		autostart = flag.Bool("autostart", true, "默认勾选开机自动启动")
		makensis  = flag.String("makensis", "makensis", "makensis 可执行路径")
		doNSIS    = flag.Bool("nsis", true, "是否生成 NSIS 安装器（无 makensis 时设 false）")
	)
	flag.Parse()

	if *exe == "" {
		fmt.Fprintln(os.Stderr, "usage: packager -exe <path> [-arch amd64] [-version v1.0.0] ...")
		os.Exit(2)
	}

	cfg := packaging.InstallConfig{
		AppName:                 "DeskCalendar",
		Version:                 *version,
		Arch:                    *arch,
		SourceEXE:               *exe,
		OutDir:                  *outdir,
		IconPath:                *icon,
		CreateDesktopShortcut:   *desktop,
		CreateStartMenuShortcut: *startmenu,
		AutoStart:               *autostart,
	}

	ctx := context.Background()

	pp := packaging.PortablePackager{}
	p, err := pp.Package(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "便携版打包失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("便携版 -> %s\n", p)

	if *doNSIS {
		out, err := packaging.NSISPackager{Makensis: *makensis}.Package(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "NSIS 安装器打包失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("安装器 -> %s\n", out)
	} else {
		fmt.Println("跳过 NSIS（未启用或缺少 makensis）")
	}
}
