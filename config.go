package breaker

import (
	"context"
	"math"
	rand "math/rand/v2"
	"reflect"
	"time"

	windowpkg "github.com/faustbrian/go-circuit-breaker/window"
)

const (
	defaultCountWindowSize    = 100
	defaultMinimumThroughput  = 10
	defaultSlowCallDuration   = 30 * time.Second
	defaultOpenDuration       = 30 * time.Second
	defaultHalfOpenProbeCount = 10
	defaultPermitTTL          = 5 * time.Minute
	// MaxHalfOpenProbes bounds simultaneously retained half-open permits.
	MaxHalfOpenProbes = 1 << 16
	// MaxEventBuffer bounds the asynchronous observer queue allocation.
	MaxEventBuffer = 1 << 16
	// MaxNameBytes bounds retained breaker identity and diagnostic rendering.
	MaxNameBytes = 256
)

// WindowConfig selects a bounded rolling outcome window. Only CountWindow and
// TimeWindow values are supported; nil selects CountWindow{Size: 100}. Their
// pointers are not alternate configurations, despite promoted marker methods.
type WindowConfig interface{ windowConfig() }

// CountWindow retains the most recent Size classified outcomes, while ignored
// completions accumulate separately without evicting them.
type CountWindow struct {
	// Size must be positive and at most window.MaxCountSize; zero is invalid.
	Size int
}

func (CountWindow) windowConfig() {}

// TimeWindow retains aggregates in a bounded number of fixed-duration buckets.
type TimeWindow struct {
	// BucketDuration is positive; BucketDuration*BucketCount must fit time.Duration.
	BucketDuration time.Duration
	// BucketCount is positive and at most window.MaxBucketCount. Buckets, not
	// requests, bound allocation; each aggregates potentially many outcomes.
	BucketCount int
}

func (TimeWindow) windowConfig() {}

// RuleCombination determines how multiple enabled opening thresholds compose.
type RuleCombination uint8

const (
	// OpenWhenAny opens when at least one enabled rule is satisfied.
	OpenWhenAny RuleCombination = iota
	// OpenWhenAll opens only when every enabled rule is satisfied.
	OpenWhenAll
)

// IgnoredConsecutiveBehavior controls whether an ignored completion changes
// the consecutive-failure streak.
type IgnoredConsecutiveBehavior uint8

const (
	// PreserveConsecutiveFailures leaves the failure streak unchanged on ignored calls.
	PreserveConsecutiveFailures IgnoredConsecutiveBehavior = iota
	// ResetConsecutiveFailures clears the streak on ignored calls.
	ResetConsecutiveFailures
)

// OpeningRules contains policy thresholds. Zero disables an individual rule.
// Enabled ratios are in (0, 1]; all rules also require MinimumThroughput.
type OpeningRules struct {
	// ConsecutiveFailures opens at this failure streak; zero disables it.
	ConsecutiveFailures uint64
	// FailureCount opens at this many retained failures; zero disables it.
	FailureCount uint64
	// FailureRatio compares failures/classified using float64; zero disables it.
	FailureRatio float64
	// SlowCount compares retained slow successes plus failures; zero disables it.
	SlowCount uint64
	// SlowRatio compares slow/classified using float64; zero disables it.
	SlowRatio float64
	// Combination selects any (zero) or all enabled rules; unknown values fail.
	Combination RuleCombination
	// IgnoredBehavior preserves (zero) or resets the failure streak on ignored outcomes.
	IgnoredBehavior IgnoredConsecutiveBehavior
}

// OpenDurationPolicy selects a schedule for policy-driven openings. Supported
// values are FixedOpenDuration and ExponentialOpenDuration, not their pointers;
// nil selects a fixed 30 seconds. Recovery/reset clears the opening attempt count.
type OpenDurationPolicy interface{ openDurationPolicy() }

// Random supplies samples for optional downward scheduling jitter, not security.
// It is borrowed, may be called concurrently/outside the state mutex, and can be
// called even when a completion does not ultimately open the breaker.
type Random interface {
	// Float64 should return [0, 1). Negative samples clamp to zero; non-finite,
	// >=1 or panicking samples use zero, leaving duration unjittered.
	Float64() float64
}

