# Policies, truth tables, and classification

## Closed-state opening

Every enabled rule is evaluated only after classified outcomes reach minimum
throughput. An ignored outcome is not classified throughput.

| Rule | Opens when |
| --- | --- |
| Consecutive failures `N` | current failure streak is at least `N` |
| Failure count `N` | retained failures are at least `N` |
| Failure ratio `R` | retained failures / classified outcomes is at least `R` |
| Slow count `N` | retained slow successes plus slow failures are at least `N` |
| Slow ratio `R` | retained slow classified outcomes / classified outcomes is at least `R` |

`OpenWhenAny` opens if any enabled row is true. `OpenWhenAll` opens only if all
enabled rows are true. Rule order does not affect the result; the transition
reason is the stable `policy-opened` reason.

## Consecutive outcomes

| Outcome | Default streak effect | Reset option |
| --- | --- | --- |
| Success | reset | reset |
| Failure | increment | increment |
| Ignored | preserve | reset |
| Slow success | reset | reset |
| Slow failure | increment | increment |

Slow is orthogonal to success/failure. Slow ignored outcomes are never counted
as slow dependency calls.

## Half-open recovery

| Threshold | Success | Failure |
| --- | --- | --- |
| Required successes | close when reached | immediate or after sample |
| Success ratio | evaluate after `MaxProbes` classified completions | immediate or evaluate after sample |
| Ignored | release active slot; do not advance sample | same |

At most `MaxProbes` are active or classified in one half-open generation.
Canceled/expired probes release active capacity and do not advance the sample.

## Classification guide

Classify dependency health, not merely whether the caller obtained its desired
result. Recommended starting points:

| Completion | Typical outcome |
| --- | --- |
| Successful dependency response | success |
| Dependency transport/server failure | failure |
| Caller canceled before admission | no permit and no outcome |
| Caller cancellation after admission | explicit caller policy |
| Local validation/cache hit/rate limit/bulkhead rejection | ignored/no admission |
| HTTP 4xx | protocol/business policy, often ignored or success |
| HTTP 5xx | protocol policy, often failure |
| Queue locally full | ignored |
| Queue broker unavailable | failure |

Use `errors.Is` for wrapped and joined sentinel errors. Be deliberate with typed
nil errors: Go interfaces containing a typed nil are non-nil and therefore fail
under the default classifier. Never retain `Completion.Result` or
`Completion.Err` in a classifier.

## Count versus time windows

Use a count window when request volume is stable and “last N calls” is the right
signal. Use a time window when low/high traffic periods should represent the
same wall-time horizon. Count memory is `O(Size)`; time memory is
`O(BucketCount)`, independent of request volume and process lifetime.

## Timing

The system clock preserves Go's monotonic component during in-process elapsed
time. Serialized timestamps are wall-clock observations and should not be used
to reconstruct duration. A backward injected-clock movement clamps execution
elapsed time to zero. Jitter only shortens the selected open interval and never
exceeds the configured schedule.
