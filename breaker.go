package breaker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/faustbrian/go-circuit-breaker/window"
)

type rollingWindow interface {
	add(time.Time, window.Record)
	snapshot(time.Time) window.Snapshot
}

type countRollingWindow struct{ value *window.Count }

func (w countRollingWindow) add(_ time.Time, record window.Record) {
	_ = w.value.Add(record)
}

func (w countRollingWindow) snapshot(_ time.Time) window.Snapshot {
	return w.value.Snapshot()
}

type timeRollingWindow struct{ value *window.Time }

func (w timeRollingWindow) add(at time.Time, record window.Record) {
	_ = w.value.Add(at, record)
}

func (w timeRollingWindow) snapshot(at time.Time) window.Snapshot {
	return w.value.Snapshot(at)
}

// Breaker protects calls to one caller-defined dependency boundary. Use New;
// its state is concurrency-safe, but it must not be copied and its zero/nil values
// are not usable. Callback dependencies own their concurrency and blocking policy.
// Admission capacity is bounded only in half-open state, not as a global bulkhead.
type Breaker struct {
	mu     sync.Mutex
	config normalizedConfig
	window rollingWindow

	state               State
	mode                Mode
	generation          uint64
	transitionCount     uint64
	lastTransition      time.Time
	consecutiveFailures uint64
	openCount           uint64
	currentOpenDuration time.Duration
	nextProbeAt         time.Time
	halfOpenActive      int
	halfOpenCompleted   int
	halfOpenSuccesses   int
	admitted            uint64
	rejected            uint64
	completed           uint64
	totalSuccesses      uint64
	totalFailures       uint64
	totalIgnored        uint64
	halfOpenPermits     map[*Permit]struct{}
	changed             chan struct{}
	pendingEvents       []TransitionEvent
	eventChannel        chan TransitionEvent
	eventStop           chan struct{}
	eventDone           chan struct{}
	eventCloseOnce      sync.Once
	eventClosed         atomic.Bool
	eventMu             sync.Mutex
	observerCounters    observerCounters
}

// New validates config and constructs normal/closed generation one with empty
// health/lifetime counters. Invalid options return nil with InvalidConfigError.
// Value policies are copied; callbacks are borrowed. Configured asynchronous
// observation starts one worker, for which the creator owns Close/Shutdown.
func New(config Config) (*Breaker, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	rolling := newRollingWindow(normalized)

	b := &Breaker{
		config:          normalized,
		window:          rolling,
		state:           StateClosed,
		mode:            ModeNormal,
		generation:      1,
		halfOpenPermits: make(map[*Permit]struct{}),
		changed:         make(chan struct{}),
	}
	b.startObserver()
	return b, nil
}

func newRollingWindow(config normalizedConfig) rollingWindow {
	if config.timeWindow != nil {
		value, _ := window.NewTime(config.timeWindow.BucketDuration, config.timeWindow.BucketCount)
		return timeRollingWindow{value: value}
	}
	value, _ := window.NewCount(config.countWindowSize)
	return countRollingWindow{value: value}
}

// Permit represents admitted execution bound to both state and generation.
// Obtain it from Acquire, do not copy it, and report Complete or Cancel. Concurrent
// terminal calls serialize exactly once; stale permits can count lifetime outcomes
// but cannot change current health. It never owns or cancels caller operations.
type Permit struct {
	breaker    *Breaker
	generation uint64
	state      State
	deadline   time.Time
	recording  bool
	execution  bool
	status     permitStatus
}

type permitStatus uint8

const (
	permitActive permitStatus = iota
	permitCompleted
	permitCanceled
	permitExpired
)

