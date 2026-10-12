# Contributing

## Before Editing

1. Read the README, [`AGENTS.md`](AGENTS.md) and the affected API contract.
2. Start with `go test ./...` or a focused package/test selector.
3. Inspect direct consumers when changing a public or composition contract.
4. Preserve unrelated work and generated/corpus provenance.

An ordinary fix stays in this repository. It does not require sibling checkouts,
Tools edits, ecosystem catalog updates or agent-goal ledgers. Change manifests
only when their owned facts change, not merely because source changed.

## Changes

Keep commits focused and conventional. Update changelogs for material public
behavior, compatibility or adoption changes, not CI-only administration.
Public API changes require compatibility
evidence and documentation. Specification behavior requires a decision record,
fixture coverage, and interoperability evidence.

New direct dependencies and dependency updates must follow the
[dependency governance policy](AGENTS.md#dependencies-and-supply-chain). Package-local
update bots are forbidden; the root policy owns every module and action update.

Specification-backed changes must follow the
[specification governance contract](AGENTS.md#design), update
the affected stable decision entries, and complete the Specification Decisions
section of the pull request template. An unresolved interpretation or stale
source pin is release-blocking; peer behavior cannot silently select policy.

Mutation is optional developer-local or explicitly selected full assurance, not a PR
prerequisite. Investigate viable survivors and add focused regressions for real
defects; moving a campaign off PRs does not waive a confirmed correctness or
security defect. Equivalent/invalid mutants retain reviewed narrow dispositions.

Do not fork shared verification workflows, add permanent replacements, machine-specific
paths, bypass flags, broad mutation exclusions, or aggregate quality metrics
that hide a failing package.

## Verification

For ordinary source work, `go test ./...` runs deterministic tests, fuzz seeds
and saved corpus regressions. Use the qualified public Golib v2.4.0 verifier
declared in `.golib.yaml` for repository checks:

```bash
make inventory
make check
make ci
```

`check` and `ci` use bounded local checks, including the named half-open admission,
completion, timer and observer race assertions in `.golib.yaml`; they do not run
full mutation or repeated
assurance campaigns. Documentation changes need relevant structure, links and
examples, not unrelated runtime work. Public API, concurrency and lifecycle
changes retain their affected compatibility, race and composition checks.

For active exploration, select one fuzz target and a small budget; long campaigns
remain explicit full assurance. Optional local mutation selects one package with
the qualified verifier. These examples are not all-change requirements:

```bash
GOMAXPROCS=2 go test -run '^$' -fuzz '^FuzzPermitOperationSequences$' -fuzztime=2s -parallel=1 -timeout=2m .
golib mutation --module . --package .
```

For changes to the HTTP/database/protocol composition contract, run the affected
integration tests. The nested consumer module remains a separate public-dependency
boundary and tests the declared released core baseline, not uncommitted core:

```bash
go test ./integration/...
(cd integration/consumers && go test ./...)
```

Candidate-core and direct-consumer qualification must select the intended source
explicitly in disposable consumer storage, not introduce permanent replacements
or mistake released-core results for current-source proof.

`make check-full` retains explicit full assurance, including the configured
repeated lifecycle tests and original fuzz budgets.
Relevant release qualification uses `golib release dry-run --module .`. Select
these for their material risk or release boundary, not every commit. Report
unavailable or failed checks honestly; a fast local pass is not public-release
or hosted-CI qualification. CI-only changes do not require a module release.

## Selected Monthly Assurance

The owner-level [selected-assurance caller](.github/workflows/assurance.yml) is
configured for main only, on day 20 of each month at 06:47 UTC. It selects one
of the 7 current declared root-module fuzz targets using the shared UTC-week
rotation, with 30 seconds of active exploration. The selected risks are
permit/state snapshot invariants, observer-failure accounting, configuration
resource bounds, execution-outcome accounting and count/time-window reference
models. Root and `window` are owned production packages in the root module;
`breakertest` supplies test fixtures, not a selected production target. Injected
clocks do not certify real dependency timing. The nested released-core consumer
and HTTP/database/protocol composition checks retain their full/manual routes.

Base-free ordinary checks execute deterministic seeds, not active exploration;
committed-range CI selects affected active smoke. Manual assurance requires an
exact anchored selector from the root fuzz operation in `.golib.yaml`, for
example `^FuzzPermitOperationSequences$`.
Empty, unknown and malformed selections refuse. The shared job bounds setup
and execution to ten minutes, active workers and CPU to two, owned memory to
2 GiB, native output to 1 MiB and private-cache lifetime. Failure, cancellation,
exhausted budgets and cleanup refusal remain attributable failures. This lane
does not run mutation or replace Required, original full fuzz budgets,
explicit `make check-full`, or relevant release qualification.

## Adding A Module

Follow [repository structure policy](AGENTS.md#repository-structure). New modules
require an explicit purpose, ownership boundary, dependency review, package
catalog entry, full quality gates, documentation, changelog, license, security
policy, compatibility plan, and release dry-run.

## Source Documentation

Install the checksum-pinned public verifier declared in `.golib.yaml`.
`make source-docs-check` validates objective Go comment, marker, link and
generated-producer structure; `make check`, `make check-full` and `make ci`
require it before their existing checks. Shared CI runs the same strict
contract and includes its result in Required. This is structural enforcement,
not a substitute for source-truth review or behavioral verification.
