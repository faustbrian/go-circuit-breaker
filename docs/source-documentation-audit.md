# Source documentation audit

## Scope and reproduction

Tier A source audit at exact fetched remote-main base
`763b75cc0a75136afd9d0fd891073356f03cd9ee` in the selected detached worktree.
Reproduce with `git ls-files '*.go'`, modules.json/packages.json and
`golib source-docs inventory --json`. All 54 tracked Go sources: 13 production,
41 assurance; root public module `github.com/faustbrian/go-circuit-breaker`
(root breaker, window, breakertest) and non-releasable nested
`integration/consumers` (test-only published-version consumer harness). The root
integration directory is also a test-only package, not a transport adapter.

Before source editing, objective inventory reports 148 missing enum, field,
interface-method and error documentation sites. All three public package
comments exist but require useful lifecycle/ownership boundaries. Source
investigation is separate from missing-comment repair: classified sample
capacity versus active permits, TTL maintenance versus operation completion,
stale/disabled lifetime accounting, observer shutdown/order, unvalidated caller
name/error metadata, count ignored totals, and full-range timestamp arithmetic.
No source policy or runtime algorithm is replaced by this pass.

Outcome/owner: truthful source contracts for root admission/Execute users and
standalone window/test-support consumers. Named risks: stale permits corrupting
new generations, abandoned probe capacity, callback reentrancy or blocking,
observer lifetime, copied versus borrowed configuration, sampling denominator,
float duration/ratio boundaries and resource admission. Select native format,
all-file AST/non-comment-token/directive equivalence, three rendered packages,
native docs/examples/links, current source-doc checks, changed-content secrets,
markers and ordinary complete final-diff review. API mode is apidiff in both
.golib.yaml and native compatibility script; no prose baseline regeneration.
No broad unchanged runtime/race/leak/fuzz/performance/consumer/release campaign.
Go checks require one serialized session with three exact owned caches.

## Investigated invariants and contracts

- Breaker mutex owns admission, state/mode/generation, permit status, windows and
  counters. Collaborators (Clock/Timer/Random/Classifier/operation/Observer) run
  outside it and are borrowed concurrency/lifetime responsibilities. Snapshot
  fields are copied values, not handles to state; atomic observer counters can
  advance independently of the state snapshot. Breaker/Permit must not be copied.
- New copies Opening/HalfOpen value policies and normalizes sealed value policy
  types; pointers to unsupported implementations are not alternate providers.
  Name is nonempty <=256 bytes but is not a sanitization/authentication boundary.
  Lifetime counters/generation use ordinary uint64 arithmetic, not a saturation
  or globally unique/distributed fencing guarantee. Allocation maxima do not cap
  total callers, process heap, callback work or returned protocol resources.
- MinimumThroughput gates every closed opening rule on classified success/failure
  count, even consecutive rules. Ignored does not advance classified throughput
  or slow dimensions; it preserves/resets the consecutive streak by policy.
  Count windows retain classified outcomes only and accumulate ignored totals;
  time-window ignored outcomes expire in their bucket. Ratio comparisons use
  float64 conversion/division, not exact rational arithmetic.
- Open eligibility is lazy: admission at/after NextProbeAt transitions to half-open,
  not a background timer or Snapshot alone. MaxProbes bounds classified completed
  plus active probes in one generation. Ignored/canceled/expired probes release
  active slots without consuming classified quota. RequiredSuccesses may close
  early; SuccessRatio is checked at a full classified sample. Half-open outcomes
  affect its recovery counters, not the retained prior closed-state health window.
- Complete terminal status is exactly once under the mutex. Stale/disabled permits
  can still complete into lifetime counts but cannot mutate current health.
  Invalid outcomes do not consume a manual permit. Cancel never cancels protected
  work or records an outcome; it only releases bookkeeping capacity. Manual TTL
  expiry is inclusive and lazy on completion/cancel or half-open maintenance.
- Execute borrows ctx/operation, runs synchronously and does not enforce a timeout,
  retry or fallback. Classifier observes ephemeral Completion only. The original
  result is preserved; nonnil operation error takes precedence over a completion
  error. Panics in operation/classifier record failure then re-panic; clock panic
  cleanup avoids corrupting permits. Late Execute completion is lifetime-counted
  even if lazy half-open maintenance expired its permit; current health changes
  only if the permit remains active/current. Context cancellation is not inherently
  ignored by the default nonnil-error classifier.
