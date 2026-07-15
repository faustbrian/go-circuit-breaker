# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial breaker state, administrative mode, and outcome contracts.
- Validated construction with safe, finite count-window defaults.
- Typed invalid-configuration errors with `errors.Is` and `errors.As` support.
- Bounded count- and time-based rolling outcome windows with deterministic
  expiry and backward-clock handling.
- Validated opening-rule composition, fixed and exponential open schedules,
  and bounded half-open recovery policies.
- Deterministic opening decisions for consecutive failures, failure counts and
  ratios, slow-call counts and ratios, and any/all rule composition.
- Generation-bound two-step admission with typed open and half-open rejection,
  bounded probes, exactly-once completion, and stale-completion isolation.
- Immutable concurrent snapshots with window, transition, admission, rejection,
  ratio, and open-timing aggregates.
- Generic context-aware execution with typed result and original error
  preservation, caller-defined classification, slow-call observation, and
  failure recording before operation or classifier panics are re-panicked.
- Explicit force-open, disabled, isolated, release, and reset controls that
  invalidate older generations without overloading policy-driven state.
- Context-independent permit cancellation and bounded expiry that reclaim
  half-open capacity without per-permit goroutines or timers.
- Count windows preserve their classified sample when ignored outcomes are
  observed, preventing local or caller-owned outcomes from displacing the
  dependency-health signal.
- Optional context-aware half-open waiting with a finite maximum wait and
  wakeups on probe completion, cancellation, expiry, state transition, and
  administrative mode changes.
- Immutable transition events with policy and administrative reasons, exact
  before/after snapshots, synchronous delivery outside internal locks, and
  bounded asynchronous delivery with explicit overflow behavior.
- Observer panics and returned errors are isolated from breaker results and
  exposed as bounded aggregate counters alongside dropped-event metrics.
- Construction rejects non-finite thresholds and multipliers, overflowing time
  intervals, and allocation-sized count windows, time buckets, probe sets, and
  asynchronous event queues.
- Half-open admission now bounds the complete classified recovery sample rather
  than only simultaneous probe concurrency; ignored probes remain replaceable.
- Consecutive-failure rules expose validated ignored-outcome behavior, preserving
  the streak by default or resetting it when selected explicitly.
- Snapshots expose configured window capacity and minimum throughput, current
  sample size, ratio definedness, and half-open completion progress.
- Unsupported permit outcomes return a stable typed error without consuming the
  permit or mutating breaker state.
- The clock contract now owns timers as well as timestamps, making half-open
  maximum waits deterministic in tests and rejecting typed-nil clock values at
  construction.
- Fixed and exponential open schedules support bounded downward jitter through
  caller-supplied deterministic randomness; invalid samples safely fall back to
  the unjittered duration, and recovery resets exponential escalation.
- `breakertest.Clock` provides concurrency-safe manual time movement, ordered
  deterministic timers, backward clock jumps, timer cancellation, and active
  timer accounting for leak assertions.
- `breakertest.Recorder` and `breakertest.ScriptedClassifier` provide bounded,
  concurrency-safe transition capture and deterministic classification without
  retaining operation results or errors.
- Contention tests prove single-generation opening, strict half-open bounds,
  exactly-once concurrent completion, and snapshot consistency during resets
  and administrative transitions under the race detector.
- A randomized reference state machine, breaker-level time-window expiry tests,
  and half-open recovery truth tables cover generation, ratio, sample, reset,
  and idle-time behavior across composed policies.
- Fuzz targets cover hostile configuration, arbitrary permit and administrative
  sequences, count-window reference parity, and time-window timestamp movement.
- Leak checks cover canceled admission timers and asynchronous observer draining;
  benchmarks cover closed execution, rejection, snapshots, half-open contention,
  observers, and count/time window rollover.
- Boundary and internal-invariant tests bring the root, `window`, and
  `breakertest` packages to meaningful 100% production statement coverage.
- Asynchronous observer enqueue and shutdown are serialized independently of
  breaker state, preventing events from being queued after the worker drains.
- Integration suites demonstrate caller-owned HTTP response bodies, exact
  operation-error preservation, and protocol-owned database and queue outcome
  classification without adding protocol dependencies to core.
- Reproducible format, vet, lint, test, coverage, race, fuzz, leak, benchmark,
  documentation, compatibility, and GO-SAFETY-1 targets now match pinned CI,
  vulnerability scanning, dependency review, and provenance-enabled releases.
- Public documentation now covers quick adoption, every configuration surface,
  policy truth tables, permits, composition, observability, operations,
  incidents, migration, architecture, threat findings, and release evidence;
  runnable examples cover execution, permits, windows, policies, observers,
  and administrative control.
- Fuzz evidence now includes force-open, isolation, snapshot, observer panic,
  observer failure, and reentrant observer sequences; reproducible CPU, memory,
  and mutex profile targets support contention and allocation audits.
- Profile test binaries are written alongside their temporary profile artifacts
  instead of leaking into the repository working tree.
