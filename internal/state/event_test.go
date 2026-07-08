package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/infra/log"
)

func TestSubscribeAndPublish(t *testing.T) {
	bus := NewEventBus(log.Nop())
	var mu sync.Mutex
	var received bool
	var gotYear int

	unsub, err := bus.Subscribe(TopicDateChanged, func(ctx context.Context, e Event) error {
		if p, ok := e.Payload.(DateChangedPayload); ok {
			mu.Lock()
			received = true
			gotYear = p.Year
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if unsub == nil {
		t.Fatal("expected non-nil Unsubscribe")
	}

	bus.Publish(context.Background(), Event{
		Topic:   TopicDateChanged,
		Payload: DateChangedPayload{Year: 2026, Month: 7, Day: 8},
	})

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("handler was not called on Publish")
	}
	if gotYear != 2026 {
		t.Fatalf("payload year = %d, want 2026", gotYear)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	bus := NewEventBus(log.Nop())
	var mu sync.Mutex
	count := 0
	inc := func(ctx context.Context, e Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	}

	unsub, err := bus.Subscribe(TopicDateChanged, inc)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	bus.Publish(context.Background(), Event{Topic: TopicDateChanged})
	unsub()
	bus.Publish(context.Background(), Event{Topic: TopicDateChanged})

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("handler invoked %d times after unsubscribe, want 1", count)
	}
}

func TestHandlerErrorIsolated(t *testing.T) {
	bus := NewEventBus(log.Nop())
	var mu sync.Mutex
	goodCalled := false

	bad := func(ctx context.Context, e Event) error { return errors.New("boom") }
	good := func(ctx context.Context, e Event) error {
		mu.Lock()
		goodCalled = true
		mu.Unlock()
		return nil
	}

	bus.Subscribe(TopicDateChanged, bad)
	bus.Subscribe(TopicDateChanged, good)
	bus.Publish(context.Background(), Event{Topic: TopicDateChanged})

	mu.Lock()
	defer mu.Unlock()
	if !goodCalled {
		t.Fatal("good handler was blocked by a preceding handler error")
	}
}

func TestPanicIsolated(t *testing.T) {
	bus := NewEventBus(log.Nop())
	var mu sync.Mutex
	goodCalled := false

	boom := func(ctx context.Context, e Event) error { panic("handler panic") }
	good := func(ctx context.Context, e Event) error {
		mu.Lock()
		goodCalled = true
		mu.Unlock()
		return nil
	}

	bus.Subscribe(TopicDateChanged, boom)
	bus.Subscribe(TopicDateChanged, good)
	// Must not propagate the panic to the caller.
	bus.Publish(context.Background(), Event{Topic: TopicDateChanged})

	mu.Lock()
	defer mu.Unlock()
	if !goodCalled {
		t.Fatal("good handler was blocked by a panicking handler")
	}
}

func TestPublishAsync(t *testing.T) {
	bus := NewEventBus(log.Nop())
	var mu sync.Mutex
	var received bool
	done := make(chan struct{})

	bus.Subscribe(TopicDateChanged, func(ctx context.Context, e Event) error {
		mu.Lock()
		received = true
		mu.Unlock()
		close(done)
		return nil
	})

	bus.PublishAsync(context.Background(), Event{Topic: TopicDateChanged})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async handler was not invoked within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Fatal("async handler did not receive the event")
	}
}

func TestNilHandlerRejected(t *testing.T) {
	bus := NewEventBus(log.Nop())
	if _, err := bus.Subscribe(TopicDateChanged, nil); err == nil {
		t.Fatal("expected error when subscribing a nil handler")
	}
}
