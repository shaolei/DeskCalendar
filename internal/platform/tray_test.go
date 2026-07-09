package platform

import (
	"context"
	"testing"
	"time"
)

// fakeTrayManager 是测试用 TrayManager：记录回调、回放 Bounds，并提供 SimulateClick。
type fakeTrayManager struct {
	bounds  Rect
	onClick func()
}

func (m *fakeTrayManager) SetIcon(icon []byte) error    { return nil }
func (m *fakeTrayManager) SetTooltip(tip string)        {}
func (m *fakeTrayManager) OnClick(fn func())            { m.onClick = fn }
func (m *fakeTrayManager) Bounds() (int, int, int, int) { return m.bounds.X, m.bounds.Y, m.bounds.W, m.bounds.H }
func (m *fakeTrayManager) Run(ctx context.Context, menu *TrayMenu) error {
	return nil
}
func (m *fakeTrayManager) Remove() error { return nil }

// SimulateClick 触发已注册的单击回调（模拟用户点击托盘）。
func (m *fakeTrayManager) SimulateClick() {
	if m.onClick != nil {
		m.onClick()
	}
}

// TestTrayManager_Contract 编译期校验 fake 与 NewTrayManager 产物满足接口。
func TestTrayManager_Contract(t *testing.T) {
	var _ TrayManager = (*fakeTrayManager)(nil)
	var _ TrayManager = NewTrayManager()
}

// TestTrayManager_ClickSendsCommand 验证点击→回调→命令闭环（边界：命令经 channel 下发）。
func TestTrayManager_ClickSendsCommand(t *testing.T) {
	m := &fakeTrayManager{bounds: Rect{X: 10, Y: 20, W: 24, H: 24}}
	cmdCh := make(chan TrayCommand, 1)
	// shell 注册的点击处理：向主线程 channel 发 CmdToggle。
	m.OnClick(func() { cmdCh <- CmdToggle })
	m.SimulateClick()

	select {
	case c := <-cmdCh:
		if c != CmdToggle {
			t.Errorf("click produced %v, want CmdToggle", c)
		}
	case <-time.After(time.Second):
		t.Fatal("no command received within 1s after SimulateClick")
	}
}

// TestTrayManager_Bounds 验证 Bounds 回放注入的屏幕坐标。
func TestTrayManager_Bounds(t *testing.T) {
	m := &fakeTrayManager{bounds: Rect{X: 10, Y: 20, W: 24, H: 24}}
	x, y, w, h := m.Bounds()
	if x != 10 || y != 20 || w != 24 || h != 24 {
		t.Errorf("Bounds = (%d,%d,%d,%d) want (10,20,24,24)", x, y, w, h)
	}
}

// TestTrayCommand_StableEnumOrder 枚举顺序稳定（下游序列化/测试依赖）。
func TestTrayCommand_StableEnumOrder(t *testing.T) {
	if CmdShow != 0 || CmdHide != 1 || CmdToggle != 2 || CmdQuit != 3 {
		t.Errorf("TrayCommand order changed: %d %d %d %d", CmdShow, CmdHide, CmdToggle, CmdQuit)
	}
}