// FixedOpenDuration uses the same positive duration for every opening.
type FixedOpenDuration time.Duration

func (FixedOpenDuration) openDurationPolicy() {}

// ExponentialOpenDuration grows on each policy opening until recovery/reset,
// capped at Maximum. Multiplication uses float64 then duration truncation, not
// exact integer exponentiation; jitter is applied after selecting the schedule.
type ExponentialOpenDuration struct {
	// Initial is the positive first-opening duration.
	Initial time.Duration
	// Multiplier is finite and at least one; one keeps Initial for every opening.
	Multiplier float64
	// Maximum is at least Initial and caps the pre-jitter schedule.
	Maximum time.Duration
}

func (ExponentialOpenDuration) openDurationPolicy() {}

// HalfOpenFailureAction controls when a failed probe reopens the breaker.
type HalfOpenFailureAction uint8

const (
	// ReopenImmediately opens on the first classified failure in the probe sample.
	ReopenImmediately HalfOpenFailureAction = iota
	// ReopenAfterSample defers failure until the classified sample fills, unless
	// the configured success threshold recovers earlier.
	ReopenAfterSample
)

// HalfOpenPolicy bounds active plus classified-completed probes in one generation
// and selects exactly one recovery threshold. Ignored/canceled/expired probes
// release capacity without consuming classified quota. Slow flags do not affect
// half-open recovery and these outcomes do not alter the retained health window.
type HalfOpenPolicy struct {
	// MaxProbes is positive through MaxHalfOpenProbes, bounding classified
	// completions plus active probes, not only simultaneous work.
	MaxProbes int
	// RequiredSuccesses selects count recovery when positive, at most MaxProbes;
	// nonpositive disables count recovery when SuccessRatio is enabled.
	RequiredSuccesses int
	// SuccessRatio selects recovery at a full classified MaxProbes sample when
	// positive, at most one; zero disables it. Exactly one threshold must be enabled.
	SuccessRatio float64
	// FailureAction reopens immediately (zero) or after the sample; unknown values fail.
	FailureAction HalfOpenFailureAction
}

// HalfOpenAdmissionPolicy controls excess callers while probe quota is full.
// Supported values are RejectExcessProbes (also the nil default) and WaitForProbe;
// pointers are unsupported. It does not bound total closed-state concurrency.
type HalfOpenAdmissionPolicy interface{ halfOpenAdmissionPolicy() }

// RejectExcessProbes rejects immediately when active plus classified-completed
// probes fill the half-open quota.
type RejectExcessProbes struct{}

func (RejectExcessProbes) halfOpenAdmissionPolicy() {}

// WaitForProbe waits context-sensitively for capacity up to MaxWait.
type WaitForProbe struct {
	// MaxWait is positive and bounds the whole wait, including admission retries.
	// Context cancellation can end it sooner; one borrowed-clock timer is stopped
	// on return. It does not guarantee scheduling fairness or callback deadlines.
	MaxWait time.Duration
}

func (WaitForProbe) halfOpenAdmissionPolicy() {}

// Clock supplies shared timestamps and admission timers. It is borrowed for the
// Breaker lifetime; providers own concurrent safety/coherent time and run outside
// the state mutex. No deadline/recovery is imposed on provider calls. Backward
// operation duration clamps to zero; rolling time buckets clamp observed progress.
type Clock interface {
	// Now returns the current timestamp; it must not require the state mutex.
	Now() time.Time
	// NewTimer returns an owned nonnil timer usable concurrently with clock calls.
	// Acquire stops it on every return path after successful construction.
	NewTimer(time.Duration) Timer
}

// Timer is the minimal provider-owned timer used by context-aware admission.
// Its receive channel stays usable until Stop; providers must not close it early
// as closure would be observed as an admission timeout. No timer reset is needed.
type Timer interface {
	// C returns a stable receive channel for timer expiry notification.
	C() <-chan time.Time
	// Stop releases/prevents pending timer activity and reports whether stopped
	// before expiry; it need not drain a notification already sent.
	Stop() bool
}