// Acquire requests permission to execute caller-owned work with a nonnil context.
// Cancellation before linearization returns ctx.Err without admission; once
// admitted, cancellation does not automatically cancel/complete the permit.
// Open/admin rejection returns RejectionError; only configured excess half-open
// admission waits, up to its absolute MaxWait/ctx deadline with one stopped timer.
// Open-to-half-open eligibility and permit expiry are lazily observed, not workers.
func (b *Breaker) Acquire(ctx context.Context) (*Permit, error) {
	return b.acquire(ctx, false)
}

func (b *Breaker) acquire(ctx context.Context, execution bool) (*Permit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var timer Timer
	var timeout <-chan time.Time
	var waitDeadline time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		now := b.config.clock.Now()
		b.mu.Lock()
		if err := ctx.Err(); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		if timer != nil && !now.Before(waitDeadline) {
			err := b.rejectLocked(ErrHalfOpenWaitTimeout)
			b.unlockAndDispatch()
			return nil, err
		}
		permit, err, wait := b.tryAcquireLocked(now)
		if !wait {
			if permit != nil {
				permit.execution = execution
			}
			events := b.takeEventsLocked()
			b.mu.Unlock()
			b.dispatch(events)
			return permit, err
		}
		changed := b.changed
		createTimer := timer == nil
		if timer == nil {
			waitDeadline = now.Add(b.config.halfOpenMaxWait)
		}
		events := b.takeEventsLocked()
		b.mu.Unlock()
		b.dispatch(events)
		if createTimer {
			timer = b.config.clock.NewTimer(b.config.halfOpenMaxWait)
			timeout = timer.C()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			b.mu.Lock()
			err := b.rejectLocked(ErrHalfOpenWaitTimeout)
			b.unlockAndDispatch()
			return nil, err
		case <-changed:
		}
	}
}

func (b *Breaker) tryAcquireLocked(now time.Time) (*Permit, error, bool) {
	switch b.mode {
	case ModeForceOpen:
		return nil, b.rejectLocked(ErrForceOpen), false
	case ModeIsolated:
		return nil, b.rejectLocked(ErrIsolated), false
	case ModeDisabled:
		permit := &Permit{
			breaker:    b,
			generation: b.generation,
			state:      b.state,
			deadline:   now.Add(b.config.permitTTL),
			recording:  false,
		}
		b.admitted++
		return permit, nil, false
	}

	if b.state == StateOpen {
		if now.Before(b.nextProbeAt) {
			return nil, b.rejectLocked(ErrOpen), false
		}
		b.transitionLocked(StateHalfOpen, ReasonOpenIntervalElapsed, now)
	}
	if b.state == StateHalfOpen {
		b.expireHalfOpenLocked(now)
	}
	if b.state == StateHalfOpen &&
		b.halfOpenCompleted+b.halfOpenActive >= b.config.halfOpen.MaxProbes {
		switch b.config.halfOpenWait {
		case true:
			return nil, nil, true
		}
		return nil, b.rejectLocked(ErrHalfOpenExhausted), false
	}

	permit := &Permit{
		breaker:    b,
		generation: b.generation,
		state:      b.state,
		deadline:   now.Add(b.config.permitTTL),
		recording:  true,
	}
	b.admitted++
	if b.state == StateHalfOpen {
		b.halfOpenActive++
		b.halfOpenPermits[permit] = struct{}{}
	}
	return permit, nil, false
}

func (b *Breaker) rejectLocked(cause error) error {
	b.rejected++
	return &RejectionError{
		Name:       b.config.name,
		State:      b.state,
		Mode:       b.mode,
		Generation: b.generation,
		RetryAt:    b.nextProbeAt,
		Cause:      cause,
	}
}

