package solid

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrSignalNotAvailable = errors.New("signal not available")

type register struct {
	signal *Signal
	done   chan struct{}
}

type Signal struct {
	// keep track of the number of signals
	// that have been broadcasted
	count       atomic.Int64
	ch          chan struct{}
	broadcast   *Broadcast
	withHistory int64
}

func (s *Signal) trigger() {
	// Always increment count to guarantee notification delivery
	s.count.Add(1)

	// Non-blocking send to wake up waiting goroutine
	select {
	case s.ch <- struct{}{}:
	default:
		// Channel full, but count is already incremented so notification won't be lost
	}
}

// decrement atomically decrements count if > 0, returns true if decremented
func (s *Signal) decrement() bool {
	for {
		current := s.count.Load()
		if current <= 0 {
			return false
		}
		if s.count.CompareAndSwap(current, current-1) {
			return true
		}
		// CAS failed, retry - this is rare under normal conditions
	}
}

// Wait blocks until a signal is received or the context is done
// if signal is created with withHistory set to a base generation, and broadcasts have happened since then
// it will return immediately for each pending and not block. This is useful for cases where you want to
// know how many broadcasts have happened since the base generation. if broadcast is closed or Signal is Done
// it will return ErrSignalNotAvailable
func (s *Signal) Wait(ctx context.Context) error {
	// Fast path: check if we already have pending notifications
	if s.decrement() {
		return nil
	}

	// Slow path: wait for notification
	for {
		select {
		case _, ok := <-s.ch:
			if !ok {
				return ErrSignalNotAvailable
			}
			// Got wakeup signal, try to consume a notification
			if s.decrement() {
				return nil
			}
			// Spurious wakeup (another goroutine consumed it), wait again
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Done closes the signal and removes it from the broadcaster
func (s *Signal) Done() {
	select {
	case s.broadcast.unsubscribe <- s:
		// Successfully sent unsubscribe request
	case <-s.broadcast.done:
		// Broadcaster is closed, nothing to do
	}
}

type Broadcast struct {
	total atomic.Int64
	// Map to store all subscriber channels
	subscribers map[*Signal]struct{}
	// Channel to receive messages for broadcasting
	input chan struct{}
	// Channel to handle unsubscribe requests
	unsubscribe chan *Signal
	// Channel to handle new subscriptions
	subscribe chan *register
	// Channel to stop the broadcaster
	done chan struct{}
}

// loop handles all broadcaster operations
func (b *Broadcast) loop() {
	for {
		select {
		case <-b.input:
			b.total.Add(1)

			// Broadcast message to all subscribers
			for s := range b.subscribers {
				s.trigger()
			}

		case reg := <-b.subscribe:
			currentTotal := b.total.Load()
			if reg.signal.withHistory < 0 {
				reg.signal.withHistory = currentTotal
			} else if reg.signal.withHistory > currentTotal {
				reg.signal.withHistory = currentTotal
			}

			// Catch up on any broadcasts since withHistory
			pending := currentTotal - reg.signal.withHistory
			if pending > 0 {
				reg.signal.count.Store(pending)
			}
			// Add new subscriber
			b.subscribers[reg.signal] = struct{}{}
			// Signal that the CreateSignal that the signal is ready
			close(reg.done)

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

type SignalOptFunc func(s *Signal)

func WithBufferSize(value int) SignalOptFunc {
	return func(s *Signal) {
		s.ch = make(chan struct{}, value)
	}
}

// WithHistory sets the base generation (total count) from which to catch up on broadcasts.
// If not set (default -1), uses the current total at creation time (no historical catch-up, but protects against join-time misses).
// Set to an older value to receive all broadcasts since that point.
func WithHistory(baseGen int64) SignalOptFunc {
	return func(s *Signal) {
		s.withHistory = baseGen
	}
}

// CreateSignal creates a new signal and subscribes it to the broadcaster
// bufferSize is the size of the channel buffer, usually 1 is enough but you can increase it if you want to
// please make sure to test and benchmark it upon increasing the buffer size.
// withHistory if set to a base generation, the signal will catch up on broadcasts since that generation
// this is useful for cases where you want to know how many broadcasts have happened since a specific point
func (b *Broadcast) CreateSignal(opts ...SignalOptFunc) *Signal {
	s := &Signal{
		broadcast:   b,
		withHistory: 0,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.ch == nil {
		s.ch = make(chan struct{}, 1)
	}

	done := make(chan struct{})

	select {
	case b.subscribe <- &register{
		signal: s,
		done:   done,
	}:
		// Successfully sent subscribe request, wait for confirmation
		<-done
		return s
	case <-b.done:
		// Broadcaster is closed
		return nil
	}
}

// Notify sends a signal to all subscribers and unblocks all waiting signals
func (b *Broadcast) Notify() {
	b.input <- struct{}{}
}

// Close closes the broadcaster and all signals
func (b *Broadcast) Close() {
	close(b.done)
}

type BroadCastOptFunc func(b *Broadcast)

// WithInitialTotal sets the initial total count for the broadcaster
// This can be useful for testing or specific use cases where you want
// the broadcaster to start with a predefined count.
func WithInitialTotal(total int64) BroadCastOptFunc {
	return func(b *Broadcast) {
		b.total.Store(total)
	}
}

// NewBroadcast creates a new broadcaster instance
// You can pass BroadCastOptFunc to customize the broadcaster
func NewBroadcast(opts ...BroadCastOptFunc) *Broadcast {
	b := &Broadcast{
		subscribers: make(map[*Signal]struct{}),
		input:       make(chan struct{}),
		unsubscribe: make(chan *Signal),
		subscribe:   make(chan *register),
		done:        make(chan struct{}),
	}

	for _, opt := range opts {
		opt(b)
	}

	go b.loop()
	return b
}
