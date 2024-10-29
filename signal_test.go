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

	s1 := b.CreateSignal(1)
	defer s1.Done()

	b.Broadcast()

	err := s1.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}

}

func TestBlockedUsage(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s1 := b.CreateSignal(1)
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

	for i := 0; i < n; i++ {
		go func(s *solid.Signal) {
			defer wg.Done()
			defer s.Done()

			err := s.Wait(context.Background())
			if err != nil {
				t.Error(err)
			}
			count.Add(1)

		}(b.CreateSignal(1))
	}

	b.Broadcast()

	wg.Wait()

	if count.Load() != int64(n) {
		t.Errorf("expected %d signals, got %d", n, count.Load())
	}
}

func TestCountSignal(t *testing.T) {
	t.Parallel()

	b := solid.NewBroadcast()
	defer b.Close()

	s := b.CreateSignal(0)
	defer s.Done()

	for range 1000 {
		b.Broadcast()
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

func Benchmark1Singal(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcast()
	defer bc.Close()

	s := bc.CreateSignal(1)
	defer s.Done()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Broadcast()
		s.Wait(context.Background())
	}
}

func BenchmarkBroadcast100Signals(b *testing.B) {
	b.ReportAllocs()

	bc := solid.NewBroadcast()
	defer bc.Close()

	for i := 0; i < 100; i++ {
		bc.CreateSignal(1)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bc.Broadcast()
	}
}
