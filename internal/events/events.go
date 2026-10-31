// Package events is an in-process pub/sub used to push changes to dashboard clients over SSE.
package events

import (
	"sync"
)

type Type string

const (
	DeviceCreated Type = "device.created" // a new (pending) device appeared
	DeviceUpdated Type = "device.updated" // settings or status changed
	DeviceDeleted Type = "device.deleted"
	DeviceSeen    Type = "device.seen" // device polled /api/display (new telemetry)
	DeviceLogs    Type = "device.logs" // new log entries arrived
)

type Event struct {
	Type     Type  `json:"type"`
	DeviceID int64 `json:"device_id,omitempty"`
}

// Broker fans events out to subscribers. Slow subscribers drop events rather than block
// publishers; clients refetch on reconnect anyway.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[chan Event]struct{})}
}

func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel of events and a function that ends the subscription.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
		})
	}
}