// Complete reports one Outcome and optional slow flag. Invalid enums return
// InvalidOutcomeError without consuming the permit; valid terminal actions are
// exactly once, with stable completed/canceled/expired sentinels on repetition.
// Manual TTL expiry at or after deadline records no outcome. Stale/disabled
// completions count lifetime totals but not current health; ignored is never slow.
// It does not validate operation data. Borrowed Clock/Random may run before status
// checks; a Clock panic still attempts bookkeeping completion then re-panics.
func (p *Permit) Complete(outcome Outcome, slow bool) error {
	if outcome > OutcomeIgnored {
		return &InvalidOutcomeError{Outcome: outcome}
	}
	b := p.breaker
	now, clockPanic := safeClockNow(b.config.clock)
	jitterSample := b.jitterSample()
	if clockPanic != nil {
		_ = p.completeAt(outcome, slow, p.admissionTime(), jitterSample)
		panic(clockPanic)
	}
	return p.completeAt(outcome, slow, now, jitterSample)
}

func (p *Permit) completeAt(outcome Outcome, slow bool, now time.Time, jitterSample float64) error {
	if outcome > OutcomeIgnored {
		return &InvalidOutcomeError{Outcome: outcome}
	}
	b := p.breaker
	b.mu.Lock()
	defer b.unlockAndDispatch()
	switch p.status {
	case permitCompleted:
		return ErrPermitCompleted
	case permitCanceled:
		return ErrPermitCanceled
	case permitExpired:
		if p.execution {
			p.status = permitCompleted
			p.recordLifetimeLocked(outcome)
			return nil
		}
		return ErrPermitExpired
	}
	if !p.execution && !now.Before(p.deadline) {
		p.expireLocked()
		return ErrPermitExpired
	}
	p.status = permitCompleted
	p.recordLifetimeLocked(outcome)
	if p.belongsToCurrentHalfOpenLocked() {
		delete(b.halfOpenPermits, p)
	}
	if !p.recording {
		return nil
	}
	if !p.belongsToCurrentStateLocked() {
		return nil
	}

	if b.state == StateHalfOpen {
		b.completeHalfOpenLocked(outcome, now, jitterSample)
		return nil
	}
	record := window.Record{Class: window.Class(outcome), Slow: slow && outcome != OutcomeIgnored}
	b.window.add(now, record)
	switch outcome {
	case OutcomeSuccess:
		b.consecutiveFailures = 0
	case OutcomeFailure:
		b.consecutiveFailures++
	case OutcomeIgnored:
		if b.config.opening.IgnoredBehavior == ResetConsecutiveFailures {
			b.consecutiveFailures = 0
		}
	}
	snapshot := b.window.snapshot(now)
	if openingDecision(b.config.opening, b.config.minimumThroughput, b.consecutiveFailures, snapshot) {
		b.openLocked(now, ReasonPolicyOpened, jitterSample)
	}
	return nil
}

func (p *Permit) recordLifetimeLocked(outcome Outcome) {
	b := p.breaker
	b.completed++
	switch outcome {
	case OutcomeSuccess:
		b.totalSuccesses++
	case OutcomeFailure:
		b.totalFailures++
	case OutcomeIgnored:
		b.totalIgnored++
	}
}

// Cancel abandons an admitted permit without recording an outcome and releases
// current half-open capacity. It does not cancel protected work or its context.
// Repetition/TTL expiry returns the corresponding terminal sentinel. Clock panic
// still attempts bookkeeping cancellation then re-panics. Use a nonnil permit.
func (p *Permit) Cancel() error {
	b := p.breaker
	now, clockPanic := safeClockNow(b.config.clock)
	if clockPanic != nil {
		_ = p.cancelAt(p.admissionTime())
		panic(clockPanic)
	}
	return p.cancelAt(now)
}

func (p *Permit) admissionTime() time.Time {
	return p.deadline.Add(-p.breaker.config.permitTTL)
}

func (p *Permit) cancelAt(now time.Time) error {
	b := p.breaker
	b.mu.Lock()
	defer b.unlockAndDispatch()
	switch p.status {
	case permitCompleted:
		return ErrPermitCompleted
	case permitCanceled:
		return ErrPermitCanceled
	case permitExpired:
		return ErrPermitExpired
	}
	if !now.Before(p.deadline) {
		p.expireLocked()
		return ErrPermitExpired
	}
	p.status = permitCanceled
	p.releaseHalfOpenLocked()
	return nil
}

