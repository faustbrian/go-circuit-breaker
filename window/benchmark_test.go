package window_test

import (
	"testing"
	"time"

	"github.com/faustbrian/go-circuit-breaker/window"
)

// BenchmarkCountRollover times serial failure/slow insertions into a 100-record
// ring, including eviction once full; ResetTimer excludes construction.
func BenchmarkCountRollover(b *testing.B) {
	rolling, err := window.NewCount(100)
	if err != nil {
		b.Fatalf("NewCount() error = %v", err)
	}
	record := window.Record{Class: window.Failure, Slow: true}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = rolling.Add(record)
	}
}

// BenchmarkTimeRollover times insertion plus timestamp arithmetic into 60
// one-second buckets, advancing one second per operation; ResetTimer excludes
// construction.
func BenchmarkTimeRollover(b *testing.B) {
	rolling, err := window.NewTime(time.Second, 60)
	if err != nil {
		b.Fatalf("NewTime() error = %v", err)
	}
	record := window.Record{Class: window.Success}
	now := time.Unix(100, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = rolling.Add(now.Add(time.Duration(index)*time.Second), record)
	}
}

// BenchmarkTimeSnapshot times serial fixed-timestamp scans of 60 filled
// one-second buckets. ResetTimer excludes construction/fill; this does not
// benchmark idle jumps or concurrent ownership.
func BenchmarkTimeSnapshot(b *testing.B) {
	rolling, err := window.NewTime(time.Second, 60)
	if err != nil {
		b.Fatalf("NewTime() error = %v", err)
	}
	now := time.Unix(100, 0)
	for index := range 60 {
		_ = rolling.Add(now.Add(time.Duration(index)*time.Second), window.Record{Class: window.Success})
	}
	now = now.Add(59 * time.Second)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = rolling.Snapshot(now)
	}
}
