package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
	"github.com/faustbrian/go-circuit-breaker/breakertest"
)

func ExampleExecute() {
	circuit, err := breaker.New(breaker.Config{Name: "catalog"})
	if err != nil {
		return
	}
	result, err := breaker.Execute(context.Background(), circuit,
		func(context.Context) (string, error) { return "available", nil })
	fmt.Println(result, err)
	// Output: available <nil>
}

func ExampleBreaker_Acquire() {
	circuit, err := breaker.New(breaker.Config{Name: "stream"})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	if err := permit.Complete(breaker.OutcomeSuccess, false); err != nil {
		return
	}
	fmt.Println(circuit.Snapshot().Successes)
	// Output: 1
}

func ExampleConfig_failureRate() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "database",
		Window:            breaker.CountWindow{Size: 20},
		MinimumThroughput: 2,
		Opening:           &breaker.OpeningRules{FailureRatio: 0.5},
		OpenDuration:      breaker.FixedOpenDuration(time.Minute),
	})
	if err != nil {
		return
	}
	errUnavailable := errors.New("unavailable")
	for range 2 {
		_, err = breaker.Execute(context.Background(), circuit,
			func(context.Context) (struct{}, error) {
				return struct{}{}, errUnavailable
			})
		if !errors.Is(err, errUnavailable) {
			return
		}
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: open
}

func ExampleConfig_timeWindowAndSlowCalls() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "search",
		Window:            breaker.TimeWindow{BucketDuration: time.Second, BucketCount: 30},
		MinimumThroughput: 10,
		Opening:           &breaker.OpeningRules{SlowRatio: 0.8},
		SlowCallDuration:  500 * time.Millisecond,
	})
	if err != nil {
		return
	}
	snapshot := circuit.Snapshot()
	fmt.Println(snapshot.WindowCapacity, snapshot.MinimumThroughput)
	// Output: 30 10
}

func ExampleConfig_observer() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "payments",
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		Observer: func(event breaker.TransitionEvent) error {
			fmt.Println(event.Before.State, event.After.State, event.Reason)
			return nil
		},
		EventDelivery: breaker.SynchronousEvents{},
	})
	if err != nil {
		return
	}
	errDeclined := errors.New("declined upstream")
	_, err = breaker.Execute(context.Background(), circuit,
		func(context.Context) (struct{}, error) {
			return struct{}{}, errDeclined
		})
	if !errors.Is(err, errDeclined) {
		return
	}
	// Output: closed open policy-opened
}

func ExampleBreaker_ForceOpen() {
	circuit, err := breaker.New(breaker.Config{Name: "maintenance"})
	if err != nil {
		return
	}
	if err := circuit.ForceOpen(); err != nil {
		return
	}
	_, err = circuit.Acquire(context.Background())
	fmt.Println(errors.Is(err, breaker.ErrForceOpen))
	if err := circuit.Release(); err != nil {
		return
	}
	// Output: true
}

func ExampleBreaker_SetMode() {
	circuit, err := breaker.New(breaker.Config{Name: "maintenance"})
	if err != nil {
		return
	}
	if err := circuit.SetMode(breaker.ModeDisabled); err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		return
	}
	fmt.Println(circuit.Snapshot().Mode, circuit.Snapshot().Failures)

	if err := circuit.SetMode(breaker.ModeIsolated); err != nil {
		return
	}
	_, err = circuit.Acquire(context.Background())
	fmt.Println(errors.Is(err, breaker.ErrIsolated))

	if err := circuit.Reset(); err != nil {
		return
	}
	fmt.Println(circuit.Snapshot().State, circuit.Snapshot().Mode)
	// Output:
	// disabled 0
	// true
	// closed normal
}

func ExampleBreaker_Shutdown() {
	var observed atomic.Uint64
	circuit, err := breaker.New(breaker.Config{
		Name: "events",
		Observer: func(breaker.TransitionEvent) error {
			observed.Add(1)
			return nil
		},
	})
	if err != nil {
		return
	}
	if err := circuit.ForceOpen(); err != nil {
		return
	}
	if err := circuit.Shutdown(context.Background()); err != nil {
		return
	}
	fmt.Println(observed.Load())
	// Output: 1
}

func ExampleConfig_halfOpenRecovery() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         1,
			RequiredSuccesses: 1,
		},
	})
	if err != nil {
		return
	}
	errUnavailable := errors.New("unavailable")
	_, err = breaker.Execute(context.Background(), circuit,
		func(context.Context) (struct{}, error) {
			return struct{}{}, errUnavailable
		})
	if !errors.Is(err, errUnavailable) {
		return
	}
	clock.Advance(time.Second)
	_, err = breaker.Execute(context.Background(), circuit,
		func(context.Context) (struct{}, error) { return struct{}{}, nil })
	if err != nil {
		return
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: closed
}

func ExampleConfig_exponentialOpenDuration() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration: breaker.ExponentialOpenDuration{
			Initial:    time.Second,
			Multiplier: 2,
			Maximum:    time.Minute,
		},
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         1,
			RequiredSuccesses: 1,
		},
	})
	if err != nil {
		return
	}
	completeFailure := func() bool {
		permit, err := circuit.Acquire(context.Background())
		if err != nil {
			return false
		}
		return permit.Complete(breaker.OutcomeFailure, false) == nil
	}
	if !completeFailure() {
		return
	}
	fmt.Println(circuit.Snapshot().CurrentOpenDuration)
	clock.Advance(time.Second)
	if !completeFailure() {
		return
	}
	fmt.Println(circuit.Snapshot().CurrentOpenDuration)
	// Output:
	// 1s
	// 2s
}

