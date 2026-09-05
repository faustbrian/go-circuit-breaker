# Circuit-breaker consumer integration

This internal, non-releasable interoperability harness proves how applications
compose the public circuit-breaker API with `database/sql` and the Golib
JSON-RPC client. It owns executable cross-module evidence only; it does not own
production APIs, protocol policy, retries, database drivers, transports, or
application configuration.

The harness is an engineering-inventory module. It is not an installable
library, is not published under semantic-version tags, and must not appear in
the consumer catalog. Its supported toolchain is Go 1.26.6.

## Run the harness

From this directory, run the complete suite against the repository's current
root source through the intentional parent workspace:

```sh
go test ./...
```

To verify the exact released module versions pinned by this module instead of
the parent workspace's local circuit-breaker source, disable workspace
discovery explicitly:

```sh
GOWORK=off go test ./...
```

From the repository root, the aggregate gate performs module-isolated,
pinned-release verification for this harness. The shared tooling runs module
commands with `GOWORK=off`, so the harness does not resolve the workspace's local
circuit-breaker source:

```sh
make check
```

Both tests are deterministic and require no network service, database server,
credentials, environment variables, or caller configuration.

The module contains one external test package, `consumerintegration_test`, and
exports no package or reusable helper.

## Evidence map

| Test | Proves |
| --- | --- |
| `database_sql_test.go` | Dependency failures open the breaker, open rejection skips the driver, successful `*sql.Rows` remain caller-owned, and the caller closes them. |
| `jsonrpc_test.go` | Local JSON-RPC validation is ignored, wrapped transport failures retain their causes and open the breaker, and open rejection skips the transport. |

The module imports the released circuit-breaker and JSON-RPC modules through
its own `go.mod`. Production circuit-breaker code does not import this harness
or JSON-RPC, so the integration cannot add a runtime dependency or cycle.

## Construction and lifecycle

Each test constructs a circuit breaker with the validated default bounded count
window, an explicit one-sample opening policy, finite open duration, and
protocol-owned classifier. Construction fails the test if validation rejects
that configuration. There are no configuration files, builders, production
globals, or hidden initialization. A test-only atomic sequence gives each
standard-library driver registration a unique name.

Tests may run concurrently. The breaker owns its synchronized state and is
shut down by test cleanup. The harness and fake SQL driver start no goroutines
directly, but `database/sql` owns a connection-opener goroutine and connection
pool from `sql.Open` until cleanup calls `DB.Close`. The returned rows remain
caller-owned and are closed by the test. Context cancellation is passed through
the public operation boundaries; the harness owns no independent drain or
shutdown sequence.

Errors remain owned by their source packages. Assertions use `errors.Is` to
distinguish local validation, JSON-RPC transport failure, the wrapped transport
cause, SQL driver failure, and breaker rejection. Classification affects
breaker health only and does not rewrite the returned error or imply retry.

## When to use this module

Use this harness when changing the circuit-breaker contract, the JSON-RPC
client boundary, dependency-failure classification, rejection behavior, or
caller-owned SQL row lifecycle. Do not import it from an application, copy its
one-sample opening policy into production, or treat its classifiers as a
universal database or RPC policy. Application owners must choose thresholds,
timeouts, retries, fallbacks, and protocol-specific outcomes for their own
workloads.

## Security and operations

The fixtures use no secrets or customer data and retain no request or response
payloads. Keep future diagnostics bounded and free of credentials, SQL values,
RPC parameters, tenant identifiers, and operation errors. This harness makes
no performance or production-readiness claim; root benchmarks and operational
guidance remain authoritative for the public breaker.

Compatibility is tied to the exact module versions in `go.mod` and `go.sum`.
Any dependency update must preserve the public composition outcomes above and
pass the repository gate. Because this module is non-releasable, it has no
installation, migration, deprecation, or independent support policy.

## Troubleshooting and FAQ

If module resolution or checks fail, first confirm Go 1.26.6 is active and the
checked-in `go.mod` and `go.sum` are unchanged. The deterministic SQL driver is
intentional: no local PostgreSQL instance should be started. The JSON-RPC test
also uses an in-memory transport, so a network request indicates a regression.

For expected breaker behavior, use the public API and composition guides below.
For reproducible harness defects, follow the repository support and
contribution guidance; security reports must use the private reporting path.

## References

- [Circuit-breaker composition guide](../../docs/composition.md)
- [Public API and defaults](../../docs/api.md)
- [Verification and performance evidence](../../docs/verification.md)
- [Security policy](../../SECURITY.md)
- [Support](../../SUPPORT.md)
- [Contributing and testing](../../CONTRIBUTING.md)
- [Repository changelog](../../CHANGELOG.md)
- [License](../../LICENSE)
- [Golib ecosystem index](https://github.com/faustbrian/go-library-tools/blob/v1.5.3/docs/ecosystem/README.md)
- [Resilience family guidance](https://github.com/faustbrian/go-library-tools/blob/v1.5.3/docs/ecosystem/design-language.md#package-families-and-selection)
- [Public circuit-breaker API reference](https://pkg.go.dev/github.com/faustbrian/go-circuit-breaker)
