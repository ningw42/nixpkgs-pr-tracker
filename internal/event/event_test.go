package event

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishNoSubscribers(t *testing.T) {
	bus := New()
	// Should not panic
	bus.Publish(Event{Type: PRAdded, PRNumber: 1})
}

func TestPublishSingleSubscriber(t *testing.T) {
	bus := New()

	var received Event
	bus.Subscribe(func(e Event) {
		received = e
	})

	sent := Event{
		Type:      PRMerged,
		PRNumber:  42,
		Title:     "test pr",
		Author:    "user1",
		Timestamp: time.Now(),
	}
	bus.Publish(sent)

	if received.Type != sent.Type {
		t.Errorf("Type = %q, want %q", received.Type, sent.Type)
	}
	if received.PRNumber != sent.PRNumber {
		t.Errorf("PRNumber = %d, want %d", received.PRNumber, sent.PRNumber)
	}
	if received.Title != sent.Title {
		t.Errorf("Title = %q, want %q", received.Title, sent.Title)
	}
}

func TestPublishMultipleSubscribers(t *testing.T) {
	bus := New()

	var count1, count2 int
	bus.Subscribe(func(e Event) { count1++ })
	bus.Subscribe(func(e Event) { count2++ })

	bus.Publish(Event{Type: PRAdded, PRNumber: 1})

	if count1 != 1 {
		t.Errorf("subscriber 1 count = %d, want 1", count1)
	}
	if count2 != 1 {
		t.Errorf("subscriber 2 count = %d, want 1", count2)
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := New()

	var count atomic.Int64
	bus.Subscribe(func(e Event) {
		count.Add(1)
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{Type: PRAdded, PRNumber: 1})
		}()
	}
	wg.Wait()

	if count.Load() != 100 {
		t.Errorf("count = %d, want 100", count.Load())
	}
}
