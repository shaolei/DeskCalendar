package state

import (
	"sync/atomic"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
)

// Dispatcher 在主线程消费命令并应用到 Store，是单向数据流的落点。
//
// 双循环装配：生产者线程只调用 Enqueue（非阻塞）；主线程 OnUpdate 调用 Pump 排空
// 命令并落到 Store 的 Signal（唯一合法写入点）。
type Dispatcher struct {
	cmdCh  chan Command
	stores *Stores
	logger log.Logger
	// droppedTotal 自启动以来因 channel 满被丢弃的命令总数，便于观测。
	droppedTotal atomic.Uint64
}

// NewDispatcher 创建分发器；channel 带缓冲以避免生产者（如 systray goroutine）阻塞。
// logger 为 nil 时退化为无日志（log.Nop）。
func NewDispatcher(stores *Stores, logger log.Logger) *Dispatcher {
	if logger == nil {
		logger = log.Nop()
	}
	return &Dispatcher{
		cmdCh:  make(chan Command, 64),
		stores: stores,
		logger: logger,
	}
}

// Enqueue 跨线程安全入队（非阻塞）。配合主循环的 RequestRedraw 唤醒，
// 避免生产者因 channel 满而阻塞。
//
// 设计决策（S3 审查结论）：缓冲取 64，而非阻塞等待。理由：
//  1. 命令来自 systray goroutine / 定时器 / 输入意图，全部为「瞬时用户意图」，
//     若阻塞生产者会拖慢托盘响应与主循环；丢一条过期命令比卡住 UI 体验更可接受。
//  2. 在 60fps 主循环 Pump 下，64 条缓冲足以吸收瞬时峰值——实测几乎不可达满。
//  3. 丢弃被计数（DroppedTotal）而非静默：运维可观测，若长期 >0 再上调缓冲。
func (d *Dispatcher) Enqueue(cmd Command) {
	select {
	case d.cmdCh <- cmd:
	default:
		d.droppedTotal.Add(1)
		d.logger.Warn("dispatcher: cmd channel full, dropped", "cmd", cmd.Name())
	}
}

// DroppedTotal 返回自启动以来因 channel 满被丢弃的命令总数。
func (d *Dispatcher) DroppedTotal() uint64 { return d.droppedTotal.Load() }

// Pump 必须在主线程 OnUpdate 中调用：排空当前所有待处理命令。
// 非阻塞：无命令时立即返回，不 spinning。
func (d *Dispatcher) Pump() {
	for {
		select {
		case cmd := <-d.cmdCh:
			d.apply(cmd)
		default:
			return
		}
	}
}

// apply 仅主线程调用：把命令路由到对应 Store（唯一合法写入点）。
func (d *Dispatcher) apply(cmd Command) {
	if d.stores == nil {
		d.logger.Warn("dispatcher: no stores registered", "cmd", cmd.Name())
		return
	}
	switch cmd.(type) {
	case CmdShow, CmdHide, CmdToggle, CmdSetPosition:
		if d.stores.UI != nil {
			d.stores.UI.Apply(cmd)
		}
	case CmdSelectDate, CmdSetViewMode, CmdTick:
		if d.stores.Calendar != nil {
			d.stores.Calendar.Apply(cmd)
		}
	case CmdSetTheme:
		if d.stores.Theme != nil {
			d.stores.Theme.Apply(cmd)
		}
	default:
		d.logger.Warn("dispatcher: unknown command", "cmd", cmd.Name())
	}
}
