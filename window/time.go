package window

import (
	"fmt"
	"time"
)

type timeBucket struct {
	id       int64
	occupied bool
	snapshot Snapshot
}

// Time retains aggregate records in a fixed number of duration buckets.
// Time is not safe for concurrent use; the owning breaker serializes access.
type Time struct {
	bucketDuration time.Duration
	buckets        []timeBucket
	lastBucketID   int64
	initialized    bool
}

// NewTime constructs a time-based window with fixed memory use.
func NewTime(bucketDuration time.Duration, bucketCount int) (*Time, error) {
	if bucketDuration <= 0 {
		return nil, fmt.Errorf("window: bucket duration must be greater than zero")
	}
	if bucketCount <= 0 {
		return nil, fmt.Errorf("window: bucket count must be greater than zero")
	}
	if bucketCount > MaxBucketCount {
		return nil, fmt.Errorf("window: bucket count must not exceed %d", MaxBucketCount)
	}
	if bucketDuration > time.Duration(1<<63-1)/time.Duration(bucketCount) {
		return nil, fmt.Errorf("window: rolling interval overflows time.Duration")
	}

	return &Time{
		bucketDuration: bucketDuration,
		buckets:        make([]timeBucket, bucketCount),
	}, nil
}

// Add records a completion in the bucket containing at. Timestamps older than
// the latest observed timestamp are clamped so clock movement cannot resurrect
// expired data.
func (w *Time) Add(at time.Time, record Record) error {
	if !valid(record) {
		return fmt.Errorf("window: unknown outcome class %d", record.Class)
	}

	id := w.observe(at)
	index := w.index(id)
	bucket := &w.buckets[index]
	if !bucket.occupied || bucket.id != id {
		*bucket = timeBucket{id: id, occupied: true}
	}
	bucket.snapshot.add(record)

	return nil
}

// Snapshot returns aggregates that have not expired as of at.
func (w *Time) Snapshot(at time.Time) Snapshot {
	current := w.observe(at)
	oldest := current - int64(len(w.buckets)) + 1

	var snapshot Snapshot
	for i := range w.buckets {
		bucket := &w.buckets[i]
		if bucket.occupied && bucket.id >= oldest && bucket.id <= current {
			merge(&snapshot, bucket.snapshot)
		}
	}

	return snapshot
}

func (w *Time) observe(at time.Time) int64 {
	id := at.UnixNano() / int64(w.bucketDuration)
	if !w.initialized || id > w.lastBucketID {
		w.lastBucketID = id
		w.initialized = true
	}
	return w.lastBucketID
}

func (w *Time) index(id int64) int {
	index := id % int64(len(w.buckets))
	if index < 0 {
		index += int64(len(w.buckets))
	}
	return int(index)
}

func merge(destination *Snapshot, source Snapshot) {
	destination.Classified += source.Classified
	destination.Successes += source.Successes
	destination.Failures += source.Failures
	destination.Ignored += source.Ignored
	destination.SlowSuccess += source.SlowSuccess
	destination.SlowFailure += source.SlowFailure
}
