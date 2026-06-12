package solid

import (
	"context"
	"sync"
	"sync/atomic"
)

// Compile-time interface check
var (
	_ Signal    = (*signalCond)(nil)
	_ Broadcast = (*broadcastCond)(nil)
)

type signalCond struct {
	count       atomic.Int64
	closed      atomic.Bool
	broadcast   *broadcastCond
	withHistory int64
	mu          sync.Mutex
	cond        *sync.Cond
}

func (s *signalCond) trigger(n int64) {
	// The count update must happen under the mutex: a waiter holds the lock
	// between its last count check and parking in cond.Wait, so updating
	// under the lock guarantees the waiter either sees the new count or is
	// already registered with the cond when Signal fires.
	s.mu.Lock()
	s.count.Add(n)
	s.mu.Unlock()
	s.cond.Signal()
}

func (s *signalCond) close() {
	s.mu.Lock()
	s.closed.Store(true)
	s.mu.Unlock()
	s.cond.Broadcast()
}

func (s *signalCond) decrement() bool {
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

func (s *signalCond) Wait(ctx context.Context) error {
	if s.decrement() {
		return nil
	}

	if s.closed.Load() {
		return ErrSignalNotAvailable
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Only spawn a cancellation watcher when the context can actually be
	// cancelled; context.Background() and friends return a nil Done channel.
	var cancelled atomic.Bool
	if done := ctx.Done(); done != nil {
		stopWatcher := make(chan struct{})
		defer close(stopWatcher)
		go func() {
			select {
			case <-done:
				// Same lost-wakeup protection as trigger/close: update the
				// flag under the mutex before waking the waiters.
				s.mu.Lock()
				cancelled.Store(true)
				s.mu.Unlock()
				s.cond.Broadcast()
			case <-stopWatcher:
			}
		}()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.decrement() {
			return nil
		}
		if s.closed.Load() {
			return ErrSignalNotAvailable
		}
		if cancelled.Load() {
			return ctx.Err()
		}
		s.cond.Wait()
	}
}

func (s *signalCond) Done() {
	s.broadcast.unsubscribe(s)
}

type broadcastCond struct {
	mu          sync.RWMutex
	total       int64
	subscribers map[*signalCond]struct{}
	closed      atomic.Bool
}

func (b *broadcastCond) unsubscribe(s *signalCond) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[s]; exists {
		delete(b.subscribers, s)
		s.close()
	}
}

func (b *broadcastCond) CreateSignal(opts ...SignalOption) Signal {
	if b.closed.Load() {
		return nil
	}

	cfg := defaultSignalConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	s := &signalCond{
		broadcast:   b,
		withHistory: cfg.withHistory,
	}
	s.cond = sync.NewCond(&s.mu)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.Load() {
		return nil
	}

	currentTotal := b.total
	if s.withHistory < 0 {
		s.withHistory = currentTotal
	} else if s.withHistory > currentTotal {
		s.withHistory = currentTotal
	}

	pending := currentTotal - s.withHistory
	if pending > 0 {
		s.count.Store(pending)
	}

	b.subscribers[s] = struct{}{}
	return s
}

func (b *broadcastCond) Notify(n int64) {
	if n <= 0 {
		return
	}

	// Triggering while holding b.mu keeps total and subscriber membership
	// consistent (a signal created at total T sees exactly the notifications
	// after T, never twice) and avoids allocating a snapshot slice.
	// Lock order is always b.mu -> s.mu, matching unsubscribe/Close.
	b.mu.Lock()
	b.total += n
	for s := range b.subscribers {
		s.trigger(n)
	}
	b.mu.Unlock()
}

func (b *broadcastCond) Close() {
	b.closed.Store(true)

	b.mu.Lock()
	subs := b.subscribers
	b.subscribers = make(map[*signalCond]struct{})
	b.mu.Unlock()

	for s := range subs {
		s.close()
	}
}

// NewBroadcastCond creates a new sync.Cond-based broadcaster.
// This implementation uses sync.Cond with atomic operations and is recommended when:
// - Maximum throughput is required (faster than the channel-based implementation)
// - Broadcasting to many signals
// - Context cancellation is infrequent (a blocking Wait with a cancellable
//   context spawns a watcher goroutine; non-cancellable contexts do not)
func NewBroadcastCond(opts ...BroadcastOption) Broadcast {
	cfg := defaultBroadcastConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	b := &broadcastCond{
		total:       cfg.initialTotal,
		subscribers: make(map[*signalCond]struct{}),
	}

	return b
}