// Completion is the ephemeral input to an outcome classifier. Implementations
// must not retain Context, Result, or Err after returning.
type Completion struct {
	// Context is the operation caller context, borrowed for classifier use only.
	Context context.Context
	// Result is the original operation result; do not retain it.
	Result any
	// Err is the original operation error, not a redacted diagnostic; do not retain it.
	Err error
	// Duration is elapsed operation time, clamped at zero for backward clocks.
	Duration time.Duration
}

// Classifier maps an admitted completion to one Outcome synchronously outside
// the mutex. It is borrowed and must be safe when Execute calls overlap, retain
// no Completion data and supply its own blocking policy. Nil selects success for
// nil Err and failure otherwise (including typed-nil error interfaces). Invalid
// enums fail completion; panics are failure-recorded and re-panicked by Execute.
type Classifier func(Completion) Outcome

// Config contains breaker construction parameters. New copies value policies,
// not collaborator state. Keep borrowed Clock/Random/Classifier/Observer valid and
// concurrently safe for the breaker/worker lifetime. This is not a request budget.
type Config struct {
	// Name is retained identity: nonempty, at most MaxNameBytes. It is rendered
	// in snapshots/events/rejections, not validated as a secret-safe label.
	Name string
	// Window selects classified count/time storage; nil chooses 100 count records.
	Window WindowConfig
	// MinimumThroughput is positive classified evidence required by every rule;
	// zero selects 10. Count windows require it not to exceed Size.
	MinimumThroughput int
	// SlowCallDuration is a positive inclusive elapsed-time threshold; zero
	// selects 30 seconds. Ignored outcomes never count as slow.
	SlowCallDuration time.Duration
	// Opening is copied; nil selects failure ratio 0.5 with OpenWhenAny. A
	// non-nil zero policy is invalid because at least one rule must be enabled.
	Opening *OpeningRules
	// OpenDuration selects fixed/exponential policy; nil selects 30 seconds.
	OpenDuration OpenDurationPolicy
	// HalfOpen is copied; nil selects ten probes, ten required successes and
	// immediate failure reopening. A non-nil zero policy is invalid.
	HalfOpen *HalfOpenPolicy
	// Clock is borrowed; nil selects the system clock and typed nil is invalid.
	Clock Clock
	// Classifier is borrowed; nil classifies any nonnil error as failure.
	Classifier Classifier
	// PermitTTL is positive; zero selects five minutes. Manual permits expire
	// without outcomes; internally owned Execute permits retain lifetime accounting.
	// Expiry is lazy bookkeeping, never operation cancellation.
	PermitTTL time.Duration
	// HalfOpenAdmission defaults to fail-fast excess rejection; WaitForProbe
	// adds finite caller-context-sensitive waiting, not a global queue bound.
	HalfOpenAdmission HalfOpenAdmissionPolicy
	// Observer is borrowed; nil disables events. Async mode may start a worker
	// at construction; captures must be ready before the first transition callback.
	Observer Observer
	// EventDelivery requires Observer. Nil with an observer chooses async
	// buffer 64/DropNewestEvent. Synchronous delivery has no worker to shut down.
	EventDelivery EventDeliveryPolicy
	// OpenDurationJitter is finite in [0, 1); zero disables sampling. Jitter
	// only shortens the selected interval, with a one-nanosecond lower bound.
	OpenDurationJitter float64
	// Random is borrowed for jitter; nil selects math/rand/v2, typed nil fails.
	// This is scheduling randomness, not cryptographic material.
	Random Random
}

