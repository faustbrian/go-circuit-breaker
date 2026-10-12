package breaker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
	"github.com/faustbrian/go-circuit-breaker/breakertest"
)

// BenchmarkClosedExecute times default classified success/result return with
// system-clock sampling; ResetTimer excludes constructor/context setup.
func BenchmarkClosedExecute(b *testing.B) {
	circuit := mustBenchmarkBreaker(b, breaker.Config{Name: "benchmark"})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = breaker.Execute(ctx, circuit, func(context.Context) (int, error) {
			return 42, nil
		})
	}
}

// BenchmarkOpenRejection times repeated rejection during a one-hour open
// interval; ResetTimer excludes constructor and opening completion.
func BenchmarkOpenRejection(b *testing.B) {
	circuit := mustBenchmarkBreaker(b, breaker.Config{
		Name:              "benchmark",
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Hour),
	})
	permit, _ := circuit.Acquire(context.Background())
	_ = permit.Complete(breaker.OutcomeFailure, false)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = circuit.Acquire(ctx)
	}
}

// BenchmarkSnapshot times parallel snapshots of an empty closed breaker;
// ResetTimer excludes construction, not mutex/clock contention.
func BenchmarkSnapshot(b *testing.B) {
	circuit := mustBenchmarkBreaker(b, breaker.Config{Name: "benchmark"})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = circuit.Snapshot()
		}
	})
}

// BenchmarkHalfOpenContention times parallel quota rejection with one held
// synthetic-clock probe. ResetTimer excludes opening/advance/admission; it does
// not measure recovery throughput.
func BenchmarkHalfOpenContention(b *testing.B) {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit := mustBenchmarkBreaker(b, breaker.Config{
		Name:              "benchmark",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen:          &breaker.HalfOpenPolicy{MaxProbes: 1, RequiredSuccesses: 1},
	})
	permit, _ := circuit.Acquire(context.Background())
	_ = permit.Complete(breaker.OutcomeFailure, false)
	clock.Advance(time.Second)
	active, _ := circuit.Acquire(context.Background())
	defer func() { _ = active.Cancel() }()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = circuit.Acquire(ctx)
		}
	})
}

// BenchmarkSynchronousTransitionObserver times two administrative transitions
// and callbacks per iteration; ResetTimer excludes construction. Compare with
// async only after accounting for the different operation count.
func BenchmarkSynchronousTransitionObserver(b *testing.B) {
	circuit := mustBenchmarkBreaker(b, breaker.Config{
		Name:          "benchmark",
		Observer:      func(breaker.TransitionEvent) error { return nil },
		EventDelivery: breaker.SynchronousEvents{},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = circuit.ForceOpen()
		_ = circuit.Release()
	}
}

// BenchmarkAsynchronousTransitionObserver times one alternating administrative
// transition/enqueue per iteration with a MaxEventBuffer queue. ResetTimer
// excludes construction and StopTimer excludes drain; drops are permitted, so
// this is not lossless callback throughput.
func BenchmarkAsynchronousTransitionObserver(b *testing.B) {
	var observed atomic.Uint64
	circuit := mustBenchmarkBreaker(b, breaker.Config{
		Name: "benchmark",
		Observer: func(breaker.TransitionEvent) error {
			observed.Add(1)
			return nil
		},
		EventDelivery: breaker.AsynchronousEvents{
			Buffer:   breaker.MaxEventBuffer,
			Overflow: breaker.DropNewestEvent,
		},
	})
	b.Cleanup(func() {
		if err := circuit.Shutdown(context.Background()); err != nil {
			b.Errorf("Shutdown() error = %v", err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if index%2 == 0 {
			_ = circuit.ForceOpen()
		} else {
			_ = circuit.Release()
		}
	}
	b.StopTimer()
	_ = observed.Load()
}

func mustBenchmarkBreaker(b *testing.B, config breaker.Config) *breaker.Breaker {
	b.Helper()
	circuit, err := breaker.New(config)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	b.Cleanup(func() { _ = circuit.Shutdown(context.Background()) })
	return circuit
}
