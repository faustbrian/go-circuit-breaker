package breaker

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidConfig identifies invalid constructor configuration.
var ErrInvalidConfig = errors.New("breaker: invalid configuration")

var (
	// ErrOpen rejects normal admission before open interval eligibility.
	ErrOpen = errors.New("breaker: open")
	// ErrHalfOpenExhausted rejects when the classified/active probe quota is full.
	ErrHalfOpenExhausted = errors.New("breaker: half-open probes exhausted")
	// ErrHalfOpenWaitTimeout rejects when finite probe waiting expires.
	ErrHalfOpenWaitTimeout = errors.New("breaker: half-open wait timed out")
	// ErrPermitCompleted reports a repeated terminal action on a completed permit.
	ErrPermitCompleted = errors.New("breaker: permit already completed")
	// ErrPermitCanceled reports a terminal action on a canceled permit.
	ErrPermitCanceled = errors.New("breaker: permit canceled")
	// ErrPermitExpired reports an expired manual permit without recording it.
	ErrPermitExpired = errors.New("breaker: permit expired")
	// ErrForceOpen rejects admission in the administrative force-open mode.
	ErrForceOpen = errors.New("breaker: administratively force-open")
	// ErrIsolated rejects admission in administrative isolation.
	ErrIsolated = errors.New("breaker: administratively isolated")
	// ErrInvalidOutcome identifies an unsupported Outcome enum.
	ErrInvalidOutcome = errors.New("breaker: invalid outcome")
)

// InvalidConfigError identifies a configuration field that failed validation.
type InvalidConfigError struct {
	// Field identifies the invalid constructor option. Caller-created errors are
	// not sanitized; New uses package-owned field/reason text.
	Field string
	// Message describes the failed constraint, without retaining the Config value.
	Message string
}

// Error renders Field and Message unchanged with the stable category.
func (e *InvalidConfigError) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrInvalidConfig, e.Field, e.Message)
}

// Unwrap exposes ErrInvalidConfig for errors.Is.
func (e *InvalidConfigError) Unwrap() error { return ErrInvalidConfig }

// InvalidOutcomeError reports an unsupported outcome without consuming a permit.
type InvalidOutcomeError struct {
	// Outcome is the unsupported numeric classification supplied by the caller.
	Outcome Outcome
}

// Error reports the unsupported numeric Outcome and stable category.
func (e *InvalidOutcomeError) Error() string {
	return fmt.Sprintf("%v: %d", ErrInvalidOutcome, e.Outcome)
}

// Unwrap exposes ErrInvalidOutcome for errors.Is.
func (e *InvalidOutcomeError) Unwrap() error { return ErrInvalidOutcome }

// RejectionError carries admission metadata without retaining protected results,
// errors or contexts. Caller-configured Name and caller-created fields/causes are
// not a general redaction boundary; use errors.Is/As for operational decisions.
type RejectionError struct {
	// Name is retained caller-configured breaker identity, not operation data.
	// Error quotes it but does not redact secrets supplied as names.
	Name string
	// State is the policy state at rejection, independent of Mode.
	State State
	// Mode is the administrative mode at rejection.
	Mode Mode
	// Generation is the local policy/admin epoch at rejection, not a remote fence.
	Generation uint64
	// RetryAt is copied NextProbeAt; it is advisory, possibly zero/stale under
	// administrative modes, not a retry or later-admission guarantee.
	RetryAt time.Time
	// Cause is the errors.Is/As traversal target; library rejections use stable
	// sentinels. A caller-created cause may carry arbitrary diagnostic data.
	Cause error
}

// Error quotes the caller Name and renders state/mode, omitting Cause text.
func (e *RejectionError) Error() string {
	return fmt.Sprintf("breaker %q rejected execution in %s state (%s mode)", e.Name, e.State, e.Mode)
}

// Unwrap returns Cause without altering or redacting it.
func (e *RejectionError) Unwrap() error { return e.Cause }