func (b *Breaker) jitterSample() float64 {
	if b.config.openDurationJitter == 0 {
		return 0
	}
	return safeRandomSample(b.config.random)
}

func (p *Permit) expireLocked() {
	if p.status != permitActive {
		return
	}
	p.status = permitExpired
	p.releaseHalfOpenLocked()
}

func (p *Permit) releaseHalfOpenLocked() {
	b := p.breaker
	if !p.belongsToCurrentHalfOpenLocked() {
		return
	}
	if _, exists := b.halfOpenPermits[p]; !exists {
		return
	}
	delete(b.halfOpenPermits, p)
	b.halfOpenActive--
	b.signalLocked()
}

// Both generation and state bind health mutation; old completions still contribute
// lifetime accounting but must not refill a reset or recovered health window.
func (p *Permit) belongsToCurrentStateLocked() bool {
	b := p.breaker
	type epoch struct {
		generation uint64
		state      State
	}
	return epoch{generation: p.generation, state: p.state} ==
		epoch{generation: b.generation, state: b.state}
}

func (p *Permit) belongsToCurrentHalfOpenLocked() bool {
	if p.state != StateHalfOpen {
		return false
	}
	return p.belongsToCurrentStateLocked()
}

func (b *Breaker) expireHalfOpenLocked(now time.Time) {
	for permit := range b.halfOpenPermits {
		if !now.Before(permit.deadline) {
			permit.expireLocked()
		}
	}
}

func (b *Breaker) completeHalfOpenLocked(outcome Outcome, now time.Time, jitterSample float64) {
	b.halfOpenActive--
	b.signalLocked()
	// Ignored probes free active capacity but do not spend the classified sample
	// quota; replacement probes must remain possible (half_open_table_test.go).
	if outcome == OutcomeIgnored {
		return
	}
	b.halfOpenCompleted++
	if outcome == OutcomeSuccess {
		b.halfOpenSuccesses++
	} else if b.config.halfOpen.FailureAction == ReopenImmediately {
		b.openLocked(now, ReasonHalfOpenFailed, jitterSample)
		return
	}

	policy := b.config.halfOpen
	if policy.RequiredSuccesses > 0 && b.halfOpenSuccesses >= policy.RequiredSuccesses {
		b.closeLocked(now, ReasonHalfOpenRecovered)
		return
	}
	if policy.SuccessRatio > 0 && b.halfOpenCompleted >= policy.MaxProbes {
		if ratio(uint64(b.halfOpenSuccesses), uint64(b.halfOpenCompleted)) >= policy.SuccessRatio { // #nosec G115 -- mutex-owned exactly-once completions keep both counters in [0, MaxProbes].
			b.closeLocked(now, ReasonHalfOpenRecovered)
		} else {
			b.openLocked(now, ReasonHalfOpenFailed, jitterSample)
		}
		return
	}
	if policy.FailureAction == ReopenAfterSample && b.halfOpenCompleted >= policy.MaxProbes {
		b.openLocked(now, ReasonHalfOpenFailed, jitterSample)
	}
}

func (b *Breaker) openLocked(now time.Time, reason TransitionReason, jitterSample float64) {
	b.transitionLocked(StateOpen, reason, now, func() {
		b.openCount++
		b.currentOpenDuration = b.openDurationLocked(jitterSample)
		b.nextProbeAt = now.Add(b.currentOpenDuration)
	})
}

func (b *Breaker) closeLocked(now time.Time, reason TransitionReason) {
	b.transitionLocked(StateClosed, reason, now, func() {
		b.window = newRollingWindow(b.config)
		b.consecutiveFailures = 0
		b.openCount = 0
		b.currentOpenDuration = 0
		b.nextProbeAt = time.Time{}
	})
}

