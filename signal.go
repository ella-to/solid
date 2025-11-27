package solid

import (
	"context"
	"errors"
)

var ErrSignalNotAvailable = errors.New("signal not available")

// Signal represents a subscriber that can wait for broadcast notifications.
// Implementations must be safe for concurrent use.
type Signal interface {
	// Wait blocks until a signal is received or the context is done.
	// Returns nil when a notification is received.
	// Returns ErrSignalNotAvailable if the signal or broadcaster is closed.
	// Returns ctx.Err() if the context is cancelled.
	Wait(ctx context.Context) error

	// Done closes the signal and removes it from the broadcaster.
	// After Done is called, Wait will return ErrSignalNotAvailable.
	Done()
}

// Broadcast represents a broadcaster that can notify multiple subscribers.
// Implementations must be safe for concurrent use.
type Broadcast interface {
	// CreateSignal creates a new signal and subscribes it to the broadcaster.
	// Returns nil if the broadcaster is closed.
	CreateSignal(opts ...SignalOption) Signal

	// Notify sends a signal to all subscribers, unblocking any waiting signals.
	Notify()

	// Close closes the broadcaster and all associated signals.
	// After Close is called, CreateSignal returns nil and all signals return ErrSignalNotAvailable.
	Close()
}

// SignalOption configures a Signal when created.
type SignalOption func(*signalConfig)

type signalConfig struct {
	bufferSize  int
	withHistory int64
}

func defaultSignalConfig() *signalConfig {
	return &signalConfig{
		bufferSize:  1,
		withHistory: 0,
	}
}

// WithBufferSize sets the channel buffer size for channel-based implementation.
// Has no effect on sync.Cond implementation.
// Default is 1.
func WithBufferSize(size int) SignalOption {
	return func(c *signalConfig) {
		if size > 0 {
			c.bufferSize = size
		}
	}
}

// WithHistory sets the base generation (total count) from which to catch up on broadcasts.
// - If set to 0 (default): catches up on all historical broadcasts
// - If set to -1: skips all historical broadcasts (starts fresh)
// - If set to N > 0: catches up on broadcasts since generation N
func WithHistory(baseGen int64) SignalOption {
	return func(c *signalConfig) {
		c.withHistory = baseGen
	}
}

// BroadcastOption configures a Broadcast when created.
type BroadcastOption func(*broadcastConfig)

type broadcastConfig struct {
	initialTotal int64
}

func defaultBroadcastConfig() *broadcastConfig {
	return &broadcastConfig{
		initialTotal: 0,
	}
}

// WithInitialTotal sets the initial total count for the broadcaster.
// This is useful for testing or when restoring state.
func WithInitialTotal(total int64) BroadcastOption {
	return func(c *broadcastConfig) {
		c.initialTotal = total
	}
}
