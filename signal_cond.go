package solid

import (
	"context"
	"sync"
	"sync/atomic"
)

type SignalCond struct {
	count       atomic.Int64 // pending notifications for this signal
	closed      atomic.Bool
	broadcast   *BroadcastCond
	withHistory int64

	// For waiting - we still need cond for blocking
	mu   sync.Mutex
	cond *sync.Cond
}

func (s *SignalCond) trigger() {
	s.count.Add(1)
	s.cond.Signal() // Wake up one waiting goroutine
}

func (s *SignalCond) close() {
	s.closed.Store(true)
	s.cond.Broadcast() // Wake up all waiting goroutines
}

// decrement atomically decrements count if > 0, returns true if decremented
func (s *SignalCond) decrement() bool {
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

// Wait blocks until a signal is received or the context is done
func (s *SignalCond) Wait(ctx context.Context) error {
	// Fast path: check if we already have pending notifications
	if s.decrement() {
		return nil
	}

	// Check if already closed
	if s.closed.Load() {
		return ErrSignalNotAvailable
	}

	// Set up context cancellation watcher
	cancelled := &atomic.Bool{}
	stopWatcher := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancelled.Store(true)
			s.cond.Broadcast()
		case <-stopWatcher:
			return
		}
	}()

	// Slow path: need to wait
	s.mu.Lock()
	for {
		// Check conditions while holding lock (required for cond.Wait)
		if s.decrement() {
			s.mu.Unlock()
			close(stopWatcher)
			return nil
		}
		if s.closed.Load() {
			s.mu.Unlock()
			close(stopWatcher)
			return ErrSignalNotAvailable
		}
		if cancelled.Load() {
			s.mu.Unlock()
			close(stopWatcher)
			return ctx.Err()
		}
		s.cond.Wait()
	}
}

// Done closes the signal and removes it from the broadcaster
func (s *SignalCond) Done() {
	s.broadcast.unsubscribe(s)
}

type BroadcastCond struct {
	mu          sync.RWMutex
	total       int64
	subscribers map[*SignalCond]struct{}
	closed      atomic.Bool
}

func (b *BroadcastCond) unsubscribe(s *SignalCond) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[s]; exists {
		delete(b.subscribers, s)
		s.close()
	}
}

type SignalCondOptFunc func(s *SignalCond)

func WithCondBufferSize(value int) SignalCondOptFunc {
	// Buffer size doesn't apply to sync.Cond implementation
	// Kept for API compatibility
	return func(s *SignalCond) {}
}

func WithCondHistory(baseGen int64) SignalCondOptFunc {
	return func(s *SignalCond) {
		s.withHistory = baseGen
	}
}

// CreateSignal creates a new signal and subscribes it to the broadcaster
func (b *BroadcastCond) CreateSignal(opts ...SignalCondOptFunc) *SignalCond {
	if b.closed.Load() {
		return nil
	}

	s := &SignalCond{
		broadcast:   b,
		withHistory: 0,
	}
	s.cond = sync.NewCond(&s.mu)

	for _, opt := range opts {
		opt(s)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check after acquiring lock
	if b.closed.Load() {
		return nil
	}

	currentTotal := b.total
	if s.withHistory < 0 {
		s.withHistory = currentTotal
	} else if s.withHistory > currentTotal {
		s.withHistory = currentTotal
	}

	// Catch up on any broadcasts since withHistory
	pending := currentTotal - s.withHistory
	if pending > 0 {
		s.count.Store(pending)
	}

	b.subscribers[s] = struct{}{}
	return s
}

// Notify sends a signal to all subscribers
func (b *BroadcastCond) Notify() {
	b.mu.Lock()
	b.total++
	// Copy subscribers to avoid holding lock during trigger
	subs := make([]*SignalCond, 0, len(b.subscribers))
	for s := range b.subscribers {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, s := range subs {
		s.trigger()
	}
}

// Close closes the broadcaster and all signals
func (b *BroadcastCond) Close() {
	b.closed.Store(true)

	b.mu.Lock()
	subs := make([]*SignalCond, 0, len(b.subscribers))
	for s := range b.subscribers {
		subs = append(subs, s)
	}
	b.subscribers = make(map[*SignalCond]struct{})
	b.mu.Unlock()

	for _, s := range subs {
		s.close()
	}
}

type BroadcastCondOptFunc func(b *BroadcastCond)

func WithCondInitialTotal(total int64) BroadcastCondOptFunc {
	return func(b *BroadcastCond) {
		b.total = total
	}
}

// NewBroadcastCond creates a new broadcaster instance using sync.Cond
func NewBroadcastCond(opts ...BroadcastCondOptFunc) *BroadcastCond {
	b := &BroadcastCond{
		subscribers: make(map[*SignalCond]struct{}),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}
