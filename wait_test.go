package breaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func TestHalfOpenWaiterAcquiresReleasedCapacity(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0)}
	b := waitingOpenBreaker(t, clock, time.Second)
	clock.Advance(time.Second)

	active, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}

	result := make(chan struct {
		permit *breaker.Permit
		err    error
	}, 1)
	go func() {
		permit, acquireErr := b.Acquire(context.Background())
		result <- struct {
			permit *breaker.Permit
			err    error
		}{permit: permit, err: acquireErr}
	}()

	select {
	case got := <-result:
		t.Fatalf("waiting Acquire() returned early: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	if err := active.Cancel(); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("waiting Acquire() error = %v", got.err)
		}
		if got.permit == nil {
			t.Fatal("waiting Acquire() permit = nil")
		}
		_ = got.permit.Cancel()
	case <-time.After(time.Second):
		t.Fatal("waiting Acquire() did not receive released capacity")
	}
}

func TestHalfOpenWaitHonorsCallerCancellation(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0)}
	b := waitingOpenBreaker(t, clock, time.Second)
	clock.Advance(time.Second)
	active, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer func() { _ = active.Cancel() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
	if got := b.Snapshot().ActiveHalfOpen; got != 1 {
		t.Fatalf("Snapshot().ActiveHalfOpen = %d, want 1", got)
	}
}

func TestHalfOpenWaitHasFiniteMaximum(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0)}
	b := waitingOpenBreaker(t, clock, time.Minute)
	clock.Advance(time.Second)
	active, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer func() { _ = active.Cancel() }()

	result := make(chan error, 1)
	go func() {
		_, waitErr := b.Acquire(context.Background())
		result <- waitErr
	}()
	select {
	case err := <-result:
		t.Fatalf("waiting Acquire() returned before fake-clock advance: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	clock.Advance(time.Minute)
	select {
	case err := <-result:
		if !errors.Is(err, breaker.ErrHalfOpenWaitTimeout) {
			t.Fatalf("waiting Acquire() error = %v, want ErrHalfOpenWaitTimeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting Acquire() ignored fake-clock timer")
	}
}

func TestNewRejectsInvalidHalfOpenWait(t *testing.T) {
	t.Parallel()

	_, err := breaker.New(breaker.Config{
		Name:              "inventory",
		HalfOpenAdmission: breaker.WaitForProbe{MaxWait: 0},
	})
	if !errors.Is(err, breaker.ErrInvalidConfig) {
		t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
	}
}

func waitingOpenBreaker(t *testing.T, clock *fakeClock, maximumWait time.Duration) *breaker.Breaker {
	t.Helper()
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
		HalfOpenAdmission: breaker.WaitForProbe{MaxWait: maximumWait},
	})
	permit, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("initial Acquire() error = %v", err)
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		t.Fatalf("initial Complete() error = %v", err)
	}
	return b
}