- Mode changes retain policy state/window but advance generation and reset the
  half-open sample. Same-mode changes are no-ops. Disabled bypasses health recording
  but still counts successful terminal outcomes in lifetime totals. Reset starts
  normal/closed with fresh health window/open schedule while retaining lifetime
  totals. Release resumes retained policy, not a reset.
- Fixed/exponential schedules are positive; exponential float multiplication is
  duration-truncated and capped, with nonincreasing/overflow-like conversion
  falling back to maximum. Downward jitter uses one borrowed sample outside the
  lock, clamps negatives to zero and invalid/high/panicking samples to zero;
  duration is floored to one nanosecond. Samples can be requested on completions
  that do not ultimately open the breaker; no cryptographic randomness is claimed.
- Admission waits only for half-open quota, with ctx and one finite timer per
  waiting caller stopped on return. A mutex-owned close/replace notification
  broadcasts changes; wait deadlines survive retries. Timer/provider callbacks
  are outside the state mutex. Clock/timer providers must implement coherent,
  safe shared time; no callback deadline or panic recovery contract is imposed
  universally. Nil contexts/zero Breaker/Permit are not generally usable.
- Events snapshot transitions under the state lock but dispatch after unlocking.
  Different callers may reorder delivery relative to generation order. Synchronous
  callbacks may overlap/reenter and block their transitioning caller. Async mode
  owns one worker/bounded queue, can drop, isolates observer errors/panics into
  counters and retains no error text. eventMu serializes producers/Close; queue
  is not closed while producers could send. Close stops async acceptance without
  waiting and does not disable breaker or synchronous observers. Shutdown drains
  accepted async callbacks or returns ctx error, never interrupting a blocked
  callback; calling it from that worker can self-wait.
- Count ring insertion/eviction adjusts classified/slow dimensions in constant
  work; ignored bypasses ring. Time observes mathematical floor bucket identity,
  not truncation toward zero before epoch. Wide sign/magnitude arithmetic avoids
  UnixNano aliasing outside its int64 range; Snapshot advances observed bucket
  monotonically, clamps backward additions and scans fixed bucket capacity rather
  than every skipped interval. Windows themselves require external serialization.
- Test Clock is mutex-safe and manually driven: no timer goroutine; Stop removes
  retained pending timers, notifications carry scheduled deadlines and channels
  stay open. Advance is separate Now+Set operations, not atomic cumulative addition
  across concurrent calls; negative movement is allowed. Recorder copies events
  in delivery order, evicts oldest at caller-sized capacity without a hard maximum.
  ScriptedClassifier copies outcomes and intentionally does not validate enums.

## Variants, generation and assurance

No tracked generated Go source, regeneration directive, platform suffix or build
constraint exists. Existing narrow integer/random security suppressions are
preserved exactly; no unfinished or deprecated source marker was found. Seven
fuzz targets cover configuration, operation/observer sequences, resource bounds,
execution outcome/duration and window reference arithmetic. Nine benchmarks
measure closed/open/snapshot/probe/observer and count/time window shapes; their
actual setup/timer boundaries are recorded adjacent to each benchmark, without
new latency or memory promises. Twenty-five examples cover root, window and
manual support workflows. Test-only models and arbitrary-precision bucket oracles
are owned fixtures, not external protocol conformance corpora or generated code.
Nested consumers pin released breaker v1.0.0/JSON-RPC v1.1.1 without local replace;
that unchanged harness is not proof of this worktree's documentation source.
Repository design/policy tables provide maintained semantic authority, not a
pinned external circuit-breaker standard. Archived assurance remains immutable.

## Tracked Go coverage

