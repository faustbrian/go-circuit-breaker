// Package breakertest provides manually driven clock/timer, recorder and
// classifier seams for breaker tests. NewClock/NewRecorder/NewScriptedClassifier
// own copied support state, not operations, servers or background workers.
// Calls are mutex-safe but tests still own scenario synchronization and manual
// time progress; these tools do not simulate scheduler fairness or network timing.
package breakertest

import (
	"sort"
	"sync"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

// Clock is a manually advanced, concurrency-safe breaker clock without timer
// goroutines. Its zero value starts at time.Time{}; do not copy an initialized
// clock. Compound Advance is not an atomic concurrent read-modify-write.
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*Timer
}

// NewClock constructs a clock at start.
func NewClock(start time.Time) *Clock { return &Clock{now: start} }

// Now returns the current synthetic time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer constructs a timer owned by this clock. Nonpositive duration buffers
// its deadline immediately; positive timers fire only through Set/Advance. No
// worker or wall-clock wakeup is started; stop waiting timers when done.
func (c *Clock) NewTimer(duration time.Duration) breaker.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &Timer{
		clock:    c,
		deadline: c.now.Add(duration),
		channel:  make(chan time.Time, 1),
	}
	if duration <= 0 {
		timer.fired = true
		timer.channel <- timer.deadline
		return timer
	}
	c.timers = append(c.timers, timer)
	return timer
}

// Advance sets the observed current time plus duration and fires eligible timers.
// Negative duration is allowed. Now and Set are separately synchronized, so
// concurrent Advances can overwrite one another rather than sum atomically.
func (c *Clock) Advance(duration time.Duration) { c.Set(c.Now().Add(duration)) }

// Set moves the clock to an arbitrary timestamp. Forward movement fires every
// eligible timer; backward movement never resurrects fired timers. Eligible
// notifications carry each scheduled deadline, not at. Equal deadlines preserve
// registration order when enqueued; receiving/select scheduling remains test-owned.
func (c *Clock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
	sort.SliceStable(c.timers, func(i, j int) bool {
		return c.timers[i].deadline.Before(c.timers[j].deadline)
	})
	retained := c.timers
	active := retained[:0]
	for _, timer := range c.timers {
		if !c.now.Before(timer.deadline) {
			timer.fired = true
			timer.channel <- timer.deadline
			continue
		}
		active = append(active, timer)
	}
	clear(retained[len(active):])
	c.timers = active
}

// ActiveTimers reports timers that have neither fired nor stopped.
func (c *Clock) ActiveTimers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, timer := range c.timers {
		if !timer.stopped {
			count++
		}
	}
	return count
}

// Timer is a single-shot timer obtained from Clock.NewTimer. It owns a buffered
// deadline notification channel that is never closed. C/Stop may be used while
// the clock advances; the zero value is not a usable stoppable timer.
type Timer struct {
	clock    *Clock
	deadline time.Time
	channel  chan time.Time
	stopped  bool
	fired    bool
}

// C returns the timer notification channel.
func (t *Timer) C() <-chan time.Time { return t.channel }

// Stop prevents a pending timer from firing and removes it from retained clock
// timers. It returns false if already fired/stopped and does not drain a delivered
// notification. It starts no background cleanup.
func (t *Timer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	for index, timer := range t.clock.timers {
		if timer != t {
			continue
		}
		copy(t.clock.timers[index:], t.clock.timers[index+1:])
		last := len(t.clock.timers) - 1
		t.clock.timers[last] = nil
		t.clock.timers = t.clock.timers[:last]
		break
	}
	return true
}
