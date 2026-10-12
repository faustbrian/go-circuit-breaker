package window

import "fmt"

// Count retains a fixed number of the most recent classified records and a
// separate cumulative ignored count. Obtain it from NewCount; do not copy its
// mutable ring/aggregate state as independent windows. Zero is not configured.
// Count is not safe for concurrent use; the owning breaker serializes access.
type Count struct {
	records  []Record
	next     int
	length   int
	snapshot Snapshot
}

// NewCount constructs a count-based window with fixed memory use.
func NewCount(size int) (*Count, error) {
	if size <= 0 {
		return nil, fmt.Errorf("window: count size must be greater than zero")
	}
	if size > MaxCountSize {
		return nil, fmt.Errorf("window: count size must not exceed %d", MaxCountSize)
	}

	return &Count{records: make([]Record, size)}, nil
}

// Add records a valid classified completion, evicting the oldest classified
// record when full. Ignored only increments the cumulative ignored total, without
// storing/evicting a ring record or counting Slow. Invalid class leaves state
// unchanged and returns an error. Caller serialization is required.
func (w *Count) Add(record Record) error {
	if !valid(record) {
		return fmt.Errorf("window: unknown outcome class %d", record.Class)
	}
	// Ignored events are not dependency evidence and cannot evict classified
	// samples (TestCountIgnoredOutcomesDoNotEvictClassifiedOutcomes).
	if record.Class == Ignored {
		w.snapshot.add(record)
		return nil
	}

	if w.length == len(w.records) {
		w.snapshot.remove(w.records[w.next])
	} else {
		w.length++
	}

	w.records[w.next] = record
	w.next = (w.next + 1) % len(w.records)
	w.snapshot.add(record)

	return nil
}

// Snapshot returns the current aggregate by value.
func (w *Count) Snapshot() Snapshot { return w.snapshot }
