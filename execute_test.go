package breaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func TestExecutePreservesTypedResultAndOriginalError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("dependency unavailable")
	b := mustBreaker(t, breaker.Config{Name: "inventory"})
	result, err := breaker.Execute(context.Background(), b, func(context.Context) (int, error) {
		return 42, wantErr
	})

	if result != 42 {
		t.Fatalf("Execute() result = %d, want 42", result)
	}
	if err != wantErr {
		t.Fatalf("Execute() error = %v, want original error", err)
	}
	if got := b.Snapshot(); got.Failures != 1 || got.Successes != 0 {
		t.Fatalf("Snapshot() = %+v", got)
	}
}

func TestExecuteDoesNotInvokeRejectedOperation(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Unix(100, 0)}
	b := openBreaker(t, clock, &breaker.HalfOpenPolicy{
		MaxProbes:         1,
		RequiredSuccesses: 1,
	})
	invoked := false

	_, err := breaker.Execute(context.Background(), b, func(context.Context) (string, error) {
		invoked = true
		return "unexpected", nil
	})

	if !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("Execute() error = %v, want ErrOpen", err)
	}
	if invoked {
		t.Fatal("Execute() invoked rejected operation")
	}
}

func TestExecuteHonorsClassifierAndSlowThreshold(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Unix(100, 0)}
	b := mustBreaker(t, breaker.Config{
		Name:             "inventory",
		Clock:            clock,
		SlowCallDuration: 100 * time.Millisecond,
		Classifier: func(completion breaker.Completion) breaker.Outcome {
			if completion.Result == "cached" {
				return breaker.OutcomeIgnored
			}
			return breaker.OutcomeSuccess
		},
	})

	_, err := breaker.Execute(context.Background(), b, func(context.Context) (string, error) {
		clock.Advance(time.Second)
		return "cached", errors.New("local cache marker")
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want original operation error")
	}
	if got := b.Snapshot(); got.Ignored != 1 || got.WindowClassified != 0 || got.SlowFailures != 0 {
		t.Fatalf("Snapshot() = %+v", got)
	}
}

func TestExecuteCancellationBeforeAdmissionDoesNotConsumePermit(t *testing.T) {
	t.Parallel()

	b := mustBreaker(t, breaker.Config{Name: "inventory"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	invoked := false

	_, err := breaker.Execute(ctx, b, func(context.Context) (int, error) {
		invoked = true
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if invoked {
		t.Fatal("Execute() invoked operation after pre-admission cancellation")
	}
	if got := b.Snapshot(); got.Admitted != 0 || got.WindowClassified != 0 {
		t.Fatalf("Snapshot() = %+v", got)
	}
}

func TestExecuteRecordsOperationPanicAndRepanicsOriginalValue(t *testing.T) {
	t.Parallel()

	b := mustBreaker(t, breaker.Config{Name: "inventory"})
	panicValue := &struct{ message string }{message: "boom"}

	recovered := capturePanic(func() {
		_, _ = breaker.Execute(context.Background(), b, func(context.Context) (int, error) {
			panic(panicValue)
		})
	})
	if recovered != panicValue {
		t.Fatalf("recovered panic = %#v, want original value", recovered)
	}
	if got := b.Snapshot(); got.Failures != 1 {
		t.Fatalf("Snapshot().Failures = %d, want 1", got.Failures)
	}
}

func TestExecuteRecordsClassifierPanicAndRepanics(t *testing.T) {
	t.Parallel()

	b := mustBreaker(t, breaker.Config{
		Name: "inventory",
		Classifier: func(breaker.Completion) breaker.Outcome {
			panic("classifier panic")
		},
	})

	recovered := capturePanic(func() {
		_, _ = breaker.Execute(context.Background(), b, func(context.Context) (int, error) {
			return 1, nil
		})
	})
	if recovered != "classifier panic" {
		t.Fatalf("recovered panic = %#v", recovered)
	}
	if got := b.Snapshot(); got.Failures != 1 {
		t.Fatalf("Snapshot().Failures = %d, want 1", got.Failures)
	}
}

func capturePanic(operation func()) (recovered any) {
	defer func() { recovered = recover() }()
	operation()
	return nil
}
