package solid

import (
	"context"
	"sync/atomic"
)

// Compile-time interface check
var (
	_ Signal    = (*signalChan)(nil)
	_ Broadcast = (*broadcastChan)(nil)
)

type registerChan struct {
	signal *signalChan
	done   chan struct{}
}

type signalChan struct {
	count       atomic.Int64
	ch          chan struct{}
	broadcast   *broadcastChan
	withHistory int64
}

func (s *signalChan) trigger(n int64) {
	s.count.Add(n)
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *signalChan) decrement() bool {
	for {
		current := s.count.Load()
		if current <= 0 {
			return false
		}
		if s.count.CompareAndSwap(current, current-1) {
			return true
		}
	}
}

func (s *signalChan) Wait(ctx context.Context) error {
	if s.decrement() {
		return nil
	}

	for {
		select {
		case _, ok := <-s.ch:
			if !ok {
				return ErrSignalNotAvailable
			}
			if s.decrement() {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *signalChan) Done() {
	select {
	case s.broadcast.unsubscribe <- s:
	case <-s.broadcast.done:
	}
}

type broadcastChan struct {
	total       atomic.Int64
	subscribers map[*signalChan]struct{}
	input       chan struct{}
	unsubscribe chan *signalChan
	subscribe   chan *registerChan
	done        chan struct{}
}

func (b *broadcastChan) loop() {
	for {
		select {
		case <-b.input:
			b.total.Add(1)
			for s := range b.subscribers {
				s.trigger(1)
			}

		case reg := <-b.subscribe:
			currentTotal := b.total.Load()
			if reg.signal.withHistory < 0 {
				reg.signal.withHistory = currentTotal
			} else if reg.signal.withHistory > currentTotal {
				reg.signal.withHistory = currentTotal
			}

			pending := currentTotal - reg.signal.withHistory
			if pending > 0 {
				reg.signal.count.Store(pending)
			}
			b.subscribers[reg.signal] = struct{}{}
			close(reg.done)

		case s := <-b.unsubscribe:
			delete(b.subscribers, s)
			close(s.ch)

		case <-b.done:
			for s := range b.subscribers {
				delete(b.subscribers, s)
				close(s.ch)
			}
			return
		}
	}
}

func (b *broadcastChan) CreateSignal(opts ...SignalOption) Signal {
	cfg := defaultSignalConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	s := &signalChan{
		broadcast:   b,
		withHistory: cfg.withHistory,
		ch:          make(chan struct{}, cfg.bufferSize),
	}

	done := make(chan struct{})

	select {
	case b.subscribe <- &registerChan{signal: s, done: done}:
		<-done
		return s
	case <-b.done:
		return nil
	}
}

func (b *broadcastChan) Notify(n int64) {
	for n > 0 {
		b.input <- struct{}{}
		n--
	}
}

func (b *broadcastChan) Close() {
	close(b.done)
}

// NewBroadcast creates a new channel-based broadcaster.
// This implementation uses Go channels for signaling and is recommended when:
// - You need native context cancellation support
// - Zero allocations in hot path is important
// - Moderate throughput is acceptable (~550 ns/op per signal)
func NewBroadcast(opts ...BroadcastOption) Broadcast {
	cfg := defaultBroadcastConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	b := &broadcastChan{
		subscribers: make(map[*signalChan]struct{}),
		input:       make(chan struct{}, cfg.bufferSize),
		unsubscribe: make(chan *signalChan),
		subscribe:   make(chan *registerChan),
		done:        make(chan struct{}),
	}
	b.total.Store(cfg.initialTotal)

	go b.loop()
	return b
}
