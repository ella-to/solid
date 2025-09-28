package solid_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ella.to/solid"
)

// TestConcurrentSignalCreation tests creating signals concurrently
func TestConcurrentSignalCreation(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	const numSignals = 100
	signals := make([]*solid.Signal, numSignals)
	var wg sync.WaitGroup

	// Create signals concurrently
	wg.Add(numSignals)
	for i := 0; i < numSignals; i++ {
		go func(index int) {
			defer wg.Done()
			signals[index] = b.CreateSignal()
		}(i)
	}
	wg.Wait()

	// Verify all signals were created
	for i, s := range signals {
		if s == nil {
			t.Errorf("Signal %d was not created", i)
		} else {
			s.Done()
		}
	}
}

// TestSignalDoneAfterBroadcastClose tests calling Done() after broadcast is closed
func TestSignalDoneAfterBroadcastClose(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	s := b.CreateSignal()

	b.Close()

	// This should not panic
	s.Done()
}

// TestMultipleWaitCalls tests multiple Wait() calls on the same signal
func TestMultipleWaitCalls(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s := b.CreateSignal()
	defer s.Done()

	// Send one notification
	b.Notify()

	// First wait should succeed immediately
	err1 := s.Wait(context.Background())
	if err1 != nil {
		t.Fatalf("First wait failed: %v", err1)
	}

	// Second wait should timeout since no more notifications
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err2 := s.Wait(ctx)
	if err2 == nil {
		t.Fatal("Second wait should have timed out")
	}
}

// TestWithBufferSize tests the WithBufferSize option
func TestWithBufferSize(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s := b.CreateSignal(solid.WithBufferSize(5))
	defer s.Done()

	// Send multiple notifications (more than default buffer)
	for i := 0; i < 10; i++ {
		b.Notify()
	}

	// Should be able to consume all notifications
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := s.Wait(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Wait %d failed: %v", i, err)
		}
	}
}

// TestHistoryWithInitialTotal tests WithHistory combined with WithInitialTotal
func TestHistoryWithInitialTotal(t *testing.T) {
	t.Parallel()

	initialTotal := int64(50)
	b := solid.NewBroadcast(solid.WithInitialTotal(initialTotal))
	defer b.Close()

	// Add more notifications
	for i := 0; i < 25; i++ {
		b.Notify()
	}

	// Create signal with history from the beginning
	s := b.CreateSignal(solid.WithHistory(0))
	defer s.Done()

	// Should be able to get all 75 notifications (50 initial + 25 added)
	expectedCount := int(initialTotal + 25)
	for i := 0; i < expectedCount; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := s.Wait(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Wait %d failed: %v", i, err)
		}
	}

	// Next wait should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := s.Wait(ctx)
	if err == nil {
		t.Fatal("Expected timeout but got success")
	}
}

// TestConcurrentNotifyAndWait tests concurrent notifications and waits (simplified)
func TestConcurrentNotifyAndWait(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	const numWorkers = 1
	const numNotifications = 5

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			s := b.CreateSignal()
			defer s.Done()

			for j := 0; j < numNotifications; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := s.Wait(ctx)
				fmt.Println("Worker", workerID, "received notification", j)
				cancel()
				if err != nil {
					t.Errorf("Worker %d failed on notification %d: %v", workerID, j, err)
					return
				}
			}
		}(i)
	}

	// Send notifications
	go func() {
		fmt.Println("Starting notifications")
		defer fmt.Println("Finished notifications")
		for i := 0; i < numNotifications; i++ {
			b.Notify()
			time.Sleep(50 * time.Millisecond) // More generous timing
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out")
	}
}

// TestHistoryEdgeCase tests edge case where history count equals total
func TestHistoryEdgeCase(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	// Send exactly 10 notifications
	for i := 0; i < 10; i++ {
		b.Notify()
	}

	// Create signal with history from exactly when all notifications happened
	s := b.CreateSignal(solid.WithHistory(10))
	defer s.Done()

	// Should timeout since no new notifications are available
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := s.Wait(ctx)
	if err == nil {
		t.Fatal("Expected timeout but got success")
	}
}
