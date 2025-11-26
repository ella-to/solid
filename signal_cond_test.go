package solid_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ella.to/solid"
)

func TestCondBasicUsage(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	s1 := b.CreateSignal()
	defer s1.Done()

	b.Notify()

	err := s1.Wait(t.Context())
	if err != nil {
		t.Fatal(err)
	}
}

func TestCondBlockedUsage(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	s1 := b.CreateSignal()
	defer s1.Done()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	err := s1.Wait(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCondMultipleSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	n := 100
	var count atomic.Int64

	var wg sync.WaitGroup

	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(s *solid.SignalCond) {
			defer wg.Done()
			defer s.Done()

			err := s.Wait(context.Background())
			if err != nil {
				t.Error(err)
			}
			count.Add(1)
		}(b.CreateSignal())
	}

	b.Notify()

	wg.Wait()

	if count.Load() != int64(n) {
		t.Errorf("expected %d signals, got %d", n, count.Load())
	}
}

func TestCondCountSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	s := b.CreateSignal()
	defer s.Done()

	for range 1000 {
		b.Notify()
	}

	for range 1000 {
		err := s.Wait(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := s.Wait(ctx); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestCondWithHistoryFromBeginning(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	n := 100

	for range n {
		b.Notify()
	}

	s := b.CreateSignal(solid.WithCondHistory(0))
	defer s.Done()

	for range n {
		err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			return s.Wait(ctx)
		}()
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := s.Wait(ctx); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestCondWithHistoryFromLatest(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	defer b.Close()

	n := 100

	for range n {
		b.Notify()
	}

	// WithCondHistory(-1) means skip all historical notifications
	s1 := b.CreateSignal(solid.WithCondHistory(-1))
	defer s1.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should timeout because we're skipping history
	if err := s1.Wait(ctx); err == nil {
		t.Fatalf("expected timeout error")
	}

	// Default behavior (withHistory: 0) catches up on all historical notifications
	s2 := b.CreateSignal()
	defer s2.Done()

	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should return immediately because default catches up on all 100 notifications
	if err := s2.Wait(ctx); err != nil {
		t.Fatalf("expected nil error but got %v", err)
	}
}

func TestCondErrSignalNotAvailable(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()

	s := b.CreateSignal()

	b.Close()

	err := s.Wait(context.Background())
	if !errors.Is(err, solid.ErrSignalNotAvailable) {
		t.Fatalf("expected ErrSignalNotAvailable but got %v", err)
	}
}

func TestCondNilSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcastCond()
	b.Close()

	s := b.CreateSignal()

	if s != nil {
		t.Fatalf("expected nil signal but got %v", s)
	}
}

func BenchmarkCond1Signal(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcastCond()
	defer bc.Close()

	s := bc.CreateSignal()
	defer s.Done()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Notify()
		s.Wait(context.Background())
	}
}

func BenchmarkCondBroadcast100Signals(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcastCond()
	defer bc.Close()

	for i := 0; i < 100; i++ {
		bc.CreateSignal()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Notify()
	}
}
