package breaker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func TestSynchronousObserverReceivesImmutableTransitionsOutsideLock(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0)}
	var b *breaker.Breaker
	var mu sync.Mutex
	var events []breaker.TransitionEvent
	observer := func(event breaker.TransitionEvent) error {
		_ = b.Snapshot()
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	}
	b = mustBreaker(t, breaker.Config{
		Name:              "inventory",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         1,
			RequiredSuccesses: 1,
		},
		Observer:      observer,
		EventDelivery: breaker.SynchronousEvents{},
	})

	complete := make(chan error, 1)
	go func() {
		permit, err := b.Acquire(context.Background())
		if err != nil {
			complete <- err
			return
		}
		complete <- permit.Complete(breaker.OutcomeFailure, false)
	}()
	select {
	case err := <-complete:
		if err != nil {
			t.Fatalf("Complete() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("observer deadlocked while reading Snapshot")
	}

	clock.Advance(time.Second)
	probe, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("probe Acquire() error = %v", err)
	}
	if err := probe.Complete(breaker.OutcomeSuccess, false); err != nil {
		t.Fatalf("probe Complete() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 {
		t.Fatalf("observer event count = %d, want 3", len(events))
	}
	wantReasons := []breaker.TransitionReason{
		breaker.ReasonPolicyOpened,
		breaker.ReasonOpenIntervalElapsed,
		breaker.ReasonHalfOpenRecovered,
	}
	for index, event := range events {
		if event.Reason != wantReasons[index] {
			t.Fatalf("event[%d].Reason = %v, want %v", index, event.Reason, wantReasons[index])
		}
		if event.After.Generation != event.Before.Generation+1 {
			t.Fatalf("event[%d] generations = %d -> %d", index, event.Before.Generation, event.After.Generation)
		}
		if event.Generation != event.After.Generation || !event.Timestamp.Equal(clock.Now()) && index == 2 {
			t.Fatalf("event[%d] metadata = %+v", index, event)
		}
	}
	if events[0].Before.State != breaker.StateClosed || events[0].After.State != breaker.StateOpen {
		t.Fatalf("opening event = %+v", events[0])
	}
	if events[0].After.Failures != 1 {
		t.Fatalf("opening event failures = %d, want 1", events[0].After.Failures)
	}
}

func TestObserverPanicAndFailureDoNotCorruptBreaker(t *testing.T) {
	t.Parallel()

	for name, observer := range map[string]breaker.Observer{
		"panic": func(breaker.TransitionEvent) error { panic("observer panic") },
		"error": func(breaker.TransitionEvent) error { return errors.New("observer error") },
	} {
		observer := observer
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := mustBreaker(t, breaker.Config{
				Name:              "inventory",
				MinimumThroughput: 1,
				Opening:           &breaker.OpeningRules{FailureCount: 1},
				Observer:          observer,
				EventDelivery:     breaker.SynchronousEvents{},
			})
			permit, err := b.Acquire(context.Background())
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			got := b.Snapshot()
			if got.State != breaker.StateOpen || got.ObserverFailures != 1 {
				t.Fatalf("Snapshot() = %+v", got)
			}
		})
	}
}

func TestAsynchronousObserverIsBoundedAndDoesNotBlockAdmission(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	b := mustBreaker(t, breaker.Config{
		Name:              "inventory",
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		Observer: func(breaker.TransitionEvent) error {
			once.Do(func() { close(started) })
			<-release
			return nil
		},
		EventDelivery: breaker.AsynchronousEvents{
			Buffer:   1,
			Overflow: breaker.DropNewestEvent,
		},
	})

	permit, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("asynchronous observer did not start")
	}

	if err := b.ForceOpen(); err != nil {
		t.Fatalf("ForceOpen() error = %v", err)
	}
	if err := b.Isolate(); err != nil {
		t.Fatalf("Isolate() error = %v", err)
	}
	if _, err := b.Acquire(context.Background()); !errors.Is(err, breaker.ErrIsolated) {
		t.Fatalf("Acquire() error = %v, want ErrIsolated", err)
	}
	if got := b.Snapshot().DroppedEvents; got == 0 {
		t.Fatal("Snapshot().DroppedEvents = 0, want bounded overflow")
	}

	close(release)
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestNewValidatesEventDelivery(t *testing.T) {
	t.Parallel()

	tests := []breaker.Config{
		{
			Name:     "inventory",
			Observer: func(breaker.TransitionEvent) error { return nil },
			EventDelivery: breaker.AsynchronousEvents{
				Buffer: 0,
			},
		},
		{
			Name:     "inventory",
			Observer: func(breaker.TransitionEvent) error { return nil },
			EventDelivery: breaker.AsynchronousEvents{
				Buffer:   1,
				Overflow: breaker.EventOverflowPolicy(99),
			},
		},
	}

	for _, config := range tests {
		if _, err := breaker.New(config); !errors.Is(err, breaker.ErrInvalidConfig) {
			t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
		}
	}
}