| File | Audited boundary |
| --- | --- |
| `admin_permit_test.go` | TestPermitCancellationReleasesHalfOpenCapacity, TestAbandonedPermitExpiresAndReleasesHalfOpenCapacity and 4 related contract partitions |
| `benchmark_test.go` | BenchmarkClosedExecute, BenchmarkOpenRejection and 4 related contract partitions |
| `boundary_behavior_test.go` | TestPermitTerminalStateErrors, TestDefaultClassifierAndBackwardElapsedTime and 6 related contract partitions |
| `breaker.go` | Generation/state linearization, permits, lazy expiry, half-open quota and snapshots |
| `breakertest/clock.go` | Manual clock/timer lifetime, retained references and compound Advance behavior |
| `breakertest/clock_internal_test.go` | TestStoppedTimersAreNotRetained |
| `breakertest/clock_test.go` | TestClockAdvancesAndFiresTimersDeterministically, TestClockStopAndBackwardSetDoNotFireTimer and 2 related contract partitions |
| `breakertest/example_test.go` | ExampleClock, ExampleRecorder and 1 related contract partitions |
| `breakertest/support.go` | Copied event/script storage, delivery order and capacity/enum seams |
| `breakertest/support_test.go` | TestRecorderRetainsBoundedEventCopies, TestNewRecorderRejectsNonPositiveCapacity and 1 related contract partitions |
| `classification_contract_test.go` | TestDefaultClassifierErrorMatrix, TestCallerClassifierOwnsCancellationPolicy and 3 related contract partitions |
| `collaborator_reentrancy_test.go` | TestClockNowCanReenterBreaker, TestClockPanicDoesNotCorruptBreaker and 7 related contract partitions |
| `concurrency_test.go` | TestOpeningUnderContentionCreatesOneGenerationAndEvent, TestHalfOpenAdmissionBoundUnderContention and 3 related contract partitions |
| `config.go` | Validated allocation bounds, copied policies and borrowed collaborator contracts |
| `config_bounds_test.go` | TestNewRejectsNonFinitePolicyValues, TestNewRejectsAllocationSizedConfiguration and 5 related contract partitions |
| `config_policy_test.go` | TestNewAcceptsCompletePolicyConfiguration, TestNewRejectsInvalidPolicyCombinations |
| `config_test.go` | TestNewAppliesFiniteDefaults, TestNewRejectsInvalidConfiguration and 1 related contract partitions |
| `doc.go` | Protocol-neutral owner and caller-owned cancellation/resources |
| `duration_policy_test.go` | TestOpenDurationAppliesBoundedDeterministicJitter, TestExponentialOpenDurationCapsAndResetsAfterRecovery and 3 related contract partitions |
| `error_contract_test.go` | TestEveryAdmissionRejectionSupportsErrorsIsAndAs |
| `errors.go` | Stable typed categories, rejection/config diagnostic ownership |
| `example_test.go` | ExampleExecute, ExampleBreaker_Acquire and 18 related contract partitions |
| `execute.go` | Synchronous generic operation/classification, error precedence and panic cleanup |
| `execute_test.go` | TestExecutePreservesTypedResultAndOriginalError, TestExecuteDoesNotInvokeRejectedOperation and 9 related contract partitions |
| `fuzz_test.go` | FuzzConfigurationNeverPanics, FuzzPermitOperationSequences and 3 related contract partitions |
| `half_open_table_test.go` | TestHalfOpenRecoveryTruthTable |
| `integration/consumers/database_sql_test.go` | TestDatabaseSQLDependencyFailureAndRowsOwnership |
| `integration/consumers/jsonrpc_test.go` | TestJSONRPCClientSeparatesLocalValidationAndTransportFailure |
| `integration/http_composition_test.go` | TestHTTPCompositionRecordsOneLogicalOutcomeAcrossRetries, TestHTTPLocalShortCircuitsBypassBreakerAdmission and 2 related contract partitions |
| `integration/integration_test.go` | TestHTTPLogicalOperationClassificationAndBodyOwnership, TestDatabaseCallPreservesDriverError and 1 related contract partitions |
| `integration/non_http_contract_test.go` | TestNonHTTPProtocolPoliciesRemainCallerOwned |
| `internal_invariant_test.go` | TestPermitInternalLifecycleGuardsAreIdempotent, TestInternalValidatedPolicyDefaultsAreUnreachable and 5 related contract partitions |
| `leak_test.go` | TestCanceledHalfOpenWaitReleasesClockTimer, TestCloseDrainsObserverQueueAndDropsLaterEvents |
| `model_test.go` | TestRandomizedStateMachineMatchesReferenceModel |
| `observer.go` | Unlocked callbacks, bounded drop queue and separate shutdown ownership |
| `observer_test.go` | TestSynchronousObserverReceivesImmutableTransitionsOutsideLock, TestObserverPanicAndFailureDoNotCorruptBreaker and 6 related contract partitions |
| `policy.go` | Minimum classified throughput, enabled-rule combinations and float ratios |
| `policy_test.go` | TestOpeningDecisionTruthTable |
| `snapshot_error_test.go` | TestSnapshotExposesConfiguredBoundsAndRatioDefinedness, TestSnapshotExposesHalfOpenRecoveryProgress and 1 related contract partitions |
| `state_machine_test.go` | TestConsecutiveFailuresOpenAndReject, TestOpenEligibilityTransitionsOnceAndBoundsHalfOpenProbes and 4 related contract partitions |
| `time_window_integration_test.go` | TestBreakerTimeWindowExpiresOutcomesBeforeOpeningDecision, TestBreakerTimeWindowOpensAtExactBucketedThreshold |
| `types.go` | Policy states, administrative modes and classified/ignored enum contracts |
| `wait_test.go` | TestHalfOpenWaiterAcquiresReleasedCapacity, TestHalfOpenWaitHonorsCallerCancellation and 4 related contract partitions |
| `window/benchmark_test.go` | BenchmarkCountRollover, BenchmarkTimeRollover and 1 related contract partitions |
| `window/bounds_test.go` | TestWindowConstructorsRejectOversizedAllocation, TestNewTimeRejectsOverflowingInterval |
| `window/count.go` | Fixed classified ring versus cumulative ignored accounting |
| `window/count_test.go` | TestCountRetainsOnlyMostRecentClassifiedOutcomes, TestCountIgnoredOutcomesDoNotEvictClassifiedOutcomes and 4 related contract partitions |
| `window/example_test.go` | ExampleCount, ExampleTime |
| `window/fuzz_test.go` | FuzzCountMatchesBoundedReference, FuzzTimeWindowTimestamps |
| `window/internal_invariant_test.go` | TestConstructorsAcceptExactResourceBounds, TestBucketIDMatchesArbitraryPrecisionFloorDivision and 4 related contract partitions |
| `window/time.go` | Wide signed floor buckets, monotone observation and inclusive horizon |
| `window/time_internal_test.go` | TestBucketIDFloorDivision, TestBucketIDPreservesWideTimestampOrdering and 2 related contract partitions |
| `window/time_test.go` | TestTimeMatchesReferenceAcrossLongRandomizedSequences, TestTimeExpiresBucketsAtExactBoundary and 10 related contract partitions |
| `window/window.go` | Classified versus ignored/slow aggregate dimensions |

