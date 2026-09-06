package window_test

import (
	"fmt"
	"time"

	"github.com/faustbrian/go-circuit-breaker/window"
)

func ExampleCount() {
	recent, err := window.NewCount(2)
	if err != nil {
		return
	}
	if err := recent.Add(window.Record{Class: window.Success}); err != nil {
		return
	}
	if err := recent.Add(window.Record{Class: window.Failure, Slow: true}); err != nil {
		return
	}
	if err := recent.Add(window.Record{Class: window.Ignored}); err != nil {
		return
	}
	snapshot := recent.Snapshot()
	fmt.Println(snapshot.Classified, snapshot.Failures, snapshot.SlowFailure, snapshot.Ignored)
	// Output: 2 1 1 1
}

func ExampleTime() {
	recent, err := window.NewTime(time.Second, 2)
	if err != nil {
		return
	}
	start := time.Unix(100, 0)
	if err := recent.Add(start, window.Record{Class: window.Failure}); err != nil {
		return
	}
	fmt.Println(recent.Snapshot(start).Failures)
	fmt.Println(recent.Snapshot(start.Add(2 * time.Second)).Failures)
	// Output:
	// 1
	// 0
}
