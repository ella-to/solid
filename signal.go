package solid

import (
	"context"
	"sync/atomic"
)

var value struct{}

type Signal struct {
	// keep track of the number of signals
	// that have been broadcasted
	count     atomic.Int64
	ch        chan struct{}
	boradcast *Broadcast
}

func (s *Signal) trigger() {
	select {
	case s.ch <- value:
	default:
		s.count.Add(1)
	}
}

func (s *Signal) hasMore() bool {
	for {
		current := s.count.Load()
		if current == 0 {
			return false
		}
		if s.count.CompareAndSwap(current, current-1) {
			return true
		}
	}
}

func (s *Signal) Wait(ctx context.Context) error {
	if s.hasMore() {
		return nil
	}

	select {
	case <-s.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Signal) Done() {
	s.boradcast.unsubscribe <- s
}

type Broadcast struct {
	// Map to store all subscriber channels
	subscribers map[*Signal]struct{}
	// Channel to receive messages for broadcasting
	input chan struct{}
	// Channel to handle unsubscribe requests
	unsubscribe chan *Signal
	// Channel to handle new subscriptions
	subscribe chan *Signal
	// Channel to stop the broadcaster
	done chan struct{}
}

// loop handles all broadcaster operations
func (b *Broadcast) loop() {
	for {
		select {
		case <-b.input:
			// Broadcast message to all subscribers
			for s := range b.subscribers {
				s.trigger()
			}

		case s := <-b.subscribe:
			// Add new subscriber
			b.subscribers[s] = struct{}{}

		case s := <-b.unsubscribe:
			// Remove subscriber
			delete(b.subscribers, s)
			close(s.ch)

		case <-b.done:
			// Clean up all subscribers
			for s := range b.subscribers {
				delete(b.subscribers, s)
				close(s.ch)
			}

			return
		}
	}
}

func (b *Broadcast) CreateSignal(bufferSize int) *Signal {
	s := &Signal{
		ch:        make(chan struct{}, bufferSize),
		boradcast: b,
	}
	b.subscribe <- s
	return s
}

func (s *Broadcast) Broadcast() {
	s.input <- struct{}{}
}

func (b *Broadcast) Close() {
	close(b.done)
}

func NewBroadcast() *Broadcast {
	b := &Broadcast{
		subscribers: make(map[*Signal]struct{}),
		input:       make(chan struct{}),
		unsubscribe: make(chan *Signal),
		subscribe:   make(chan *Signal),
		done:        make(chan struct{}),
	}
	go b.loop()
	return b
}