## Disposition

All 54 tracked Go sources, 235 public API members (types/constants/functions,
methods, named fields and interface methods), four exported private-adapter
methods and private coordination boundaries are audited. Initial 148 structural
documentation gaps are resolved. Go 1.27.2 native formatting passes all sources;
parsed AST and compiler/security/analyzer directives match the exact base.
Non-comment token comparison normalizes only optional implicit semicolons before
closing braces: four appear when inline public structs/interface expand for
field/method comments. Every other executable token is identical.
All three public go-doc outputs render the clarified contracts. The native docs
target passes required files/links/package docs and all 25 examples; the root
integration test package has no examples. Other runtime/consumer campaigns are
not rerun. Runtime behavior, API apidiff baseline bytes, dependency pins and
historical evidence remain unchanged. No unresolved policy rationale
question has been identified; concrete method-specific zero/nil and accounting
caveats are documented rather than repaired. Hosted CI/main delivery is separate
and coordinator-owned.

## Objective Enforcement

The checksum-pinned public Tools v2.4.0 verifier now supplies
`make source-docs-check` as a prerequisite of the existing local and CI Make
routes. Ordinary shared CI selects strict source-documentation checking and
requires its result. Native prerequisites, operation budgets, security and
release controls are retained. Hosted exact-head evidence is recorded by the
coordination ledger; configuration alone does not establish green delivery.
