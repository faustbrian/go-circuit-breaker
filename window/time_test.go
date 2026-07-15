package window_test

import (
	"testing"
	"time"

	"github.com/faustbrian/go-circuit-breaker/window"
)

func TestTimeExpiresBucketsAtExactBoundary(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 3)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	start := time.Unix(100, 0)

	if err := w.Add(start, window.Record{Class: window.Failure}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := w.Add(start.Add(2*time.Second), window.Record{Class: window.Success}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	before := w.Snapshot(start.Add(2999 * time.Millisecond))
	if before.Classified != 2 || before.Failures != 1 {
		t.Fatalf("Snapshot(before expiry) = %+v", before)
	}

	atBoundary := w.Snapshot(start.Add(3 * time.Second))
	if atBoundary.Classified != 1 || atBoundary.Successes != 1 {
		t.Fatalf("Snapshot(at expiry) = %+v", atBoundary)
	}
}

func TestTimeClearsAllBucketsAfterIdleGap(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 2)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	start := time.Unix(100, 0)

	if err := w.Add(start, window.Record{Class: window.Failure, Slow: true}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := w.Snapshot(start.Add(time.Hour)); got != (window.Snapshot{}) {
		t.Fatalf("Snapshot() after idle gap = %+v, want empty", got)
	}
}

func TestTimeDoesNotResurrectExpiredDataAfterClockMovesBackward(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 2)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	start := time.Unix(100, 0)

	if err := w.Add(start, window.Record{Class: window.Failure}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := w.Snapshot(start.Add(3 * time.Second)); got != (window.Snapshot{}) {
		t.Fatalf("Snapshot() after expiry = %+v, want empty", got)
	}
	if got := w.Snapshot(start); got != (window.Snapshot{}) {
		t.Fatalf("Snapshot() after backward jump = %+v, want empty", got)
	}
}

func TestTimeClampsBackwardCompletionToLatestObservedBucket(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 2)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	start := time.Unix(100, 0)

	_ = w.Snapshot(start.Add(3 * time.Second))
	if err := w.Add(start, window.Record{Class: window.Success}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got := w.Snapshot(start.Add(3 * time.Second))
	if got.Classified != 1 || got.Successes != 1 {
		t.Fatalf("Snapshot() = %+v, want clamped success", got)
	}
}

func TestNewTimeValidatesBounds(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		duration time.Duration
		buckets  int
	}{
		{duration: 0, buckets: 2},
		{duration: -time.Second, buckets: 2},
		{duration: time.Second, buckets: 0},
		{duration: time.Second, buckets: -1},
	} {
		if _, err := window.NewTime(test.duration, test.buckets); err == nil {
			t.Fatalf("NewTime(%v, %d) error = nil", test.duration, test.buckets)
		}
	}
}

func TestTimeRejectsUnknownClassWithoutMutation(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 2)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	if err := w.Add(time.Unix(100, 0), window.Record{Class: window.Class(99)}); err == nil {
		t.Fatal("Add() error = nil")
	}
	if got := w.Snapshot(time.Unix(100, 0)); got != (window.Snapshot{}) {
		t.Fatalf("Snapshot() = %+v", got)
	}
}

func TestTimeSupportsPreEpochBuckets(t *testing.T) {
	t.Parallel()

	w, err := window.NewTime(time.Second, 2)
	if err != nil {
		t.Fatalf("NewTime() error = %v", err)
	}
	at := time.Unix(-1, 0)
	if err := w.Add(at, window.Record{Class: window.Success}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := w.Snapshot(at); got.Successes != 1 {
		t.Fatalf("Snapshot() = %+v", got)
	}
}
