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

// Run 启动 systray 消息泵（阻塞），并渲染 menu 声明的右键菜单。
// 菜单项的回调（显示/隐藏、退出等）由 feature 经 SendCommand 下发命令；
// ctx 取消时移除图标并退出泵。
func (m *systrayTrayManager) Run(ctx context.Context, menu *TrayMenu) error {
	sm := systray.NewMenu()
	m.renderMenu(sm, menu)
	m.tray.SetMenu(sm)

	// ctx 取消 → 移除图标使 systray.Run() 返回。
	go func() {
		<-ctx.Done()
		m.tray.Remove()
	}()

	return m.tray.Run()
}

// renderMenu 将声明式菜单树渲染到 systray 菜单（递归处理子菜单）。
func (m *systrayTrayManager) renderMenu(sm *systray.Menu, menu *TrayMenu) {
	if menu == nil {
		return
	}
	for _, it := range menu.Items {
		if it == nil {
			continue
		}
		switch {
		case it.Separator:
			sm.AddSeparator()
		case it.Submenu != nil:
			sub := systray.NewMenu()
			m.renderMenu(sub, &TrayMenu{Items: it.Submenu})
			sm.AddSubmenu(it.Label, sub)
		case it.OnToggle != nil:
			// gogpu/systray 的 AddCheckbox 回调无参，需本地跟踪切换态，
			// 把「新勾选态」传给 OnToggle（保证多次点击正确翻转）。
			state := it.Checked
			fn := it.OnToggle
			sm.AddCheckbox(it.Label, it.Checked, func() {
				state = !state
				fn(state)
			})
		default:
			fn := it.OnClick
			sm.Add(it.Label, fn)
		}
	}
}

func (m *systrayTrayManager) Remove() error {
	m.tray.Remove()
	return nil
}