func (b *Breaker) transitionLocked(
	state State,
	reason TransitionReason,
	now time.Time,
	mutate ...func(),
) {
	before := b.snapshotLocked(now)
	for _, mutation := range mutate {
		mutation()
	}
	b.state = state
	b.generation++
	b.transitionCount++
	b.lastTransition = now
	b.halfOpenActive = 0
	b.halfOpenCompleted = 0
	b.halfOpenSuccesses = 0
	b.halfOpenPermits = make(map[*Permit]struct{})
	b.signalLocked()
	b.recordEventLocked(before, reason, now)
}

// Close/replace is a mutex-owned broadcast, not a worker channel: every waiter
// captures one revision and retries admission under the same absolute deadline.
func (b *Breaker) signalLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

func (b *Breaker) recordEventLocked(before Snapshot, reason TransitionReason, now time.Time) {
	switch b.config.observer.mode {
	case observerDisabled:
		return
	}
	after := b.snapshotLocked(now)
	b.pendingEvents = append(b.pendingEvents, TransitionEvent{
		Before:     before,
		After:      after,
		Reason:     reason,
		Generation: after.Generation,
		Timestamp:  now,
	})
}

// SetMode applies an administrative mode without rewriting policy state/window
// or open schedule. Changing mode advances generation, invalidates health mutation
// by outstanding permits and resets the probe sample; lifetime totals persist.
// The current mode is a no-op; unknown values return an error without mutation.
func (b *Breaker) SetMode(mode Mode) error {
	if mode > ModeIsolated {
		return fmt.Errorf("breaker: unknown administrative mode %d", mode)
	}
	now := b.config.clock.Now()
	b.mu.Lock()
	defer b.unlockAndDispatch()
	switch b.mode {
	case mode:
		return nil
	}
	before := b.snapshotLocked(now)
	b.mode = mode
	b.generation++
	b.transitionCount++
	b.lastTransition = now
	b.halfOpenActive = 0
	b.halfOpenCompleted = 0
	b.halfOpenSuccesses = 0
	b.halfOpenPermits = make(map[*Permit]struct{})
	b.signalLocked()
	b.recordEventLocked(before, modeReason(mode), now)
	return nil
}

func modeReason(mode Mode) TransitionReason {
	switch mode {
	case ModeForceOpen:
		return ReasonForceOpen
	case ModeDisabled:
		return ReasonDisabled
	case ModeIsolated:
		return ReasonIsolated
	case ModeNormal:
		return ReasonReleased
	default:
		panic("breaker: unreachable mode")
	}
}

// ForceOpen rejects work independently of the policy-driven state.
func (b *Breaker) ForceOpen() error { return b.SetMode(ModeForceOpen) }

// Disable admits work without recording health-policy outcomes. Successful
// terminal completions still contribute lifetime totals; it does not cancel work.
func (b *Breaker) Disable() error { return b.SetMode(ModeDisabled) }

// Isolate rejects work for operator-controlled maintenance.
func (b *Breaker) Isolate() error { return b.SetMode(ModeIsolated) }

// Release returns administrative control to normal policy-driven operation.
func (b *Breaker) Release() error { return b.SetMode(ModeNormal) }

// Reset starts a new normal/closed generation with empty health window, failure
// streak, probe sample and open schedule. It retains lifetime/observer totals and
// does not cancel outstanding operations, whose old permits become health-stale.
func (b *Breaker) Reset() error {
	now := b.config.clock.Now()
	b.mu.Lock()
	defer b.unlockAndDispatch()
	b.transitionLocked(StateClosed, ReasonReset, now, func() {
		b.window = newRollingWindow(b.config)
		b.mode = ModeNormal
		b.consecutiveFailures = 0
		b.openCount = 0
		b.currentOpenDuration = 0
		b.nextProbeAt = time.Time{}
	})
	return nil
}

