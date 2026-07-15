# API and configuration reference

`New(Config)` validates the entire configuration before traffic is admitted and
copies all value configuration into immutable internal state. A classifier,
clock, random source, and observer are caller-owned functions/interfaces and
must remain concurrency-safe.

## Defaults

Only `Name` is required.

| Setting | Default |
| --- | --- |
| Window | last 100 classified outcomes |
| Minimum throughput | 10 |
| Opening rule | failure ratio at least 0.5 |
| Slow-call duration | 30 seconds |
| Open duration | fixed 30 seconds |
| Half-open | 10 probes, all 10 successes, reopen immediately |
| Permit TTL | 5 minutes |
| Excess half-open admission | reject immediately |
| Classifier | nil error succeeds; non-nil error fails |
| Observer delivery | bounded async buffer 64, drop newest |
| Jitter | none |

Supplying an observer without `EventDelivery` selects the observer default.
Without an observer, no worker is created. `SynchronousEvents` explicitly runs
the observer in the transitioning caller after the state lock is released.

## Windows and opening rules

`CountWindow{Size}` retains the newest classified outcomes. Ignored outcomes
increment the ignored diagnostic count but do not evict health samples.

`TimeWindow{BucketDuration, BucketCount}` retains bounded bucket aggregates for
`BucketDuration * BucketCount`. Idle gaps and backward clock movement are
handled deterministically.

`OpeningRules` enables any combination of consecutive failures, failure count,
failure ratio, slow count, and slow ratio. Zero disables a rule. `OpenWhenAny`
or `OpenWhenAll` composes enabled rules. Every rule waits for
`MinimumThroughput`; ratio comparisons are inclusive.

## Open and half-open policy

`FixedOpenDuration` uses one interval. `ExponentialOpenDuration` multiplies the
interval after each failed recovery, caps it at `Maximum`, and resets escalation
after recovery. `OpenDurationJitter` is a downward fraction in `[0,1)` and uses
the configured `Random` source.

`HalfOpenPolicy` limits the complete recovery sample with `MaxProbes`. Select
exactly one of `RequiredSuccesses` or `SuccessRatio`. `ReopenImmediately` reacts
to the first classified failure; `ReopenAfterSample` waits for the bounded
sample. Ignored probes release active capacity and may be replaced.

`RejectExcessProbes` fails fast. `WaitForProbe{MaxWait}` waits for capacity or a
state change until the caller context or maximum wait ends. Waiting never
consumes a permit.

## Execution and permits

`Execute[T]` calls `Acquire`, times the operation with the configured clock,
classifies the result, and completes the permit. Rejection does not call the
operation. Operation errors are returned unchanged. An operation or classifier
panic is recorded as a failure and re-panicked with the same value.

`Acquire` returns a generation-bound `Permit`. Call exactly one of:

- `Complete(outcome, slow)` for finished caller-owned work;
- `Cancel()` for work that will not complete.

Permit completion is duplicate-safe. Expired, canceled, and already-completed
permits return stable sentinel errors. A stale permit becomes a no-op against a
new generation. Permit expiry is reclaimed on permit use, admission, or
snapshot; core does not run a background reaper.

## Administrative control and lifecycle

`ForceOpen` and `Isolate` reject. `Disable` admits without recording. `Release`
returns to normal policy operation. `Reset` creates a new closed generation,
normal mode, and empty window. `SetMode` is the common typed API.

`Close` only drains/stops an asynchronous observer worker. It is idempotent and
does not close admission. A breaker without async observation needs no close.

## Snapshots, events, and errors

`Snapshot` contains state, mode, generation, transition timing, bounded window
aggregates, admissions, rejections, half-open progress, ratio definedness, open
timing, observer failures, and dropped events. It contains no result or error.

Use `errors.Is` with `ErrOpen`, `ErrHalfOpenExhausted`,
`ErrHalfOpenWaitTimeout`, `ErrForceOpen`, `ErrIsolated`, permit sentinels,
`ErrInvalidOutcome`, and `ErrInvalidConfig`. Use `errors.As` for
`RejectionError`, `InvalidConfigError`, or `InvalidOutcomeError`. A rejection
contains only breaker identity, state, mode, generation, and retry time.

The `window` package exposes bounded `Count` and `Time` structures. The
`breakertest` package exposes a deterministic clock/timer, bounded transition
recorder, and scripted classifier.
