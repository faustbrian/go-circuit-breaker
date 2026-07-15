# State-machine specification and threat model

## Transition table

| From | Condition | To | Window | Generation |
| --- | --- | --- | --- | --- |
| closed | opening policy true | open | retained for diagnosis | increment |
| open | interval elapsed during admission | half-open | retained | increment |
| half-open | recovery threshold true | closed | fresh empty window | increment |
| half-open | recovery failure true | open | retained until recovery | increment |
| any | reset | closed/normal | fresh empty window | increment |

Administrative mode changes do not rewrite policy state, but increment the
generation and invalidate permits. Force-open and isolated reject; disabled
admits without recording; normal restores policy admission. Impossible enum
values are rejected.

## Linearization points

All state-machine linearization occurs while `Breaker.mu` is held:

| Operation | Linearization point |
| --- | --- |
| Admit/reject | permit creation or rejection-counter increment |
| Open/half-open/close | state and generation assignment in transition |
| Permit complete/cancel/expire | permit terminal status assignment |
| Administrative mode | mode and generation assignment |
| Reset | closed transition and new-window assignment |
| Snapshot | aggregate capture after current expiry reclamation |

The protected operation and classifier run without the lock. Transition events
are built from before/after snapshots while locked and delivered after unlock.
Async enqueue/close serialization uses a separate event lock. Snapshot values
are copied, so callers never observe mutable internal structures.

## Resource ownership and complexity

- Count windows allocate at most `MaxCountSize` records; time windows allocate
  at most `MaxBucketCount` aggregates.
- Half-open retained permits are bounded by `MaxHalfOpenProbes`.
- Async observer queues are bounded by `MaxEventBuffer` and one worker.
- No operation result or error is retained. There is no per-call goroutine,
  timer, history, finalizer, cgo, or unsafe code in production.
- Waiting admission owns one finite timer per waiting caller and stops it on
  every return path. Core otherwise has no permanent goroutine.

## Threat model and findings

| Threat | Impact | Disposition/evidence |
| --- | --- | --- |
| Threshold crossing contention | duplicate transitions/probes | serialized transition; high-contention and race tests |
| Stale/duplicate completion | corrupt new generation | generation binding and terminal permit status tests |
| Abandoned probe | exhausted half-open capacity | finite TTL, cancellation, lazy expiry, leak tests |
| Dependency latency collapse | caller pile-up | slow-call rules; timeout remains caller responsibility |
| Retry storm | amplified failure | documented ordering and one logical recording |
| Clock jump/scheduler pause | early/late recovery or stale buckets | deterministic clock, boundary, idle-gap, backward-jump tests |
| Callback/classifier/observer panic | deadlock/corruption | callbacks outside lock; panic/reentrancy tests; observer isolation |
| Telemetry overload | admission latency/memory growth | bounded queue/drop policy; no rejection events |
| Secret/result disclosure | sensitive diagnostics | aggregate-only snapshots/events/errors |
| Operator race | contradictory state | generation invalidation and race tests |
| Coordinated replica probes | recovery surge | caller-configured bounded downward jitter |
| Allocation denial of service | memory exhaustion | validated hard maxima for windows/probes/events |

No high- or medium-severity finding remains open. Operational risks that cannot
be solved by core—incorrect classification, excessive caller retries, failure
to close protocol resources, and abandoned permits until TTL—are explicit
caller responsibilities.

## State invariants

- Generation is nonzero and increments exactly once per committed transition.
- Active half-open probes are never negative or above `MaxProbes`.
- A current permit is recorded at most once; a stale permit records nothing.
- Classified count equals successes plus failures; ignored is separate.
- Slow successes/failures are subsets of their corresponding outcome counts.
- Open timing is populated only after policy opening and reset on recovery.
- Observer code never runs under the state lock.
