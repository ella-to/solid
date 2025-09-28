package solid

import (
	"context"
	"errors"
	"sync/atomic"
)

var value struct{}

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
	// Use a unified approach: always use count, use channel only for wakeup
	wasZero := s.count.Add(1) == 1

	// If this was the first notification (count went from 0 to 1),
	// try to wake up a waiting goroutine via channel
	if wasZero {
		select {
		case s.ch <- value:
			// Successfully sent wakeup signal
		default:
			// Channel full, but that's okay since count has the notification
		}
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

// Wait blocks until a signal is received or the context is done
// if signal is created with withHistory=true, and broadcasted has already happened
// it will return immediately and not block. This is useful for cases where you want to
// know how many broadcasts have happened since the signal was created. if bloadcasted is closed or Signal is Done
// it will return ErrSignalNotAvailable
func (s *Signal) Wait(ctx context.Context) error {
	// Check if we have pending notifications
	if s.hasMore() {
		return nil
	}

	// Wait for wakeup signal
	select {
	case _, ok := <-s.ch:
		if !ok {
			return ErrSignalNotAvailable
		}
		// Channel received, now consume from count
		if s.hasMore() {
			return nil
		}
		// This shouldn't happen with our new trigger logic, but handle gracefully
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
	total int64
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
			b.total++

			// Broadcast message to all subscribers
			for s := range b.subscribers {
				s.trigger()
			}

		case reg := <-b.subscribe:
			if reg.signal.withHistory > -1 {
				total := b.total - reg.signal.withHistory
				if total >= 0 {
					reg.signal.count.Store(total)
				}
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

func WithHistory(count int64) SignalOptFunc {
	return func(s *Signal) {
		s.withHistory = count
	}
}

// CreateSignal creates a new signal and subscribes it to the broadcaster
// bufferSize is the size of the channel buffer, usually 1 is enough but you can increase it if you want to
// please make sure to test and benchmark it upon increasing the buffer size.
// withHistory if set to true, the signal will keep track of the number of broadcasts that have happened since it was created
// this is useful for cases where you want to know how many broadcasts have happened since the signal was created
func (b *Broadcast) CreateSignal(opts ...SignalOptFunc) *Signal {
	select {
	case <-b.done:
		return nil
	default:
	}

	s := &Signal{
		broadcast:   b,
		withHistory: -1,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.ch == nil {
		s.ch = make(chan struct{}, 1)
	}

	done := make(chan struct{})

	b.subscribe <- &register{
		signal: s,
		done:   done,
	}

	// need to do this to make sure the history and count sets before the signal is returned
	<-done

	return s
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

func WithInitialTotal(total int64) BroadCastOptFunc {
	return func(b *Broadcast) {
		b.total = total
	}
}

func NewBroadcast(opts ...BroadCastOptFunc) *Broadcast {
	b := &Broadcast{
		subscribers: make(map[*Signal]struct{}),
		input:       make(chan struct{}),
		unsubscribe: make(chan *Signal),
		subscribe:   make(chan *register),
		done:        make(chan struct{}),
		total:       0,
	}

	for _, opt := range opts {
		opt(b)
	}

	go b.loop()
	return b
}
