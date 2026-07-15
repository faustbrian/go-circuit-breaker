package breaker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func TestOpeningUnderContentionCreatesOneGenerationAndEvent(t *testing.T) {
	const callers = 128
	var openingEvents atomic.Uint64
	b := mustBreaker(t, breaker.Config{
		Name:              "inventory",
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		Observer: func(event breaker.TransitionEvent) error {
			if event.Reason == breaker.ReasonPolicyOpened {
				openingEvents.Add(1)
			}
			return nil
		},
		EventDelivery: breaker.SynchronousEvents{},
	})

	permits := make([]*breaker.Permit, callers)
	for index := range permits {
		permit, err := b.Acquire(context.Background())
		if err != nil {
			t.Fatalf("Acquire() %d error = %v", index, err)
		}
		permits[index] = permit
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(callers)
	for _, permit := range permits {
		permit := permit
		go func() {
			defer group.Done()
			<-start
			if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
				t.Errorf("Complete() error = %v", err)
			}
		}()
	}
	close(start)
	group.Wait()

	got := b.Snapshot()
	if got.State != breaker.StateOpen || got.Generation != 2 || got.TransitionCount != 1 {
		t.Fatalf("Snapshot() = %+v", got)
	}
	if openingEvents.Load() != 1 {
		t.Fatalf("opening events = %d, want 1", openingEvents.Load())
	}
	if got.Failures != 1 {
		t.Fatalf("Snapshot().Failures = %d, want only transition winner", got.Failures)
	}
}

func TestHalfOpenAdmissionBoundUnderContention(t *testing.T) {
	const (
		maxProbes = 7
		callers   = 256
	)
	clock := &fakeClock{now: time.Unix(100, 0)}
	b := openBreaker(t, clock, &breaker.HalfOpenPolicy{
		MaxProbes:         maxProbes,
		RequiredSuccesses: maxProbes,
	})
	clock.Advance(time.Minute)

	start := make(chan struct{})
	results := make(chan struct {
		permit *breaker.Permit
		err    error
	}, callers)
	for range callers {
		go func() {
			<-start
			permit, err := b.Acquire(context.Background())
			results <- struct {
				permit *breaker.Permit
				err    error
			}{permit: permit, err: err}
		}()
	}
	close(start)

	admitted := make([]*breaker.Permit, 0, maxProbes)
	rejected := 0
	for range callers {
		result := <-results
		if result.err == nil {
			admitted = append(admitted, result.permit)
			continue
		}
		if !errors.Is(result.err, breaker.ErrHalfOpenExhausted) {
			t.Fatalf("Acquire() error = %v", result.err)
		}
		rejected++
	}
	if len(admitted) != maxProbes || rejected != callers-maxProbes {
		t.Fatalf("admitted/rejected = %d/%d", len(admitted), rejected)
	}
	if got := b.Snapshot().ActiveHalfOpen; got != maxProbes {
		t.Fatalf("Snapshot().ActiveHalfOpen = %d, want %d", got, maxProbes)
	}
	for _, permit := range admitted {
		_ = permit.Cancel()
	}
}

func TestConcurrentDuplicateCompletionCountsExactlyOnce(t *testing.T) {
	const callers = 128
	b := mustBreaker(t, breaker.Config{Name: "inventory"})
	permit, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			results <- permit.Complete(breaker.OutcomeSuccess, false)
		}()
	}
	close(start)
	successes := 0
	duplicates := 0
	for range callers {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, breaker.ErrPermitCompleted):
			duplicates++
		default:
			t.Fatalf("Complete() error = %v", err)
		}
	}
	if successes != 1 || duplicates != callers-1 {
		t.Fatalf("successful/duplicate completions = %d/%d", successes, duplicates)
	}
	if got := b.Snapshot().Successes; got != 1 {
		t.Fatalf("Snapshot().Successes = %d, want 1", got)
	}
}

func TestConcurrentSnapshotResetModesAndCompletionsRemainConsistent(t *testing.T) {
	const iterations = 200
	b := mustBreaker(t, breaker.Config{Name: "inventory"})
	var group sync.WaitGroup
	group.Add(4)
	go func() {
		defer group.Done()
		for range iterations {
			permit, err := b.Acquire(context.Background())
			if err == nil {
				_ = permit.Complete(breaker.OutcomeSuccess, false)
			}
		}
	}()
	go func() {
		defer group.Done()
		for range iterations {
			_ = b.Reset()
		}
	}()
	go func() {
		defer group.Done()
		for range iterations {
			_ = b.ForceOpen()
			_ = b.Release()
			_ = b.Disable()
			_ = b.Release()
		}
	}()
	go func() {
		defer group.Done()
		for range iterations * 4 {
			snapshot := b.Snapshot()
			if snapshot.Generation == 0 || snapshot.ActiveHalfOpen < 0 {
				t.Errorf("inconsistent Snapshot() = %+v", snapshot)
			}
		}
	}()
	group.Wait()
}
