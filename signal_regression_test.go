package solid_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ella.to/solid"
)

// TestNotifyAfterClose verifies that Notify on a closed broadcaster returns
// instead of blocking forever.
func TestNotifyAfterClose(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			b.Close()

			done := make(chan struct{})
			go func() {
				defer close(done)
				b.Notify(1)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Notify blocked after Close")
			}
		})
	}
}

// TestDoubleDone verifies that calling Done twice on the same signal does
// not panic.
func TestDoubleDone(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			defer b.Close()

			s := b.CreateSignal()
			s.Done()
			s.Done()
		})
	}
}

// TestDoubleClose verifies that calling Close twice on the broadcaster does
// not panic.
func TestDoubleClose(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			b.Close()
			b.Close()
		})
	}
}

// TestNotifyNonPositive verifies that Notify with zero or negative counts is
// a no-op and does not corrupt the pending count.
func TestNotifyNonPositive(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			defer b.Close()

			s := b.CreateSignal()
			defer s.Done()

			b.Notify(0)
			b.Notify(-5)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			if err := s.Wait(ctx); err == nil {
				t.Fatal("expected timeout, got a notification")
			}
		})
	}
}

// TestNotifyBatchDeliversAll verifies that a single Notify(n) is observed as
// n consumable notifications.
func TestNotifyBatchDeliversAll(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			defer b.Close()

			s := b.CreateSignal()
			defer s.Done()

			const n = 500
			b.Notify(n)

			for i := range n {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				err := s.Wait(ctx)
				cancel()
				if err != nil {
					t.Fatalf("wait %d failed: %v", i, err)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if err := s.Wait(ctx); err == nil {
				t.Fatal("expected timeout after consuming all notifications")
			}
		})
	}
}

// TestBlockedWaiterWakeup stresses the blocked-wait path: a waiter parks
// before each notification arrives, so a lost wakeup shows up as a hang.
func TestBlockedWaiterWakeup(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			defer b.Close()

			s := b.CreateSignal()
			defer s.Done()

			const rounds = 1000

			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				for range rounds {
					if err := s.Wait(ctx); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()

			for range rounds {
				b.Notify(1)
			}

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("waiter failed: %v", err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("waiter hung: lost wakeup")
			}
		})
	}
}

// TestBlockedWaiterClose verifies that a parked waiter is released with
// ErrSignalNotAvailable when the broadcaster closes.
func TestBlockedWaiterClose(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			s := b.CreateSignal()

			var wg sync.WaitGroup
			wg.Add(1)
			var waitErr error
			go func() {
				defer wg.Done()
				waitErr = s.Wait(context.Background())
			}()

			// Give the waiter a moment to park before closing.
			time.Sleep(50 * time.Millisecond)
			b.Close()

			doneCh := make(chan struct{})
			go func() {
				wg.Wait()
				close(doneCh)
			}()

			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
				t.Fatal("waiter not released by Close")
			}

			if !errors.Is(waitErr, solid.ErrSignalNotAvailable) {
				t.Fatalf("expected ErrSignalNotAvailable, got %v", waitErr)
			}
		})
	}
}

// TestWaitWithAlreadyCancelledContext verifies Wait returns the context
// error immediately when the context is already done.
func TestWaitWithAlreadyCancelledContext(t *testing.T) {
	t.Parallel()

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			t.Parallel()

			b := impl.new()
			defer b.Close()

			s := b.CreateSignal()
			defer s.Done()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			if err := s.Wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context.Canceled, got %v", err)
			}
		})
	}
}
