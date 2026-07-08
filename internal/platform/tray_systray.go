package platform

import (
	"context"

	"github.com/gogpu/systray"
)

// systrayTrayManager 基于 gogpu/systray 的真实托盘实现（零 CGO）。
type systrayTrayManager struct {
	tray *systray.SystemTray
}

// newSystrayTrayManager 构造 gogpu/systray 托盘。
func newSystrayTrayManager() TrayManager {
	return &systrayTrayManager{tray: systray.New()}
}

func (m *systrayTrayManager) SetIcon(icon []byte) error {
	m.tray.SetIcon(icon)
	return nil
}

func (m *systrayTrayManager) SetTooltip(tip string) {
	m.tray.SetTooltip(tip)
}

// OnClick 注册左键单击回调（回调内应只向 cmdCh 发命令）。
func (m *systrayTrayManager) OnClick(fn func()) {
	m.tray.OnClick(fn)
}

// Bounds 返回托盘图标的屏幕坐标（物理像素）。
func (m *systrayTrayManager) Bounds() (int, int, int, int) {
	return m.tray.Bounds()
}

// Run 启动 systray 消息泵（阻塞），并装配右键菜单（显示/隐藏、退出）。
// 右键菜单命令经同一 cmdCh 下发；ctx 取消时移除图标并退出泵。
func (m *systrayTrayManager) Run(ctx context.Context, cmdCh chan<- TrayCommand) error {
	menu := systray.NewMenu()
	menu.Add("显示/隐藏", func() { sendTrayCmd(cmdCh, CmdToggle) })
	menu.AddSeparator()
	menu.Add("退出", func() { sendTrayCmd(cmdCh, CmdQuit) })
	m.tray.SetMenu(menu)

	// ctx 取消 → 移除图标使 systray.Run() 返回。
	go func() {
		<-ctx.Done()
		m.tray.Remove()
	}()

	return m.tray.Run()
}

func (m *systrayTrayManager) Remove() error {
	m.tray.Remove()
	return nil
}

// sendTrayCmd 非阻塞发送托盘命令（避免主线程未消费时泄漏 goroutine）。
func sendTrayCmd(ch chan<- TrayCommand, c TrayCommand) {
	select {
	case ch <- c:
	default:
	}
}
