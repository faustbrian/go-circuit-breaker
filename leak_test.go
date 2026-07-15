package breaker_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
	"github.com/faustbrian/go-circuit-breaker/breakertest"
)

func TestCanceledHalfOpenWaitReleasesClockTimer(t *testing.T) {
	clock := breakertest.NewClock(time.Unix(100, 0))
	b := mustBreaker(t, breaker.Config{
		Name:              "inventory",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         1,
			RequiredSuccesses: 1,
		},
		HalfOpenAdmission: breaker.WaitForProbe{MaxWait: time.Hour},
	})
	completeOutcome(t, b, breaker.OutcomeFailure)
	clock.Advance(time.Second)
	active, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer func() { _ = active.Cancel() }()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, waitErr := b.Acquire(ctx)
		result <- waitErr
	}()
	eventually(t, func() bool { return clock.ActiveTimers() == 1 })
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
	eventually(t, func() bool { return clock.ActiveTimers() == 0 })
}

func TestCloseDrainsObserverQueueAndDropsLaterEvents(t *testing.T) {
	var observed atomic.Uint64
	b := mustBreaker(t, breaker.Config{
		Name: "inventory",
		Observer: func(breaker.TransitionEvent) error {
			observed.Add(1)
			return nil
		},
		EventDelivery: breaker.AsynchronousEvents{
			Buffer:   16,
			Overflow: breaker.DropNewestEvent,
		},
	})
	for range 5 {
		_ = b.ForceOpen()
		_ = b.Release()
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := observed.Load(); got != 10 {
		t.Fatalf("observed events after Close() = %d, want 10", got)
	}
	if err := b.ForceOpen(); err != nil {
		t.Fatalf("ForceOpen() after Close() error = %v", err)
	}
	if got := b.Snapshot().DroppedEvents; got != 1 {
		t.Fatalf("Snapshot().DroppedEvents = %d, want 1", got)
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not satisfied before deadline")
		}
		runtime.Gosched()
	}
}
