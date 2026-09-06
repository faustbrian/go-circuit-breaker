package breakertest_test

import (
	"fmt"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
	"github.com/faustbrian/go-circuit-breaker/breakertest"
)

func ExampleClock() {
	clock := breakertest.NewClock(time.Unix(100, 0))
	timer := clock.NewTimer(time.Second)
	clock.Advance(time.Second)
	fmt.Println((<-timer.C()).Equal(time.Unix(101, 0)), clock.ActiveTimers())
	// Output: true 0
}

func ExampleRecorder() {
	recorder, err := breakertest.NewRecorder(1)
	if err != nil {
		return
	}
	if err := recorder.Observe(breaker.TransitionEvent{Reason: breaker.ReasonPolicyOpened}); err != nil {
		return
	}
	if err := recorder.Observe(breaker.TransitionEvent{Reason: breaker.ReasonReset}); err != nil {
		return
	}
	fmt.Println(recorder.Events()[0].Reason, recorder.Dropped())
	recorder.Reset()
	fmt.Println(len(recorder.Events()), recorder.Dropped())
	// Output:
	// reset 1
	// 0 0
}

func ExampleScriptedClassifier() {
	classifier := breakertest.NewScriptedClassifier(
		breaker.OutcomeIgnored,
		breaker.OutcomeFailure,
		breaker.OutcomeSuccess,
	)
	fmt.Println(classifier.Classify(breaker.Completion{}))
	fmt.Println(classifier.Classify(breaker.Completion{}))
	fmt.Println(classifier.Classify(breaker.Completion{}))
	fmt.Println(classifier.Calls(), classifier.Remaining())
	// Output:
	// failure
	// success
	// ignored
	// 3 0
}
