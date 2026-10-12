package breaker

// State is the policy-driven state of a circuit breaker.
type State uint8

const (
	// StateClosed admits normal work and evaluates classified rolling outcomes.
	StateClosed State = iota
	// StateOpen rejects normal admission until its open interval is eligible.
	StateOpen
	// StateHalfOpen admits a bounded classified recovery sample and active probes.
	StateHalfOpen
)

// String returns a stable state label, or "unknown" for an unsupported value.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Mode is an explicit administrative override.
type Mode uint8

const (
	// ModeNormal applies the current policy state without an override.
	ModeNormal Mode = iota
	// ModeForceOpen rejects work without changing policy state or open timing.
	ModeForceOpen
	// ModeDisabled admits without health-policy recording; lifetime totals still count.
	ModeDisabled
	// ModeIsolated rejects work for explicit operator-controlled maintenance.
	ModeIsolated
)

// String returns a stable mode label, or "unknown" for an unsupported value.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeForceOpen:
		return "force-open"
	case ModeDisabled:
		return "disabled"
	case ModeIsolated:
		return "isolated"
	default:
		return "unknown"
	}
}

// Outcome is the mutually exclusive classification of one completion.
type Outcome uint8

const (
	// OutcomeSuccess contributes success, classified throughput and optional slow data.
	OutcomeSuccess Outcome = iota
	// OutcomeFailure contributes failure, classified throughput and optional slow data.
	OutcomeFailure
	// OutcomeIgnored contributes no classified/slow sample, but counts in lifetime totals.
	OutcomeIgnored
)

// String returns a stable outcome label, or "unknown" for an unsupported value.
func (o Outcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeFailure:
		return "failure"
	case OutcomeIgnored:
		return "ignored"
	default:
		return "unknown"
	}
}
