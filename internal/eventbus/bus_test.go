package eventbus

import (
	"sync"
	"testing"
	"time"
)

func event(typ string) Event {
	return Event{Type: typ, Timestamp: time.Now()}
}

func TestPublishNoSubscribers(t *testing.T) {
	bus := New()
	bus.Publish(event("task.created")) // should not panic
}

func TestSubscribeReceivesEvent(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer sub.Cancel()

	bus.Publish(event("task.created"))

	select {
	case e := <-sub.C:
		if e.Type != "task.created" {
			t.Errorf("expected task.created, got %s", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := New()
	sub1 := bus.Subscribe()
	sub2 := bus.Subscribe()
	defer sub1.Cancel()
	defer sub2.Cancel()

	bus.Publish(event("task.created"))

	for i, sub := range []*Subscription{sub1, sub2} {
		select {
		case e := <-sub.C:
			if e.Type != "task.created" {
				t.Errorf("sub %d: expected task.created, got %s", i, e.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timeout", i)
		}
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	sub.Cancel()

	bus.Publish(event("task.created"))

	// Channel should be closed, reads should return zero value immediately.
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		// Channel is closed, no more events.
	}
}

func TestDoubleCancelNoPanic(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	sub.Cancel()
	sub.Cancel() // should not panic
}

func TestRingBufferOverflow(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer sub.Cancel()

	// Fill the buffer.
	for i := 0; i < defaultBufferSize; i++ {
		bus.Publish(Event{Type: "fill", Detail: i})
	}

	// Publish one more — should drop oldest (i=0) and deliver newest.
	bus.Publish(Event{Type: "overflow", Detail: defaultBufferSize})

	// First event should be i=1 (i=0 was dropped).
	e := <-sub.C
	if e.Detail.(int) != 1 {
		t.Errorf("expected oldest-after-drop (1), got %v", e.Detail)
	}
}

func TestEventOrdering(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer sub.Cancel()

	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: "seq", Detail: i})
	}

	for i := 0; i < 10; i++ {
		e := <-sub.C
		if e.Detail.(int) != i {
			t.Errorf("expected %d, got %v", i, e.Detail)
		}
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := New()
	sub := bus.Subscribe()
	defer sub.Cancel()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(event("concurrent"))
		}()
	}
	wg.Wait()

	// Drain and count — should have received up to 100 events (or 256 buffer max).
	count := 0
	for {
		select {
		case <-sub.C:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("expected some events from concurrent publish")
	}
}

func TestConcurrentSubscribeCancel(t *testing.T) {
	bus := New()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := bus.Subscribe()
			bus.Publish(event("race"))
			sub.Cancel()
		}()
	}
	wg.Wait()
}

func TestPublishDoesNotBlock(t *testing.T) {
	bus := New()
	_ = bus.Subscribe() // subscribe but never consume

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			bus.Publish(event("flood"))
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — publish completed without blocking.
	case <-time.After(5 * time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}
}