// Duration multiplication is float64-truncated. A nonincreasing conversion also
// covers overflow-like results: choose the cap instead of wrapping the schedule.
func (b *Breaker) openDurationLocked(jitterSample float64) time.Duration {
	var duration time.Duration
	switch policy := b.config.openDuration.(type) {
	case FixedOpenDuration:
		duration = time.Duration(policy)
	case ExponentialOpenDuration:
		duration = policy.Initial
		if policy.Multiplier == 1 {
			break
		}
		for attempt := uint64(1); attempt < b.openCount; attempt++ {
			next := time.Duration(float64(duration) * policy.Multiplier)
			if next <= duration {
				return b.jitterDuration(policy.Maximum, jitterSample)
			}
			duration = min(next, policy.Maximum)
			if duration == policy.Maximum {
				return b.jitterDuration(policy.Maximum, jitterSample)
			}
		}
	default:
		panic("breaker: unreachable open duration policy")
	}
	return b.jitterDuration(duration, jitterSample)
}

func (b *Breaker) jitterDuration(duration time.Duration, sample float64) time.Duration {
	if b.config.openDurationJitter == 0 {
		return duration
	}
	jittered := time.Duration(float64(duration) * (1 - b.config.openDurationJitter*sample))
	if jittered <= 0 {
		return 1
	}
	return jittered
}

func safeRandomSample(random Random) (sample float64) {
	defer func() {
		if recover() != nil {
			sample = 0
		}
	}()
	sample = random.Float64()
	if !finite(sample) {
		return 0
	}
	sample = max(sample, 0)
	if sample >= 1 {
		return 0
	}
	return sample
}

// Snapshot is a copied view of mutex-owned breaker/health state with independent
// atomic observer counters. Callers may mutate their copy without affecting the
// breaker. Counters/generations are ordinary uint64, not saturating or distributed
// fencing identities; only configured storage/probe bounds are capacity limits.
type Snapshot struct {
	// Name is caller-configured diagnostic identity, not automatically redacted.
	Name string
	// State is policy state, not effective administrative admission mode.
	State State
	// Mode is the administrative admission override.
	Mode Mode
	// Generation is the local state/admin epoch; construction begins at one.
	Generation uint64
	// TransitionCount counts state/admin/reset transitions, excluding same-mode calls.
	TransitionCount uint64
	// LastTransition is the latest committed transition timestamp, initially zero;
	// it can repeat or move backward with caller clock behavior.
	LastTransition time.Time
	// WindowClassified counts retained health successes plus failures, excluding ignored.
	WindowClassified uint64
	// WindowSize is a compatibility alias of WindowClassified, not all retained outcomes.
	WindowSize uint64
	// WindowCapacity is count-record capacity or time-bucket count, not throughput.
	WindowCapacity int
	// MinimumThroughput is configured classified evidence required by opening rules.
	MinimumThroughput int
	// Successes is retained health-window success count, not half-open probe progress.
	Successes uint64
	// Failures is retained health-window failure count.
	Failures uint64
	// Ignored is cumulative since count-window reset, or retained in time buckets.
	// Ignored never contributes to WindowClassified or slow ratios.
	Ignored uint64
	// SlowSuccesses is the retained subset of slow classified successes.
	SlowSuccesses uint64
	// SlowFailures is the retained subset of slow classified failures.
	SlowFailures uint64
	// Admitted is lifetime admission count, including disabled work and unfinished permits.
	Admitted uint64
	// Rejected is lifetime policy/admin/wait rejection count; pre-admission ctx
	// cancellation does not increment it.
	Rejected uint64
	// Completed is lifetime successful terminal outcome count, including stale/disabled
	// and internally expired Execute completions, but not manual cancellation/expiry.
	Completed uint64
	// TotalSuccesses is the success subset of Completed, preserved by Reset.
	TotalSuccesses uint64
	// TotalFailures is the failure subset of Completed, preserved by Reset.
	TotalFailures uint64
	// TotalIgnored is the ignored subset of Completed, preserved by Reset.
	TotalIgnored uint64
	// ActiveHalfOpen counts current-generation active probes after lazy expiry.
	ActiveHalfOpen int
	// HalfOpenCompleted counts current-generation classified probe outcomes only.
	HalfOpenCompleted int
	// HalfOpenSuccesses is the success subset of HalfOpenCompleted.
	HalfOpenSuccesses int
	// FailureRatio is float64 Failures/WindowClassified, or zero if undefined.
	FailureRatio float64
	// FailureRatioDefined distinguishes evidence from a numeric zero ratio.
	FailureRatioDefined bool
	// SlowRatio is float64 (SlowSuccesses+SlowFailures)/WindowClassified.
	SlowRatio float64
	// SlowRatioDefined is false with no classified health outcomes; ratio is then zero.
	SlowRatioDefined bool
	// CurrentOpenDuration is the selected post-jitter interval retained until
	// recovery/reset; administrative modes do not recompute it.
	CurrentOpenDuration time.Duration
	// NextProbeAt is lazy open eligibility, not guaranteed future admission or retry.
	NextProbeAt time.Time
	// ObserverFailures counts callback errors/panics independently of health counters.
	ObserverFailures uint64
	// DroppedEvents counts async queue overflow and events refused after Close.
	DroppedEvents uint64
}

