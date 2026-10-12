// Package breaker provides a protocol-neutral, bounded circuit breaker.
//
// Breakers own dependency-health admission, rolling outcome state, half-open
// probes, administrative modes, immutable snapshots, and transition events.
// Callers retain ownership of operation timeouts, retries, fallbacks, protocol
// classification, request and response resources, and protected results and
// errors. Construct with New; use Execute for synchronous classified calls or
// Acquire with exactly one Complete/Cancel for caller-owned two-step work.
// A breaker protects one process-local health boundary, not a bulkhead, rate
// limiter, distributed coordinator or transport adapter.
//
// Breaker and Permit state is synchronized and must not be copied. Collaborators
// are borrowed and run outside the state mutex; callers provide their lifetime,
// concurrency and blocking/panic policy. A configured asynchronous observer owns
// one bounded worker: Close requests its stop and Shutdown waits. Neither stops
// protected work or disables admission. Other modes start no background worker.
// Production is pure Go with no infrastructure/cgo dependency; window exposes
// externally synchronized rings and breakertest supplies manually driven seams.
package breaker
