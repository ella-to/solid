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

func TestBasicUsage(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s1 := b.CreateSignal()
	defer s1.Done()

	b.Notify(1)

	err := s1.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlockedUsage(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s1 := b.CreateSignal()
	defer s1.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := s1.Wait(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	} else if errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMutlipleSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	n := 100
	var count atomic.Int64

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func(s solid.Signal) {
			defer wg.Done()
			defer s.Done()

			err := s.Wait(context.Background())
			if err != nil {
				t.Error(err)
			}
			count.Add(1)
		}(b.CreateSignal())
	}

	b.Notify(1)

	wg.Wait()

	if count.Load() != int64(n) {
		t.Errorf("expected %d signals, got %d", n, count.Load())
	}
}

func TestCountSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s := b.CreateSignal()
	defer s.Done()

	for range 1000 {
		b.Notify(1)
	}

	for range 1000 {
		err := s.Wait(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.Wait(ctx); errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error but got %v", err)
	}
}

func TestWithHistoryFromBeginning(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	n := 100

	for range n {
		b.Notify(1)
	}

	s := b.CreateSignal(solid.WithHistory(0))
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

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.Wait(ctx); errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error but got %v", err)
	}
}

func TestWithHistoryFromLatest(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	n := 100

	for range n {
		b.Notify(1)
	}

	s1 := b.CreateSignal(solid.WithHistory(-1))
	defer s1.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s1.Wait(ctx); errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error but got %v", err)
	}

	s2 := b.CreateSignal()
	defer s2.Done()

	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s2.Wait(ctx); errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error but got %v", err)
	}
}

func TestWithHistoryFromN(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	n := 100
	m := 20

	for range n {
		b.Notify(1)
	}

	s := b.CreateSignal(solid.WithHistory(int64(m)))
	defer s.Done()

	for range n - m {
		err := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			return s.Wait(ctx)
		}()
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.Wait(ctx); errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error but got %v", err)
	}
}

func TestErrSignalNotAvailable(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()

	s := b.CreateSignal()

	b.Close()

	err := s.Wait(context.Background())
	if !errors.Is(err, solid.ErrSignalNotAvailable) {
		t.Fatalf("expected ErrSignalNotAvailable but got %v", err)
	}
}

func TestNilSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	b.Close()

	s := b.CreateSignal()

	if s != nil {
		t.Fatalf("expected nil signal but got %v", s)
	}
}

func Benchmark1Singal(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcast()
	defer bc.Close()

	s := bc.CreateSignal()
	defer s.Done()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Notify(1)
		_ = s.Wait(b.Context())
	}
}

func BenchmarkBroadcast100Signals(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcast()
	defer bc.Close()

	for i := 0; i < 100; i++ {
		bc.CreateSignal()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Notify(1)
	}
}