type normalizedConfig struct {
	name               string
	countWindowSize    int
	timeWindow         *TimeWindow
	minimumThroughput  int
	slowCallDuration   time.Duration
	opening            OpeningRules
	openDuration       OpenDurationPolicy
	halfOpen           HalfOpenPolicy
	clock              Clock
	classifier         Classifier
	permitTTL          time.Duration
	halfOpenWait       bool
	halfOpenMaxWait    time.Duration
	observer           observerRuntime
	openDurationJitter float64
	random             Random
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	if config.Name == "" {
		return normalizedConfig{}, invalidConfig("Name", "must not be empty")
	}
	if len(config.Name) > MaxNameBytes {
		return normalizedConfig{}, invalidConfig("Name", "exceeds maximum length")
	}

	windowSize, timeWindow, err := normalizeWindow(config.Window)
	if err != nil {
		return normalizedConfig{}, err
	}
	if config.MinimumThroughput < 0 {
		return normalizedConfig{}, invalidConfig("MinimumThroughput", "must not be negative")
	}
	minimumThroughput := config.MinimumThroughput
	if minimumThroughput == 0 {
		minimumThroughput = defaultMinimumThroughput
	}
	if timeWindow == nil && minimumThroughput > windowSize {
		return normalizedConfig{}, invalidConfig("MinimumThroughput", "must not exceed count window size")
	}

	if config.SlowCallDuration < 0 {
		return normalizedConfig{}, invalidConfig("SlowCallDuration", "must not be negative")
	}
	slowCallDuration := config.SlowCallDuration
	if slowCallDuration == 0 {
		slowCallDuration = defaultSlowCallDuration
	}

	opening, err := normalizeOpening(config.Opening)
	if err != nil {
		return normalizedConfig{}, err
	}
	if timeWindow == nil {
		if opening.FailureCount > uint64(windowSize) { // #nosec G115 -- normalizeWindow establishes a positive bounded count size on this branch.
			return normalizedConfig{}, invalidConfig("Opening.FailureCount", "must not exceed count window size")
		}
		if opening.SlowCount > uint64(windowSize) { // #nosec G115 -- normalizeWindow establishes a positive bounded count size on this branch.
			return normalizedConfig{}, invalidConfig("Opening.SlowCount", "must not exceed count window size")
		}
	}
	openDuration, err := normalizeOpenDuration(config.OpenDuration)
	if err != nil {
		return normalizedConfig{}, err
	}
	halfOpen, err := normalizeHalfOpen(config.HalfOpen)
	if err != nil {
		return normalizedConfig{}, err
	}
	clock := config.Clock
	if clock == nil {
		clock = systemClock{}
	} else if nilValue(clock) {
		return normalizedConfig{}, invalidConfig("Clock", "must not be nil")
	}
	classifier := config.Classifier
	if classifier == nil {
		classifier = defaultClassifier
	}
	if config.PermitTTL < 0 {
		return normalizedConfig{}, invalidConfig("PermitTTL", "must not be negative")
	}
	permitTTL := config.PermitTTL
	if permitTTL == 0 {
		permitTTL = defaultPermitTTL
	}
	halfOpenWait, halfOpenMaxWait, err := normalizeHalfOpenAdmission(config.HalfOpenAdmission)
	if err != nil {
		return normalizedConfig{}, err
	}
	observer, err := normalizeObserver(config.Observer, config.EventDelivery)
	if err != nil {
		return normalizedConfig{}, err
	}
	if !finite(config.OpenDurationJitter) ||
		config.OpenDurationJitter < 0 || config.OpenDurationJitter >= 1 {
		return normalizedConfig{}, invalidConfig("OpenDurationJitter", "must be in the range [0, 1)")
	}
	random := config.Random
	if random == nil {
		random = standardRandom{}
	} else if nilValue(random) {
		return normalizedConfig{}, invalidConfig("Random", "must not be nil")
	}

	return normalizedConfig{
		name:               config.Name,
		countWindowSize:    windowSize,
		timeWindow:         timeWindow,
		minimumThroughput:  minimumThroughput,
		slowCallDuration:   slowCallDuration,
		opening:            opening,
		openDuration:       openDuration,
		halfOpen:           halfOpen,
		clock:              clock,
		classifier:         classifier,
		permitTTL:          permitTTL,
		halfOpenWait:       halfOpenWait,
		halfOpenMaxWait:    halfOpenMaxWait,
		observer:           observer,
		openDurationJitter: config.OpenDurationJitter,
		random:             random,
	}, nil
}

