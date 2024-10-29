package solid

import (
	"context"
	"errors"
	"sync/atomic"
)

var value struct{}

var (
	ErrSignalNotAvailable = errors.New("signal not available")
)

type register struct {
	signal *Signal
	done   chan struct{}
}

type Signal struct {
	// keep track of the number of signals
	// that have been broadcasted
	count       atomic.Int64
	ch          chan struct{}
	boradcast   *Broadcast
	withHistory bool
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

// Wait blocks until a signal is received or the context is done
// if signal is created with withHistory=true, and broadcasted has already happened
// it will return immediately and not block. This is useful for cases where you want to
// know how many broadcasts have happened since the signal was created. if bloadcasted is closed or Signal is Done
// it will return ErrSignalNotAvailable
func (s *Signal) Wait(ctx context.Context) error {
	if s.hasMore() {
		return nil
	}

	select {
	case _, ok := <-s.ch:
		if !ok {
			return ErrSignalNotAvailable
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done closes the signal and removes it from the broadcaster
func (s *Signal) Done() {
	s.boradcast.unsubscribe <- s
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
			if reg.signal.withHistory {
				reg.signal.count.Store(b.total)
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

// CreateSignal creates a new signal and subscribes it to the broadcaster
// bufferSize is the size of the channel buffer, usually 1 is enough but you can increase it if you want to
// please make sure to test and benchmark it upon increasing the buffer size.
// withHistory if set to true, the signal will keep track of the number of broadcasts that have happened since it was created
// this is useful for cases where you want to know how many broadcasts have happened since the signal was created
func (b *Broadcast) CreateSignal(bufferSize int, withHistory bool) *Signal {
	select {
	case <-b.done:
		return nil
	default:
	}

	s := &Signal{
		ch:          make(chan struct{}, bufferSize),
		boradcast:   b,
		withHistory: withHistory,
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
func (s *Broadcast) Notify() {
	s.input <- struct{}{}
}

// Close closes the broadcaster and all signals
func (b *Broadcast) Close() {
	close(b.done)
}

func NewBroadcast() *Broadcast {
	b := &Broadcast{
		subscribers: make(map[*Signal]struct{}),
		input:       make(chan struct{}),
		unsubscribe: make(chan *Signal),
		subscribe:   make(chan *register),
		done:        make(chan struct{}),
	}
	go b.loop()
	return b
}
