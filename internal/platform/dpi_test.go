package platform

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestScaleLogicalToPhysical(t *testing.T) {
	s := NewDPIScaler()
	cases := []struct {
		logical, dpi, want int
	}{
		{360, 96, 360},  // 100%
		{360, 144, 540}, // 150%
		{360, 192, 720}, // 200%
		{480, 96, 480},
		{100, 125, 130}, // 非整数缩放：round(100*125/96)=round(130.2)=130
	}
	for _, c := range cases {
		got := s.ScaleLogicalToPhysical(c.logical, c.dpi)
		if got != c.want {
			t.Errorf("ScaleLogicalToPhysical(%d,%d)=%d want %d", c.logical, c.dpi, got, c.want)
		}
	}
}

func TestScalePhysicalToLogical(t *testing.T) {
	s := NewDPIScaler()
	cases := []struct {
		physical, dpi, want int
	}{
		{540, 144, 360},
		{720, 192, 360},
		{360, 96, 360},
		{130, 125, 100}, // round(130*96/125)=round(99.84)=100
	}
	for _, c := range cases {
		got := s.ScalePhysicalToLogical(c.physical, c.dpi)
		if got != c.want {
			t.Errorf("ScalePhysicalToLogical(%d,%d)=%d want %d", c.physical, c.dpi, got, c.want)
		}
	}
}

func TestScaleRoundTrip(t *testing.T) {
	s := NewDPIScaler()
	// 物理↔逻辑在整数倍下应往返一致。
	phys := s.ScaleLogicalToPhysical(360, 144)
	back := s.ScalePhysicalToLogical(phys, 144)
	if back != 360 {
		t.Errorf("round-trip 360@144: physical=%d back=%d want 360", phys, back)
	}
}

// --- 切片 3：SetAwareness / EffectiveDPI 经 backend seam ---

// fakeDPIBackend 是测试用 backend，记录调用并回放配置值。
type fakeDPIBackend struct {
	gotAwareness DPIAwareness
	setErr       error
	dpiX, dpiY   int
	dpiErr       error
}

func (f *fakeDPIBackend) setAwareness(a DPIAwareness) error {
	f.gotAwareness = a
	return f.setErr
}

func (f *fakeDPIBackend) effectiveDPI() (int, int, error) {
	return f.dpiX, f.dpiY, f.dpiErr
}

func TestDPIScaler_SeamDelegatesToBackend(t *testing.T) {
	fake := &fakeDPIBackend{dpiX: 144, dpiY: 144}
	s := &defaultDPIScaler{backend: fake}

	if err := s.SetAwareness(context.Background(), DPIPerMonitorAwareV2); err != nil {
		t.Fatalf("SetAwareness unexpected error: %v", err)
	}
	if fake.gotAwareness != DPIPerMonitorAwareV2 {
		t.Errorf("backend.setAwareness got %v want %v", fake.gotAwareness, DPIPerMonitorAwareV2)
	}

	x, y, err := s.EffectiveDPI()
	if err != nil {
		t.Fatalf("EffectiveDPI unexpected error: %v", err)
	}
	if x != 144 || y != 144 {
		t.Errorf("EffectiveDPI got (%d,%d) want (144,144)", x, y)
	}
}

func TestDPIScaler_SeamPropagatesSetAwarenessError(t *testing.T) {
	fake := &fakeDPIBackend{setErr: errBoom}
	s := &defaultDPIScaler{backend: fake}
	if err := s.SetAwareness(context.Background(), DPISystemAware); !errors.Is(err, errBoom) {
		t.Errorf("SetAwareness error not propagated: got %v", err)
	}
}

// errBoom 测试用哨兵错误。
var errBoom = errors.New("boom")

func TestNewDPIScaler_WindowsWiresBackend(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backend wiring only meaningful on windows")
	}
	s := NewDPIScaler()
	ds, ok := s.(*defaultDPIScaler)
	if !ok {
		t.Fatalf("NewDPIScaler returned %T, want *defaultDPIScaler", s)
	}
	if ds.backend == nil {
		t.Error("expected non-nil dpi backend on windows (SetAwareness/EffectiveDPI would be no-ops)")
	}
}