// Snapshot returns a copied state/health view, lazily expiring half-open permits
// and time-window data at Clock.Now. It does not advance an open state to half-open
// or wait for observer callbacks; failure/drop counters are independent samples.
// Clock may block/panic before locking. A nonnil constructed Breaker is required.
func (b *Breaker) Snapshot() Snapshot {
	now := b.config.clock.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateHalfOpen:
		b.expireHalfOpenLocked(now)
	}
	return b.snapshotLocked(now)
}

func (b *Breaker) snapshotLocked(now time.Time) Snapshot {
	aggregate := b.window.snapshot(now)
	slow := aggregate.SlowSuccess + aggregate.SlowFailure
	windowCapacity := b.config.countWindowSize
	if b.config.timeWindow != nil {
		windowCapacity = b.config.timeWindow.BucketCount
	}
	ratioDefined := aggregate.Classified > 0
	return Snapshot{
		Name:                b.config.name,
		State:               b.state,
		Mode:                b.mode,
		Generation:          b.generation,
		TransitionCount:     b.transitionCount,
		LastTransition:      b.lastTransition,
		WindowClassified:    aggregate.Classified,
		WindowSize:          aggregate.Classified,
		WindowCapacity:      windowCapacity,
		MinimumThroughput:   b.config.minimumThroughput,
		Successes:           aggregate.Successes,
		Failures:            aggregate.Failures,
		Ignored:             aggregate.Ignored,
		SlowSuccesses:       aggregate.SlowSuccess,
		SlowFailures:        aggregate.SlowFailure,
		Admitted:            b.admitted,
		Rejected:            b.rejected,
		Completed:           b.completed,
		TotalSuccesses:      b.totalSuccesses,
		TotalFailures:       b.totalFailures,
		TotalIgnored:        b.totalIgnored,
		ActiveHalfOpen:      b.halfOpenActive,
		HalfOpenCompleted:   b.halfOpenCompleted,
		HalfOpenSuccesses:   b.halfOpenSuccesses,
		FailureRatio:        ratio(aggregate.Failures, aggregate.Classified),
		FailureRatioDefined: ratioDefined,
		SlowRatio:           ratio(slow, aggregate.Classified),
		SlowRatioDefined:    ratioDefined,
		CurrentOpenDuration: b.currentOpenDuration,
		NextProbeAt:         b.nextProbeAt,
		ObserverFailures:    b.observerCounters.failures.Load(),
		DroppedEvents:       b.observerCounters.dropped.Load(),
	}
}
