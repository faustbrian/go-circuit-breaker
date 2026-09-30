# Security model v1

Model version: 1. Reviewed on 2026-09-30 against source
`a3fbf2ba4f3a758e49baf2fe3ec9b40773ff5ffd`, Go 1.27, latest public release
`v1.0.0`. This documents existing behavior, not future source or the security
of every consuming application.

## Scope and trust boundaries

The root module owns the breaker, public bounded `window` data structures,
and `breakertest` support. `integration/consumers` is a non-releasable test
module. Root production has no third-party dependency or implicit network,
filesystem, process, environment, credential, authentication, authorization,
or cryptographic behavior. Protocol resources and policy remain caller-owned.

Untrusted requests may influence admission volume, context cancellation,
operation outcomes, and operation duration. Breaker construction, names,
thresholds, administrative modes, classifiers, observers, clocks, and random
sources are trusted application inputs. Do not expose construction, Reset,
Disable, or other administrative control directly to unauthorized callers.

Assets are correct dependency-health decisions, bounded retained window and
observer data, half-open sample accounting, and non-secret diagnostics. This
is a process-local health/admission policy, not a concurrency limiter, global
registry, distributed coordination system, retry engine, or deadline enforcer.

Direct owned consumers declared in `modules.json` include bulkhead and
fault-injection resilience tests, HTTP client, HTTP middleware sibling tests,
localized, service adoption/reference tests, and the nested consumer harness.
They use public admission/classification contracts; documentation-only changes
do not require source-lock updates or consumer releases.

## Security matrix

| Boundary or attack | Control and limitation | Existing evidence |
| --- | --- | --- |
| Oversized allocations and malformed policy | Fixed count/time windows, capped probe/event counts; reject non-finite thresholds and overflowing intervals before allocation | `config_bounds_test.go`, `window/bounds_test.go`, `FuzzConfigurationResourceBounds` |
| Diagnostic exposure | No operation result/error retained in windows, snapshots, or events; names bounded to 256 bytes but not sanitized for secrecy or control characters | `snapshot_error_test.go`, `config_bounds_test.go`, `observer.go` |
| Duplicate/stale completion and administrative races | One terminal permit action; generation fence prevents stale results altering replacement health state | `TestConcurrentDuplicateCompletionCountsExactlyOnce`, `TestAdministrativeGenerationStartsFreshHalfOpenSample`, `concurrency_test.go` |
| Half-open exhaustion, cancellation and timeout | Probe/sample cap; optional finite context-sensitive wait; waiter count remains caller-owned | `TestHalfOpenAdmissionBoundUnderContention`, `wait_test.go`, `TestCanceledHalfOpenWaitReleasesClockTimer` |
| Long-running execution and permit expiry | Execute records a late outcome once; expired/stale permits cannot alter replacement sample; TTL does not terminate protected work | `TestExecuteRecordsCompletionsAfterPermitTTL`, `TestExecuteRecordsPanicAfterPermitTTL`, `admin_permit_test.go` |
| Callback panic, reentrancy and failure | Callbacks outside state lock; observer failures counted; execution panics recorded and rethrown; clock cleanup covered | `observer_test.go`, `collaborator_reentrancy_test.go`, `execute_test.go` |
| Observer amplification and shutdown | Rejections emit no transition event; bounded async queue drops/counts overflow; Close requests stop, Shutdown obeys context | `TestRepeatedOpenRejectionDoesNotAmplifyTransitionEvents`, `TestAsynchronousObserverIsBoundedAndDoesNotBlockAdmission`, `TestShutdownHonorsContextWhileObserverIsBlocked` |
| Timestamp movement and window complexity | Fixed rings; time snapshot scans bounded bucket count; backwards time does not resurrect expired records | `window/time_test.go`, `FuzzTimeWindowTimestamps`, `FuzzCountMatchesBoundedReference` |
| Supply-chain compromise | Dependency-free production; nested dependencies checksummed; immutable shared CI pin and private reporting | `go.mod`, `integration/consumers/go.sum`, `.github/workflows/ci.yml`, [SECURITY.md](../../SECURITY.md) |

### Immutable verification and release verdict

