package state

import (
	"context"
	"testing"
)

func TestNewSignalInitialValue(t *testing.T) {
	s := NewSignal(42)
	if s.Get() != 42 {
		t.Fatalf("NewSignal initial value = %d, want 42", s.Get())
	}
}

func TestNewSignal_GetSetRoundTrip(t *testing.T) {
	s := NewSignal("hello")
	if s.Get() != "hello" {
		t.Fatalf("initial = %q, want hello", s.Get())
	}
	s.Set("world")
	if s.Get() != "world" {
		t.Fatalf("after Set = %q, want world", s.Get())
	}
}

// S2 回归：Set 应同步通知订阅者（coregx/signals 在 Set 返回前即派发）。
func TestNewSignalSetNotifiesSubscriber(t *testing.T) {
	s := NewSignal(0)
	var got int
	var notified bool
	unsub := s.Subscribe(context.Background(), func(v int) {
		got = v
		notified = true
	})
	defer unsub()

	s.Set(7)
	if !notified {
		t.Fatal("subscriber not notified on Set")
	}
	if got != 7 {
		t.Fatalf("subscriber got %d, want 7", got)
	}
}

// S2 回归：NewSignalWithEqual 仅在值不等时通知；内容相等的不同切片应被跳过。
func TestNewSignalWithEqualSkipsEqualValues(t *testing.T) {
	equal := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	s := NewSignalWithEqual([]int{1, 2, 3}, equal)

	var count int
	var last []int
	unsub := s.Subscribe(context.Background(), func(v []int) {
		count++
		last = v
	})
	defer unsub()

	// 内容相等（不同切片但值相同）→ 跳过通知
	s.Set([]int{1, 2, 3})
	if count != 0 {
		t.Fatalf("equal value should not notify, got %d notifications", count)
	}

	// 内容不同 → 通知一次
	s.Set([]int{1, 2, 4})
	if count != 1 {
		t.Fatalf("unequal value should notify exactly once, got %d", count)
	}
	if len(last) != 3 || last[2] != 4 {
		t.Fatalf("unexpected last value %v", last)
	}
}
