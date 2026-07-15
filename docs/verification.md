# Verification and release evidence

## Reproducible commands

`make check` is the local and CI aggregate. Its gates are:

```text
make fmt vet lint test integration coverage race fuzz leak benchmark docs
make safety compatibility
actionlint .github/workflows/*.yml
```

Coverage must be exactly 100.0% production statements. Race runs repeat every
package three times. Fuzz smoke covers configuration, permit/admin sequences,
count reference parity, and time movement. Deterministic tests cover transition
tables, ratios, concurrency, fake timers, observer reentrancy/panic, permit
expiry, and reference-model divergence. Leak checks repeat timer cancellation
and observer shutdown.

## Benchmark baseline

Recorded 2026-07-15 using Go 1.26.5, darwin/arm64, Apple M4 Max,
`go test -run=^$ -bench=. -benchtime=300ms -benchmem ./...`:

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ClosedExecute | 182.9 | 64 | 1 |
| OpenRejection | 56.17 | 80 | 1 |
| Snapshot | 156.8 | 0 | 0 |
| HalfOpenContention | 169.5 | 80 | 1 |
| SynchronousTransitionObserver | 509.2 | 1472 | 6 |
| CountRollover | 5.499 | 0 | 0 |
| TimeRollover | 5.332 | 0 | 0 |
| TimeSnapshot | 130.5 | 0 | 0 |

These are regression evidence, not cross-machine latency guarantees. CI runs a
short benchmark smoke; maintainers compare stable-runner history before release.
Run `make profile` for reproducible CPU, allocation, and mutex profiles under
`profiles/`; inspect them with `go tool pprof`. Generated profiles are evidence
artifacts and are not committed.

## Security and dependencies

Core and all packages currently have no third-party module dependency.
`govulncheck ./...` is required. GO-SAFETY-1 rejects production `unsafe`, cgo,
`go:linkname`, and finalizers. Workflows use minimal permissions and commit-pinned
actions; pull requests receive dependency review. Tagged source archives have
SHA-256 checksums and GitHub artifact attestations.

## Compatibility

Go 1.24 is the minimum tested version. Before the first tag, the exported API is
the v1 baseline. After a tag exists, `make compatibility` requires `apidiff` and
compares the latest SemVer tag with the working tree. Exported types, defaults,
state transitions, timing, classification, snapshots, and error identity are
semantic compatibility surfaces.

## Release verdict

The implementation has deterministic evidence for every core transition,
threshold, window, permit terminal path, classifier outcome, snapshot, observer,
and administrative mode. HTTP plus database and queue integration suites keep
protocol policy outside core. No release blocker is known.

Remaining risks are caller-owned policy correctness, workload-specific tuning,
and integration verification in downstream `go-http-client`/vendor repositories.
Those repositories must prove their middleware order and body ownership against
their own concrete transports; this module intentionally does not duplicate
their state machine.
