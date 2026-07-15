package window_test

import (
	"testing"
	"time"

	"github.com/faustbrian/go-circuit-breaker/window"
)

func FuzzCountMatchesBoundedReference(f *testing.F) {
	f.Add(uint8(3), []byte{0, 1, 2, 1, 0})
	f.Fuzz(func(t *testing.T, rawSize uint8, outcomes []byte) {
		size := int(rawSize%32) + 1
		if len(outcomes) > 4096 {
			t.Skip()
		}
		actual, err := window.NewCount(size)
		if err != nil {
			t.Fatalf("NewCount() error = %v", err)
		}
		var classified []window.Record
		var ignored uint64
		for _, raw := range outcomes {
			record := window.Record{Class: window.Class(raw % 3), Slow: raw&4 != 0}
			if err := actual.Add(record); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			if record.Class == window.Ignored {
				ignored++
				continue
			}
			classified = append(classified, record)
			if len(classified) > size {
				classified = classified[1:]
			}
		}
		want := window.Snapshot{Ignored: ignored}
		for _, record := range classified {
			want.Classified++
			if record.Class == window.Success {
				want.Successes++
				if record.Slow {
					want.SlowSuccess++
				}
			} else {
				want.Failures++
				if record.Slow {
					want.SlowFailure++
				}
			}
		}
		if got := actual.Snapshot(); got != want {
			t.Fatalf("Snapshot() = %+v, want %+v", got, want)
		}
	})
}

func FuzzTimeWindowTimestamps(f *testing.F) {
	f.Add(uint8(4), int64(time.Second), []byte{0, 1, 2}, []byte{0, 1, 5})
	f.Fuzz(func(t *testing.T, rawBuckets uint8, rawDuration int64, outcomes []byte, offsets []byte) {
		buckets := int(rawBuckets%32) + 1
		duration := time.Duration(rawDuration % int64(time.Hour))
		if duration <= 0 {
			duration = time.Nanosecond
		}
		if len(outcomes) > 1024 || len(offsets) > 1024 {
			t.Skip()
		}
		actual, err := window.NewTime(duration, buckets)
		if err != nil {
			t.Fatalf("NewTime() error = %v", err)
		}
		start := time.Unix(100, 0)
		for index, raw := range outcomes {
			offset := int64(index)
			if index < len(offsets) {
				offset = int64(int8(offsets[index]))
			}
			at := start.Add(time.Duration(offset) * duration)
			if err := actual.Add(at, window.Record{Class: window.Class(raw % 3), Slow: raw&4 != 0}); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			_ = actual.Snapshot(at)
		}
	})
}
