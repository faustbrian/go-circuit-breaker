package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func ExampleExecute() {
	circuit, _ := breaker.New(breaker.Config{Name: "catalog"})
	result, err := breaker.Execute(context.Background(), circuit,
		func(context.Context) (string, error) { return "available", nil })
	fmt.Println(result, err)
	// Output: available <nil>
}

func ExampleBreaker_Acquire() {
	circuit, _ := breaker.New(breaker.Config{Name: "stream"})
	permit, err := circuit.Acquire(context.Background())
	if err != nil {
		return
	}
	defer func() { _ = permit.Cancel() }()

	_ = permit.Complete(breaker.OutcomeSuccess, false)
	fmt.Println(circuit.Snapshot().Successes)
	// Output: 1
}

func ExampleConfig_failureRate() {
	circuit, _ := breaker.New(breaker.Config{
		Name:              "database",
		Window:            breaker.CountWindow{Size: 20},
		MinimumThroughput: 2,
		Opening:           &breaker.OpeningRules{FailureRatio: 0.5},
		OpenDuration:      breaker.FixedOpenDuration(time.Minute),
	})
	for range 2 {
		_, _ = breaker.Execute(context.Background(), circuit,
			func(context.Context) (struct{}, error) {
				return struct{}{}, errors.New("unavailable")
			})
	}
	fmt.Println(circuit.Snapshot().State)
	// Output: open
}

func ExampleConfig_timeWindowAndSlowCalls() {
	circuit, _ := breaker.New(breaker.Config{
		Name:              "search",
		Window:            breaker.TimeWindow{BucketDuration: time.Second, BucketCount: 30},
		MinimumThroughput: 10,
		Opening:           &breaker.OpeningRules{SlowRatio: 0.8},
		SlowCallDuration:  500 * time.Millisecond,
	})
	snapshot := circuit.Snapshot()
	fmt.Println(snapshot.WindowCapacity, snapshot.MinimumThroughput)
	// Output: 30 10
}

func ExampleConfig_observer() {
	circuit, _ := breaker.New(breaker.Config{
		Name:              "payments",
		MinimumThroughput: 1,
		Opening:           &breaker.OpeningRules{FailureCount: 1},
		Observer: func(event breaker.TransitionEvent) error {
			fmt.Println(event.Before.State, event.After.State, event.Reason)
			return nil
		},
		EventDelivery: breaker.SynchronousEvents{},
	})
	_, _ = breaker.Execute(context.Background(), circuit,
		func(context.Context) (struct{}, error) {
			return struct{}{}, errors.New("declined upstream")
		})
	// Output: closed open policy-opened
}

func ExampleBreaker_ForceOpen() {
	circuit, _ := breaker.New(breaker.Config{Name: "maintenance"})
	_ = circuit.ForceOpen()
	_, err := circuit.Acquire(context.Background())
	fmt.Println(errors.Is(err, breaker.ErrForceOpen))
	_ = circuit.Release()
	// Output: true
}
