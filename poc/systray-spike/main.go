// spike: gogpu/ui 空窗 + gogpu/systray + channel 显隐 + Bounds() 定位
//
// 目标：验证两条消息循环不冲突——
//   1) gogpu/ui 主循环（desktop.Run → gogpuApp.Run，主线程 LockOSThread）
//   2) systray.Run()（独立 goroutine，自己的 HWND_MESSAGE 消息泵）
// 并通过 channel 把托盘点击（OnClick）转成主线程上的窗口显隐/定位操作。
//
// 架构约束（来自评估文档）：
//   - systray.OnClick 在 systray goroutine 触发，禁止直接操作 gogpu/ui 窗口；
//     只向 channel 发命令，由主循环（OnUpdate，运行在 Run 的主线程）执行。
//   - 主窗口隐藏后 gogpu 循环在 WaitEvents 阻塞（0% CPU）；点击托盘时用
//     gogpuApp.RequestRedraw() 唤醒主循环，OnUpdate 再处理命令。
package main

import (
	"encoding/base64"
	"log"
	"time"

	_ "github.com/gogpu/gg/gpu" // 启用 GPU SDF 加速

	"github.com/gogpu/gogpu"
	"github.com/gogpu/systray"
	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/desktop"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

// 托盘图标（32x32 PNG，base64）。仅用于本 spike。
const iconB64 = "iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAb0lEQVR42u3Xuw0AIQwDUE/CSgzBOgzINjAAx1cErJOLlMivQXHgQsovBwIIsPvQx1yNOeArtDXHASvhKwhYhc8iYBk+g4B1+AiBG+E9hAC4Fd5CCCCAviEv4PkuoNiGFH2AohFRdEKKVqzD5JeAAsqLKqi0arVlAAAAAElFTkSuQmCC"

const (
	winW = 360
	winH = 480
)

func main() {
	icon, err := base64.StdEncoding.DecodeString(iconB64)
	if err != nil {
		log.Printf("icon decode: %v", err)
	}

	// 透明根背景（圆角面板之外透出桌面，沿用 POC 验证过的机制）
	m3 := material3.New(widget.Hex(0x1e63d8))
	th := m3.AsTheme()
	th.Colors.Background = widget.RGBA8(0, 0, 0, 0)

	gogpuApp := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("DeskCalendar").
		WithSize(winW, winH).
		WithFrameless(true))

	uiApp := app.New(
		app.WithWindowProvider(gogpuApp),
		app.WithPlatformProvider(gogpuApp),
		app.WithEventSource(gogpuApp.EventSource()),
		app.WithTheme(th),
	)
	uiApp.SetRoot(buildUI())

	// ---- 命令通道：systray 点击 / 模拟点击 只发命令，主循环执行 ----
	cmd := make(chan string, 16)
	sendCmd := func(c string) {
		select {
		case cmd <- c:
		default:
			log.Printf("cmd channel full, drop: %s", c)
		}
		gogpuApp.RequestRedraw() // 唤醒主循环（隐藏态时 WaitEvents 阻塞）
	}

	// ---- 托盘 ----
	tray := systray.New()
	tray.SetIcon(icon).SetTooltip("DeskCalendar POC")
	menu := systray.NewMenu()
	menu.Add("显示/隐藏", func() { sendCmd("toggle") })
	menu.AddSeparator()
	menu.Add("退出", func() {
		tray.Remove()
		sendCmd("quit")
	})
	tray.SetMenu(menu)
	tray.OnClick(func() { sendCmd("toggle") })
	tray.Show()
	go tray.Run() // ★ 独立 goroutine 跑 systray 消息泵

	// ---- 主循环：OnUpdate 运行在 Run 的主线程，安全操作窗口 ----
	gogpuApp.OnUpdate(func(dt float64) {
		for {
			select {
			case c := <-cmd:
				handleCmd(c, gogpuApp, tray)
			default:
				return
			}
		}
	})

	// ---- 模拟用户点击序列（无需真实鼠标，验证 channel→窗口 闭环）----
	// 真实产品里这些命令来自 tray.OnClick；这里用定时器驱动同一通道。
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("[demo] click -> hide")
		sendCmd("toggle")

		time.Sleep(2 * time.Second)
		log.Println("[demo] click -> show")
		sendCmd("toggle")

		time.Sleep(2 * time.Second)
		log.Println("[demo] position near tray")
		sendCmd("position")

		time.Sleep(2 * time.Second)
		log.Println("[demo] click -> hide (end)")
		sendCmd("toggle")

		time.Sleep(1 * time.Second)
		log.Println("[demo] quit")
		sendCmd("quit")
	}()

	log.Println("[main] desktop.Run start")
	if err := desktop.Run(gogpuApp, uiApp); err != nil {
		log.Fatal(err)
	}
	log.Println("[main] desktop.Run exit")
}

// handleCmd 在主线程（OnUpdate 调用方）执行窗口操作。
func handleCmd(c string, g *gogpu.App, tray *systray.SystemTray) {
	win := g.PrimaryWindow()
	switch c {
	case "toggle":
		if win == nil {
			return
		}
		if win.Visible() {
			win.Hide()
			log.Println("[cmd] window HIDDEN")
		} else {
			win.Show()
			log.Println("[cmd] window SHOWN")
		}
	case "position":
		if win == nil {
			return
		}
		x, y, w, h := tray.Bounds()
		log.Printf("[cmd] tray.Bounds x=%d y=%d w=%d h=%d", x, y, w, h)
		// 让窗口底边贴近托盘图标上方居中
		posX := x + w/2 - winW/2
		posY := y - winH - 8
		if posY < 0 { // 上方空间不足则放到图标下方
			posY = y + h + 8
		}
		if posX < 0 {
			posX = 0
		}
		win.SetPosition(posX, posY)
		win.Show()
		log.Printf("[cmd] window positioned at (%d,%d)", posX, posY)
	case "quit":
		log.Println("[cmd] quit")
		g.Quit()
	}
}

func buildUI() widget.Widget {
	return primitives.Box(
		primitives.Text("DeskCalendar").
			FontSize(22).
			Bold().
			Color(widget.RGBA8(255, 255, 255, 255)),
		primitives.Text("托盘日历 · 最小集成 spike").
			FontSize(13).
			Color(widget.RGBA8(220, 230, 255, 255)),
		primitives.Text("点击托盘图标切换显隐").
			FontSize(11).
			Color(widget.RGBA8(200, 215, 255, 255)),
	).
		Padding(24).
		Gap(10).
		Background(widget.RGBA8(0x1e, 0x63, 0xd8, 255)).
		Rounded(16).
		ShadowLevel(3)
}
