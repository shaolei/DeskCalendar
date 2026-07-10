package platform

import (
	"context"
)

// TrayCommand 托盘发往主线程的命令。
type TrayCommand int

const (
	CmdShow   TrayCommand = iota // 显示面板
	CmdHide                      // 隐藏面板
	CmdToggle                    // 切换显隐
	CmdQuit                      // 退出应用

	// 以下为「配置/主题/刷新」命令，由菜单回调与后台 goroutine 经 SendCommand
	// 投递到主循环；仅主循环（app.Run 的 applyConfigCommand）落地写共享状态，
	// 实现单写者（见代码审查 S1：消除跨 goroutine 直改 Config/Theme/Calendar 竞争）。
	CmdToggleLunar    // 切换「显示农历」
	CmdToggleHoliday  // 切换「显示节假日」
	CmdToggleStartup  // 切换「开机启动」
	CmdThemeLight     // 主题→浅色
	CmdThemeDark      // 主题→深色
	CmdThemeSystem    // 主题→跟随系统
	CmdRefreshToday   // 跨午夜刷新「今天」基准（calendar.RefreshToday）
	CmdRender         // 请求重渲（主题系统切换等，无配置变更）
)

// MenuItem 是托盘右键菜单项的声明式描述（由 feature 提供，platform 仅渲染，
// 不感知 config/theme 等业务语义）。回调在用户交互时于托盘 goroutine 触发。
//
// 三种形态：
//   - 普通项：Separator=false, OnClick!=nil（如 显示/隐藏、退出）
//   - 复选框项：OnToggle!=nil（如 显示农历、开机启动）
//   - 子菜单项：Submenu!=nil（如 主题 → 浅色/深色/跟随）
//   - 分隔线：Separator=true
type MenuItem struct {
	// Label 菜单项文字。
	Label string
	// Separator 为 true 时渲染为分隔线（忽略其余字段）。
	Separator bool
	// Checked 复选框初始勾选态（仅 OnToggle 项生效）。
	Checked bool
	// OnClick 普通项点击回调（非阻塞、短逻辑）。
	OnClick func()
	// OnToggle 复选框状态切换回调；参数为切换后的勾选态。
	OnToggle func(checked bool)
	// Submenu 子菜单项列表（AddSubmenu）。
	Submenu []*MenuItem
}

// TrayMenu 是托盘右键菜单根（声明式菜单树）。
type TrayMenu struct {
	Items []*MenuItem
}

// SendCommand 向命令通道非阻塞发送托盘命令（避免主循环空闲时阻塞托盘 goroutine）。
func SendCommand(ch chan<- TrayCommand, c TrayCommand) {
	select {
	case ch <- c:
	default:
	}
}

// TrayManager 系统托盘管理器（封装 gogpu/systray，纯 Go·零 CGO）。
//
// 铁律（ADR-02）：OnClick 回调只发命令到 cmdCh，绝不跨线程操作窗口；
// 窗口显隐/定位归 shell 主线程。Run 必须在独立 goroutine。
type TrayManager interface {
	// SetIcon 设置托盘图标（二进制 PNG，建议 32x32）。
	SetIcon(icon []byte) error
	// SetTooltip 设置悬停提示。
	SetTooltip(tip string)
	// OnClick 注册左键单击回调；回调在 systray goroutine 触发，
	// 实现内只向 cmdCh 发命令，禁止直接操作窗口。
	OnClick(fn func())
	// Bounds 返回托盘图标的屏幕坐标与尺寸（物理像素）。
	Bounds() (x, y, w, h int)
	// Run 在独立 goroutine 启动 systray 消息泵，并渲染 menu 声明的右键菜单。
	// 菜单项的回调（显示/隐藏、退出等）由 feature 经 SendCommand 向主循环下发
	// platform.TrayCommand；ctx 取消时移除图标并退出泵。
	Run(ctx context.Context, menu *TrayMenu) error
	// Remove 移除托盘图标并停止消息泵。
	Remove() error
}

// NewTrayManager 构造默认实现（基于 gogpu/systray）。
func NewTrayManager() TrayManager { return newSystrayTrayManager() }