func normalizeObserver(observer Observer, delivery EventDeliveryPolicy) (observerRuntime, error) {
	if observer == nil {
		if delivery != nil {
			return observerRuntime{}, invalidConfig("EventDelivery", "requires an Observer")
		}
		return observerRuntime{}, nil
	}
	if delivery == nil {
		return observerRuntime{
			observer: observer,
			mode:     observerAsynchronous,
			buffer:   64,
			overflow: DropNewestEvent,
		}, nil
	}
	switch policy := delivery.(type) {
	case SynchronousEvents:
		return observerRuntime{observer: observer, mode: observerSynchronous}, nil
	case AsynchronousEvents:
		if policy.Buffer <= 0 {
			return observerRuntime{}, invalidConfig("EventDelivery.Buffer", "must be greater than zero")
		}
		if policy.Buffer > MaxEventBuffer {
			return observerRuntime{}, invalidConfig("EventDelivery.Buffer", "exceeds maximum")
		}
		if policy.Overflow != DropNewestEvent && policy.Overflow != DropOldestEvent {
			return observerRuntime{}, invalidConfig("EventDelivery.Overflow", "is unknown")
		}
		return observerRuntime{
			observer: observer,
			mode:     observerAsynchronous,
			buffer:   policy.Buffer,
			overflow: policy.Overflow,
		}, nil
	default:
		return observerRuntime{}, invalidConfig("EventDelivery", "unsupported delivery policy")
	}
}

