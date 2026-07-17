package platform

import (
	"context"
	"time"

	"github.com/gogpu/systray"
)

// systrayTrayManager 基于 gogpu/systray 的真实托盘实现（零 CGO）。
//
// 线程模型（#147 修复，真机首跑暴露）：gogpu/systray 的 systray.New() 内部会
// 创建 HWND_MESSAGE 窗口（Create），而该窗口的消息必须由创建它的同一线程的
// 消息泵处理（Win32 铁律）。故 New（Create）/SetIcon/SetTooltip/OnClick/Run
// 全部在 Run 的 goroutine 内完成，杜绝「主 goroutine New + 子 goroutine Run」
// 导致的跨线程——否则图标不显示、点击/TaskbarCreated 回调收不到。
// SetIcon 等在 Run 之前由 app 主 goroutine 调用，此处仅缓存，待 Run goroutine
// 内 New 后应用。
type systrayTrayManager struct {
	tray    *systray.SystemTray
	icon    []byte
	tooltip string
	onClick func()
	ready   chan struct{} // Run 内 New 完成后关闭，通知 app 托盘就绪
}

// newSystrayTrayManager 构造管理器（不立即创建 systray；创建延迟到 Run 的 goroutine）。
func newSystrayTrayManager() TrayManager {
	return &systrayTrayManager{ready: make(chan struct{})}
}

func (m *systrayTrayManager) SetIcon(icon []byte) error {
	m.icon = icon
	return nil
}

func (m *systrayTrayManager) SetTooltip(tip string) {
	m.tooltip = tip
}

// OnClick 注册左键单击回调（回调内应只向 cmdCh 发命令）。
func (m *systrayTrayManager) OnClick(fn func()) {
	m.onClick = fn
}

// Ready 在托盘图标创建后关闭，调用方（app）应在首次 Show 锚定前等待，
// 确保 Bounds() 返回有效托盘矩形（避免窗口被锚定到 (0,0)）。
func (m *systrayTrayManager) Ready() <-chan struct{} {
	return m.ready
}

// Run 在调用它的 goroutine（app 以 go 启动）内：New 创建托盘 + 应用缓存的
// 图标/提示/回调 + 渲染菜单，随后 Run() 同线程泵消息。ctx 取消时移除图标。
func (m *systrayTrayManager) Run(ctx context.Context, menu *TrayMenu) error {
	// 创建与消息泵同线程（见类型注释）。
	t := systray.New()
	m.tray = t
	if len(m.icon) > 0 {
		t.SetIcon(m.icon)
	}
	if m.tooltip != "" {
		t.SetTooltip(m.tooltip)
	}
	if m.onClick != nil {
		t.OnClick(m.onClick)
	}
	sm := systray.NewMenu()
	m.renderMenu(sm, menu)
	t.SetMenu(sm)

	// 关键：gogpu/systray 不会自动加图标，必须显式 Show() 才会
	// Shell_NotifyIconW(NIM_ADD) 把图标加进通知区，并将 internal.visible 置 true。
	// 漏调则：① 托盘永不显示图标；② Bounds() 因 !visible 恒返回 (0,0,0,0)，
	// 导致窗口被锚定到屏幕左上角（真机首跑两个故障的共同根因，#147）。
	// 必须在 close(m.ready) 之前，确保 app 锚定时拿到有效托盘矩形。
	t.Show()

	// 图标已创建并可见：通知 app 可安全 Show 锚定（Bounds 有效）。
	close(m.ready)

	// ctx 取消 → 移除图标使 systray.Run() 返回。
	go func() {
		<-ctx.Done()
		t.Remove()
	}()

	return t.Run()
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
	if m.tray != nil {
		m.tray.Remove()
	}
	return nil
}

func (m *systrayTrayManager) Bounds() (int, int, int, int) {
	if m.tray == nil {
		return 0, 0, 0, 0
	}
	// 图标刚 Show() 后，explorer 通知区可能尚未完成布局，
	// Shell_NotifyIconGetRect 偶发返回 0 矩形。重试几次（每次 20ms，
	// 上限 ~200ms）拿稳定坐标，避免窗口锚定到 (0,0)。
	for i := 0; i < 10; i++ {
		x, y, w, h := m.tray.Bounds()
		if w > 0 && h > 0 {
			return x, y, w, h
		}
		time.Sleep(20 * time.Millisecond)
	}
	return m.tray.Bounds()
}
