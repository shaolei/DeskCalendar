package platform

import "context"

// DPIAwareness 进程 DPI 感知模式。
type DPIAwareness int

const (
	DPIUnaware          DPIAwareness = iota // 不感知（不采用）
	DPISystemAware                         // 系统级感知
	DPIPerMonitorAware                     // 每显示器感知
	DPIPerMonitorAwareV2                   // 每显示器 V2（推荐，支持非整数缩放）
)

// DefaultAwareness 返回推荐感知模式：PerMonitorV2。
func DefaultAwareness() DPIAwareness { return DPIPerMonitorAwareV2 }

// DPIScaler 提供 DPI 感知声明与坐标换算。
// 实现方封装零 CGO 的 Win32 DPI API（SetProcessDpiAwarenessContext 等）。
type DPIScaler interface {
	// SetAwareness 在进程启动早期（创建窗口前）声明感知模式。
	SetAwareness(ctx context.Context, a DPIAwareness) error
	// EffectiveDPI 返回当前主窗口有效 DPI（x,y，通常相等）。
	EffectiveDPI() (x, y int, err error)
	// ScaleLogicalToPhysical 逻辑坐标(基于 96 DPI)→物理像素。
	ScaleLogicalToPhysical(logical, dpi int) int
	// ScalePhysicalToLogical 物理像素→逻辑坐标。
	ScalePhysicalToLogical(physical, dpi int) int
}

// NewDPIScaler 构造默认实现（自动装配平台 backend：Windows 用真实 DPI 系统调用，
// 非 Windows 留 nil 使 SetAwareness 为 no-op、EffectiveDPI 回落 96）。
func NewDPIScaler() DPIScaler { return &defaultDPIScaler{backend: newPlatformDPIBackend()} }

// defaultDPIScaler 默认 DPI 换算实现。
// 真实 SetAwareness/EffectiveDPI 由 setAwarenessBackend seam 提供（见 dpi_windows.go）。
type defaultDPIScaler struct {
	backend dpiBackend
}

// dpiBackend 封装底层 DPI 系统调用（seam，便于测试注入 fake）。
type dpiBackend interface {
	setAwareness(a DPIAwareness) error
	effectiveDPI() (int, int, error)
}

// ScaleLogicalToPhysical 逻辑坐标(96 DPI 基准)→物理像素。
//   physical = round(logical * dpi / 96)
func (s *defaultDPIScaler) ScaleLogicalToPhysical(logical, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return int(float64(logical*dpi) / 96.0 + 0.5)
}

// ScalePhysicalToLogical 物理像素→逻辑坐标(96 DPI 基准)。
//   logical = round(physical * 96 / dpi)
func (s *defaultDPIScaler) ScalePhysicalToLogical(physical, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return int(float64(physical*96) / float64(dpi) + 0.5)
}

// SetAwareness 委托 backend 声明进程 DPI 感知。
func (s *defaultDPIScaler) SetAwareness(ctx context.Context, a DPIAwareness) error {
	if s.backend == nil {
		return nil // 无 backend（如测试默认）视为 no-op
	}
	return s.backend.setAwareness(a)
}

// EffectiveDPI 委托 backend 取有效 DPI；无 backend 时回落系统默认 96。
func (s *defaultDPIScaler) EffectiveDPI() (int, int, error) {
	if s.backend == nil {
		return 96, 96, nil
	}
	return s.backend.effectiveDPI()
}