func normalizeHalfOpenAdmission(config HalfOpenAdmissionPolicy) (bool, time.Duration, error) {
	switch policy := config.(type) {
	case nil, RejectExcessProbes:
		return false, 0, nil
	case WaitForProbe:
		if policy.MaxWait <= 0 {
			return false, 0, invalidConfig("HalfOpenAdmission.MaxWait", "must be greater than zero")
		}
		return true, policy.MaxWait, nil
	default:
		return false, 0, invalidConfig("HalfOpenAdmission", "unsupported admission policy")
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTimer(duration time.Duration) Timer {
	return systemTimer{Timer: time.NewTimer(duration)}
}

type systemTimer struct{ *time.Timer }

func (t systemTimer) C() <-chan time.Time { return t.Timer.C }

type standardRandom struct{}

func (standardRandom) Float64() float64 { return rand.Float64() } // #nosec G404 -- used only for optional scheduling jitter, never cryptographic material.

func defaultClassifier(completion Completion) Outcome {
	if completion.Err != nil {
		return OutcomeFailure
	}
	return OutcomeSuccess
}

func normalizeWindow(config WindowConfig) (int, *TimeWindow, error) {
	switch window := config.(type) {
	case nil:
		return defaultCountWindowSize, nil, nil
	case CountWindow:
		if window.Size <= 0 {
			return 0, nil, invalidConfig("Window.Size", "must be greater than zero")
		}
		if window.Size > windowpkg.MaxCountSize {
			return 0, nil, invalidConfig("Window.Size", "exceeds maximum")
		}
		return window.Size, nil, nil
	case TimeWindow:
		if window.BucketDuration <= 0 {
			return 0, nil, invalidConfig("Window.BucketDuration", "must be greater than zero")
		}
		if window.BucketCount <= 0 {
			return 0, nil, invalidConfig("Window.BucketCount", "must be greater than zero")
		}
		if window.BucketCount > windowpkg.MaxBucketCount {
			return 0, nil, invalidConfig("Window.BucketCount", "exceeds maximum")
		}
		if window.BucketDuration > time.Duration(1<<63-1)/time.Duration(window.BucketCount) {
			return 0, nil, invalidConfig("Window", "rolling interval overflows time.Duration")
		}
		return 0, &window, nil
	default:
		return 0, nil, invalidConfig("Window", "unsupported window configuration")
	}
}

func normalizeOpening(config *OpeningRules) (OpeningRules, error) {
	if config == nil {
		return OpeningRules{FailureRatio: 0.5, Combination: OpenWhenAny}, nil
	}
	rules := *config
	if rules.Combination != OpenWhenAny && rules.Combination != OpenWhenAll {
		return OpeningRules{}, invalidConfig("Opening.Combination", "is unknown")
	}
	if rules.IgnoredBehavior != PreserveConsecutiveFailures &&
		rules.IgnoredBehavior != ResetConsecutiveFailures {
		return OpeningRules{}, invalidConfig("Opening.IgnoredBehavior", "is unknown")
	}
	if err := validateRatio("Opening.FailureRatio", rules.FailureRatio); err != nil {
		return OpeningRules{}, err
	}
	if err := validateRatio("Opening.SlowRatio", rules.SlowRatio); err != nil {
		return OpeningRules{}, err
	}
	if rules.ConsecutiveFailures == 0 && rules.FailureCount == 0 &&
		rules.FailureRatio == 0 && rules.SlowCount == 0 && rules.SlowRatio == 0 {
		return OpeningRules{}, invalidConfig("Opening", "must enable at least one rule")
	}
	return rules, nil
}

func normalizeOpenDuration(config OpenDurationPolicy) (OpenDurationPolicy, error) {
	if config == nil {
		return FixedOpenDuration(defaultOpenDuration), nil
	}
	switch policy := config.(type) {
	case FixedOpenDuration:
		if policy <= 0 {
			return nil, invalidConfig("OpenDuration", "must be greater than zero")
		}
		return policy, nil
	case ExponentialOpenDuration:
		if policy.Initial <= 0 {
			return nil, invalidConfig("OpenDuration.Initial", "must be greater than zero")
		}
		if !finite(policy.Multiplier) || policy.Multiplier < 1 {
			return nil, invalidConfig("OpenDuration.Multiplier", "must be at least one")
		}
		if policy.Maximum < policy.Initial {
			return nil, invalidConfig("OpenDuration.Maximum", "must not be less than Initial")
		}
		return policy, nil
	default:
		return nil, invalidConfig("OpenDuration", "unsupported duration policy")
	}
}

func normalizeHalfOpen(config *HalfOpenPolicy) (HalfOpenPolicy, error) {
	if config == nil {
		return HalfOpenPolicy{
			MaxProbes:         defaultHalfOpenProbeCount,
			RequiredSuccesses: defaultHalfOpenProbeCount,
			FailureAction:     ReopenImmediately,
		}, nil
	}
	policy := *config
	if policy.MaxProbes < 1 {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen.MaxProbes", "must be greater than zero")
	}
	if policy.MaxProbes > MaxHalfOpenProbes {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen.MaxProbes", "exceeds maximum")
	}
	if policy.FailureAction != ReopenImmediately && policy.FailureAction != ReopenAfterSample {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen.FailureAction", "is unknown")
	}
	if err := validateRatio("HalfOpen.SuccessRatio", policy.SuccessRatio); err != nil {
		return HalfOpenPolicy{}, err
	}
	if policy.RequiredSuccesses > 0 && policy.SuccessRatio > 0 {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen", "must select one recovery threshold")
	}
	if policy.RequiredSuccesses <= 0 && policy.SuccessRatio == 0 {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen", "must select a recovery threshold")
	}
	if policy.RequiredSuccesses > policy.MaxProbes {
		return HalfOpenPolicy{}, invalidConfig("HalfOpen.RequiredSuccesses", "must not exceed MaxProbes")
	}
	return policy, nil
}

func validateRatio(field string, value float64) error {
	if !finite(value) {
		return invalidConfig(field, "must be in the range [0, 1]")
	}
	if value < 0 {
		return invalidConfig(field, "must be in the range [0, 1]")
	}
	if value > 1 {
		return invalidConfig(field, "must be in the range [0, 1]")
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nilValue(value any) bool {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func invalidConfig(field, message string) error {
	return &InvalidConfigError{Field: field, Message: message}
}