func ExampleConfig_customClassifier() {
	errLocal := errors.New("local validation")
	circuit, err := breaker.New(breaker.Config{
		Name: "catalog",
		Classifier: func(completion breaker.Completion) breaker.Outcome {
			if errors.Is(completion.Err, errLocal) {
				return breaker.OutcomeIgnored
			}
			return breaker.OutcomeFailure
		},
	})
	if err != nil {
		return
	}
	_, err = breaker.Execute(context.Background(), circuit,
		func(context.Context) (struct{}, error) { return struct{}{}, errLocal })
	fmt.Println(errors.Is(err, errLocal), circuit.Snapshot().Ignored)
	// Output: true 1
}

func ExampleConfig_waitForProbe() {
	config := breaker.Config{
		Name:              "catalog",
		HalfOpenAdmission: breaker.WaitForProbe{MaxWait: 250 * time.Millisecond},
	}
	circuit, err := breaker.New(config)
	if err != nil {
		return
	}
	fmt.Printf("%T\n", config.HalfOpenAdmission)
	if err := circuit.Close(); err != nil {
		return
	}
	// Output: breaker.WaitForProbe
}

func ExampleConfig_combinedOpeningRules() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "search",
		MinimumThroughput: 2,
		Opening: &breaker.OpeningRules{
			FailureCount: 2,
			SlowCount:    2,
			Combination:  breaker.OpenWhenAll,
		},
	})
	if err != nil {
		return
	}
	for range 2 {
		permit, err := circuit.Acquire(context.Background())
		if err != nil {
			return
		}
		if err := permit.Complete(breaker.OutcomeFailure, true); err != nil {
			return
		}
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: open
}

func ExampleConfig_ignoredOutcomeBehavior() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		MinimumThroughput: 2,
		Opening: &breaker.OpeningRules{
			ConsecutiveFailures: 2,
			IgnoredBehavior:     breaker.ResetConsecutiveFailures,
		},
	})
	if err != nil {
		return
	}
	for _, outcome := range []breaker.Outcome{
		breaker.OutcomeFailure,
		breaker.OutcomeIgnored,
		breaker.OutcomeFailure,
	} {
		permit, err := circuit.Acquire(context.Background())
		if err != nil {
			return
		}
		if err := permit.Complete(outcome, false); err != nil {
			return
		}
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: closed
}

func ExampleConfig_successRatioRecovery() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:     3,
			SuccessRatio:  2.0 / 3.0,
			FailureAction: breaker.ReopenAfterSample,
		},
	})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		return
	}
	clock.Advance(time.Second)
	for _, outcome := range []breaker.Outcome{
		breaker.OutcomeSuccess,
		breaker.OutcomeFailure,
		breaker.OutcomeSuccess,
	} {
		permit, err = circuit.Acquire(context.Background())
		if err != nil {
			return
		}
		if err := permit.Complete(outcome, false); err != nil {
			return
		}
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: closed
}

type exampleRandom float64

func (r exampleRandom) Float64() float64 { return float64(r) }

func ExampleConfig_openDurationJitter() {
	circuit, err := breaker.New(breaker.Config{
		Name:               "catalog",
		MinimumThroughput:  1,
		Opening:            &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:       breaker.FixedOpenDuration(10 * time.Second),
		OpenDurationJitter: 0.5,
		Random:             exampleRandom(0.5),
	})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		return
	}
	fmt.Println(circuit.Snapshot().CurrentOpenDuration)
	// Output: 7.5s
}

func ExampleRejectExcessProbes() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		Clock:             clock,
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		OpenDuration:      breaker.FixedOpenDuration(time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         1,
			RequiredSuccesses: 1,
		},
		HalfOpenAdmission: breaker.RejectExcessProbes{},
	})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	if err := permit.Complete(breaker.OutcomeFailure, false); err != nil {
		return
	}
	clock.Advance(time.Second)
	probe, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	_, err = circuit.Acquire(context.Background())
	fmt.Println(errors.Is(err, breaker.ErrHalfOpenExhausted))
	if err := probe.Cancel(); err != nil {
		return
	}
	// Output: true
}

func ExamplePermit_Cancel() {
	circuit, err := breaker.New(breaker.Config{Name: "stream"})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	fmt.Println(permit.Cancel())
	fmt.Println(errors.Is(permit.Cancel(), breaker.ErrPermitCanceled))
	// Output:
	// <nil>
	// true
}

func ExampleRejectionError() {
	circuit, err := breaker.New(breaker.Config{Name: "catalog"})
	if err != nil {
		return
	}
	if err := circuit.ForceOpen(); err != nil {
		return
	}
	_, err = circuit.Acquire(context.Background())
	var rejection *breaker.RejectionError
	fmt.Println(errors.As(err, &rejection), rejection.Name, rejection.Mode)
	// Output: true catalog force-open
}

func ExampleConfig_permitTTL() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	circuit, err := breaker.New(breaker.Config{
		Name:      "stream",
		Clock:     clock,
		PermitTTL: time.Second,
	})
	if err != nil {
		return
	}
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	clock.Advance(time.Second)
	fmt.Println(errors.Is(permit.Complete(breaker.OutcomeSuccess, false), breaker.ErrPermitExpired))
	// Output: true
}
