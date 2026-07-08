package platform

import "context"

// TrayCommand 托盘发往主线程的命令。
type TrayCommand int

const (
	CmdShow   TrayCommand = iota // 显示面板
	CmdHide                      // 隐藏面板
	CmdToggle                    // 切换显隐
	CmdQuit                      // 退出应用
)

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
	// Run 在独立 goroutine 启动 systray 消息泵，并把命令写入 cmdCh。
	// ctx 取消时应退出循环并清理。
	Run(ctx context.Context, cmdCh chan<- TrayCommand) error
	// Remove 移除托盘图标并停止消息泵。
	Remove() error
}

// NewTrayManager 构造默认实现（基于 gogpu/systray）。
func NewTrayManager() TrayManager { return newSystrayTrayManager() }