[CI run 36551645622](https://github.com/faustbrian/go-circuit-breaker/actions/runs/36551645622)
passed on 2026-09-29 at the exact audited source. Root and nested quality
contracts, CodeQL, repository contract and both Required jobs passed. Root
logs show vet, race, lint, Staticcheck, vulnerability, secrets, licenses,
seven fuzz targets, docs and API gates executed. Vulnerability logs report no
vulnerabilities; secret logs report no leaks. NilAway ran warning-only and
does not prove nil safety. Scheduled dependency review was skipped; release
rehearsal was not selected. Those are not claimed as passing.

| Owned module or package surface | Security disposition | Release verdict |
| --- | --- | --- |
| Root module, including breaker and `window` | No confirmed critical/high source finding; conditional risks below remain application-owned; root quality passed in the cited run | Retain v1.0.0; documentation-only, no module release |
| Root `breakertest` package | Deterministic testing support, not production timing assurance; covered by root quality | Same root release unit, no independent release |
| `integration/consumers` module | Non-production interoperability harness; its selected quality contract passed in the cited run | Non-releasable; no release |

The inventory has two modules; `window` and `breakertest` are root-module
packages, not separately released modules. Reuse applies only to unchanged
source, dependencies, and pinned tooling. New documentation requires current
structural checks and delivered-head CI. Scanner success does not certify
application classification, callback liveness, consumer security, or absence
of undisclosed vulnerabilities.

## Conditional residual risks

Repository disposition owner: `faustbrian`, package maintainer. Operational
owner: the deploying application's maintainer under its ownership policy.
Medium denotes service-availability loss requiring caller-selected policy or
trusted collaborators; low denotes metadata exposure or diagnostic loss
without a package-owned secret source. Reassess when review triggers change.

| Risk | Rationale and owner | Concrete mitigation | Review trigger |
| --- | --- | --- | --- |
| Medium: unlimited closed-state work or half-open waiters | Application maintainer; a health breaker intentionally does not bound closed concurrency or retain a finite waiter queue | Bound application admission/concurrency with a bulkhead, use immediate probe rejection or bound waiting callers, and apply request deadlines | Admission policy, workload fan-out, queue, or consumer change |
| Medium: TTL expires a still-running probe and allows replacement work | Application maintainer; expiry bounds sample retention, not actual live operations, and cannot safely terminate arbitrary work | Make transport deadlines shorter than TTL, audit cancellation, and use a separate concurrency budget | TTL/deadline change or new protected callback |
| Medium: classifier, synchronous observer, clock, random source, or timer stalls | Application maintainer; trusted callbacks cannot be forcibly killed; async shutdown can time out while a callback remains alive | Use prompt concurrency-safe collaborators and default clock/random; bound observer handoff; own Close/Shutdown and never call Shutdown from async observer | Collaborator change, callback incident, or observer lifecycle change |
| Medium: wrong classification, retries or administrative bypass defeats isolation | Application maintainer; protocol health and authorization are not core responsibilities | Keep admin controls private, classify local rejection/cancellation explicitly, bound retries and record one chosen logical-operation boundary | Protocol adapter, retry order, classifier, or admin interface change |
| Low: name contains secrets/control characters or creates metric cardinality | Application maintainer; length bound does not infer confidentiality or sanitize exporter syntax | Use fixed non-secret printable names; sanitize exported metadata; never request IDs, credentials or raw endpoints | Name derivation or exporter change |
| Low: callback retains Completion payloads or logs original error/panic | Application maintainer; trusted classifier sees ephemeral inputs and Execute preserves caller error/panic semantics | Do not retain Completion; redact at application logging/recovery boundaries | Classifier or logging/recovery change |
| Low: async event loss or callback reordering | Application maintainer; bounded overflow is intentional and synchronous callbacks may overlap | Monitor drops/failures, use generation order and snapshots for state, not events as an authoritative audit log | Observability or audit requirements change |

No critical or high in-package finding was confirmed by this source audit.
Acceptance of these risks is conditional on the stated mitigations, not proof
that every consumer applies them. Maintainer compromise and malicious releases
still require review, current scanning, release integrity and private reporting.

## Maintenance

This batch changes governance documentation only; no API, behavior, dependency,
advisory, changelog or public release is required. Future confirmed defects need
affected-version identification, focused regression evidence, upgrade guidance
and coordinated disclosure under [SECURITY.md](../../SECURITY.md). Review this
model after changes to admission, windows, permits, collaborators, diagnostics,
consumers, dependencies or release automation; increment its version when
assumptions or dispositions materially change.
