package breaker_test

import (
	"context"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
	"github.com/faustbrian/go-circuit-breaker/breakertest"
)

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

func mustBenchmarkBreaker(b *testing.B, config breaker.Config) *breaker.Breaker {
	b.Helper()
	circuit, err := breaker.New(config)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	b.Cleanup(func() { _ = circuit.Close() })
	return circuit
}
