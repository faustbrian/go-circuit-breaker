// Package window provides bounded classified-count and time-bucket outcome
// aggregates. Start with NewCount or NewTime; instances require caller
// serialization and never retain results/errors, start workers or own timers.
// Snapshot values are independent copies. Count ignores do not consume ring
// capacity and accumulate until the owner replaces that window; Time ignores
// expire in buckets. Capacities bound allocation, not event volume or uint64
// counter saturation. The package is pure Go without infrastructure/cgo.
package window

const (
	// MaxCountSize bounds the record ring allocated by NewCount.
	MaxCountSize = 1 << 20
	// MaxBucketCount bounds the aggregate bucket ring allocated by NewTime.
	MaxBucketCount = 1 << 16
)

// Class is the mutually exclusive classification of a completion.
type Class uint8

const (
	// Success counts classified evidence and optional slow success.
	Success Class = iota
	// Failure counts classified evidence and optional slow failure.
	Failure
	// Ignored excludes classified/slow evidence; its count retention is window-specific.
	Ignored
)

// Record is one completed operation without retaining its result or error.
type Record struct {
	// Class must be Success, Failure or Ignored; invalid classes reject Add.
	Class Class
	// Slow adds a slow dimension only for Success/Failure; ignored slow is discarded.
	Slow bool
}

// Snapshot is a copied aggregate: classified dimensions describe the retained
// ring/buckets; Count.Ignored is cumulative rather than ring-retained. Time.Ignored
// expires in buckets. Counters use ordinary uint64 arithmetic, not saturation.
type Snapshot struct {
	// Classified equals Successes+Failures and excludes Ignored.
	Classified uint64
	// Successes counts retained classified success records.
	Successes uint64
	// Failures counts retained classified failure records.
	Failures uint64
	// Ignored is cumulative for Count, bucket-retained/expiring for Time.
	Ignored uint64
	// SlowSuccess is the retained slow subset of Successes.
	SlowSuccess uint64
	// SlowFailure is the retained slow subset of Failures.
	SlowFailure uint64
}

func (s *Snapshot) add(record Record) {
	switch record.Class {
	case Success:
		s.Classified++
		s.Successes++
		if record.Slow {
			s.SlowSuccess++
		}
	case Failure:
		s.Classified++
		s.Failures++
		if record.Slow {
			s.SlowFailure++
		}
	case Ignored:
		s.Ignored++
	}
}

func (s *Snapshot) remove(record Record) {
	switch record.Class {
	case Success:
		s.Classified--
		s.Successes--
		if record.Slow {
			s.SlowSuccess--
		}
	case Failure:
		s.Classified--
		s.Failures--
		if record.Slow {
			s.SlowFailure--
		}
	}
}

func valid(record Record) bool {
	return record.Class <= Ignored
}
