package eventbus

import "sync"

const defaultBufferSize = 256

// EventBus is an in-process pub/sub system. Mutations publish events;
// subscribers (SSE, webhooks, MCP) receive them on buffered channels.
// Publishing never blocks — if a subscriber's buffer is full, the oldest
// event is dropped.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[*Subscription]struct{}
}

// Subscription receives events on a buffered channel.
type Subscription struct {
	C    <-chan Event
	ch   chan Event
	bus  *EventBus
	once sync.Once
}

// New creates an EventBus with no subscribers.
func New() *EventBus {
	return &EventBus{
		subscribers: make(map[*Subscription]struct{}),
	}
}

// Subscribe creates a new subscription with a buffered channel.
func (b *EventBus) Subscribe() *Subscription {
	ch := make(chan Event, defaultBufferSize)
	sub := &Subscription{C: ch, ch: ch, bus: b}

	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	return sub
}

// Publish sends an event to all subscribers. Non-blocking — if a subscriber's
// buffer is full, the oldest event is dropped to make room.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		select {
		case sub.ch <- event:
		default:
			// Buffer full — drop oldest, send newest.
			<-sub.ch
			sub.ch <- event
		}
	}
}

// Cancel removes the subscription from the bus and drains its channel.
func (s *Subscription) Cancel() {
	s.once.Do(func() {
		s.bus.mu.Lock()
		delete(s.bus.subscribers, s)
		s.bus.mu.Unlock()

		// Drain remaining events.
		for {
			select {
			case <-s.ch:
			default:
				close(s.ch)
				return
			}
		}
	})
}
