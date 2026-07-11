package animator

import (
	"math"
	"testing"
	"time"
)

func TestEasing_WithinRange(t *testing.T) {
	easings := []Easing{Linear, EaseInQuad, EaseOutQuad, EaseInOutQuad, EaseOutCubic, EaseInOutCubic}
	for _, e := range easings {
		// 端点：t=0→0，t=1→1（无过冲曲线）。
		if got := e(0); got != 0 {
			t.Errorf("easing(0) = %v, want 0", got)
		}
		if got := e(1); got != 1 {
			t.Errorf("easing(1) = %v, want 1", got)
		}
		// 中段单调递增（无过冲曲线应 0<e(t)<1 for 0<t<1）。
		prev := 0.0
		for i := 1; i <= 10; i++ {
			tv := float64(i) / 10.0
			v := e(tv)
			if v <= prev && tv < 1 {
				t.Errorf("easing not strictly increasing at t=%.1f: %v <= %v", tv, v, prev)
			}
			prev = v
		}
	}
}

func TestEasingOutBack_OverShoots(t *testing.T) {
	// EaseOutBack 应在中段过冲 >1（回弹质感）。
	var max float64
	for i := 1; i < 10; i++ {
		v := EaseOutBack(float64(i) / 10.0)
		if v > max {
			max = v
		}
	}
	if max <= 1 {
		t.Errorf("EaseOutBack should overshoot >1, got max %v", max)
	}
}

func TestNew_NormalizesDefaults(t *testing.T) {
	a := New(Spec{}) // 零值：Easing nil、Duration 0
	if a.spec.Easing == nil {
		t.Fatal("Easing should default to Linear")
	}
	if a.spec.Duration <= 0 {
		t.Fatal("Duration should default to >0")
	}
	if a.Active() {
		t.Fatal("new animator should not be active until Start")
	}
}

func TestAnimator_StartTickLifecycle(t *testing.T) {
	a := New(Spec{Duration: 100 * time.Millisecond, Easing: Linear})
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

	// 未 Start：Tick 视为已完成（progress 0, done true）。
	if p, d := a.Tick(now); p != 0 || !d {
		t.Fatalf("pre-start Tick = (%v,%v), want (0,true)", p, d)
	}

	a.Start(now)
	if !a.Active() {
		t.Fatal("Active should be true after Start")
	}

	// 中点（50ms）：线性进度 0.5，进行中。
	p, d := a.Tick(now.Add(50 * time.Millisecond))
	if math.Abs(p-0.5) > 1e-9 || d {
		t.Fatalf("mid Tick = (%v,%v), want (~0.5,false)", p, d)
	}

	// 终点之后：钳制为 1，done true，Active 复位。
	p, d = a.Tick(now.Add(200 * time.Millisecond))
	if p != 1 || !d {
		t.Fatalf("post-end Tick = (%v,%v), want (1,true)", p, d)
	}
	if a.Active() {
		t.Fatal("Active should be false after completion")
	}
}

func TestAnimator_ZeroDurationNormalized(t *testing.T) {
	// New 将 Duration≤0 归一为兜底正时长（零值 Spec 亦可用），故不会是「瞬时」。
	a := New(Spec{Duration: 0})
	if a.spec.Duration <= 0 {
		t.Fatal("Duration 0 should normalize to >0")
	}
	a.Start(time.Now())
	p, d := a.Tick(time.Now().Add(time.Hour)) // 远超时长 → 结束
	if p != 1 || !d {
		t.Fatalf("Tick = (%v,%v), want (1,true)", p, d)
	}
}

func TestAnimator_EasedProgressApplied(t *testing.T) {
	// EaseOutCubic 在中段应快于线性（>t）。
	a := New(Spec{Duration: 100 * time.Millisecond, Easing: EaseOutCubic})
	a.Start(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC))
	p, _ := a.Tick(time.Date(2026, 7, 11, 0, 0, 0, 40*1e6, time.UTC)) // 40% 时间
	if p <= 0.4 {
		t.Fatalf("EaseOutCubic at 40%% time = %v, want >0.4", p)
	}
}
