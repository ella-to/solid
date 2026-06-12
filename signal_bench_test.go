package solid_test

import (
	"context"
	"fmt"
	"testing"

	"ella.to/solid"
)

var implementations = []struct {
	name string
	new  func(opts ...solid.BroadcastOption) solid.Broadcast
}{
	{"Chan", solid.NewBroadcast},
	{"Cond", solid.NewBroadcastCond},
}

// BenchmarkNotifyWait measures the notify -> wait round trip with a single
// subscriber and a non-cancellable context (the Wait fast path).
func BenchmarkNotifyWait(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()

			s := bc.CreateSignal()
			defer s.Done()

			ctx := context.Background()
			b.ResetTimer()

			for b.Loop() {
				bc.Notify(1)
				if err := s.Wait(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNotifyWaitCancellableCtx is the same round trip but with a
// cancellable context, which exercises the cancellation plumbing.
func BenchmarkNotifyWaitCancellableCtx(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()

			s := bc.CreateSignal()
			defer s.Done()

			ctx := b.Context()
			b.ResetTimer()

			for b.Loop() {
				bc.Notify(1)
				if err := s.Wait(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWaitPending measures consuming already-delivered notifications
// (pure Wait fast path, no blocking).
func BenchmarkWaitPending(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()

			s := bc.CreateSignal()
			defer s.Done()

			bc.Notify(int64(b.N))

			ctx := context.Background()
			b.ResetTimer()

			// b.N-style loop: the pending count above must match the exact
			// iteration count, which b.Loop does not expose upfront.
			for i := 0; i < b.N; i++ {
				if err := s.Wait(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBroadcast measures Notify fan-out cost as subscriber count grows.
func BenchmarkBroadcast(b *testing.B) {
	for _, impl := range implementations {
		for _, numSignals := range []int{1, 10, 100, 1000} {
			b.Run(fmt.Sprintf("%s/%dSignals", impl.name, numSignals), func(b *testing.B) {
				b.ReportAllocs()

				bc := impl.new()
				defer bc.Close()

				for range numSignals {
					s := bc.CreateSignal()
					defer s.Done()
				}

				b.ResetTimer()

				for b.Loop() {
					bc.Notify(1)
				}
			})
		}
	}
}

// BenchmarkNotifyBatch measures delivering N notifications in a single call.
func BenchmarkNotifyBatch(b *testing.B) {
	for _, impl := range implementations {
		for _, batch := range []int64{1, 100, 10000} {
			b.Run(fmt.Sprintf("%s/Notify%d", impl.name, batch), func(b *testing.B) {
				b.ReportAllocs()

				bc := impl.new()
				defer bc.Close()

				s := bc.CreateSignal()
				defer s.Done()

				b.ResetTimer()

				for b.Loop() {
					bc.Notify(batch)
				}
			})
		}
	}
}

// BenchmarkCreateSignalDone measures subscriber churn.
func BenchmarkCreateSignalDone(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()

			b.ResetTimer()

			for b.Loop() {
				s := bc.CreateSignal()
				s.Done()
			}
		})
	}
}

// BenchmarkNotifyParallel measures contended Notify from many goroutines
// with a moderate number of subscribers.
func BenchmarkNotifyParallel(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()

			for range 10 {
				s := bc.CreateSignal()
				defer s.Done()
			}

			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					bc.Notify(1)
				}
			})
		})
	}
}

// BenchmarkBlockedWakeup measures waking a waiter that is actually parked,
// not just consuming a pre-delivered count: the waiter acks each wakeup on a
// reverse broadcaster, so every iteration is a real block/unblock cycle.
func BenchmarkBlockedWakeup(b *testing.B) {
	for _, impl := range implementations {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()

			bc := impl.new()
			defer bc.Close()
			ack := impl.new()
			defer ack.Close()

			s := bc.CreateSignal()
			ackSig := ack.CreateSignal()
			defer ackSig.Done()

			ctx := context.Background()
			go func() {
				defer s.Done()
				for s.Wait(ctx) == nil {
					ack.Notify(1)
				}
			}()

			b.ResetTimer()

			for b.Loop() {
				bc.Notify(1)
				if err := ackSig.Wait(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
