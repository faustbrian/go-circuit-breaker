# circuit-breaker

[![CI](https://github.com/faustbrian/go-circuit-breaker/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/faustbrian/go-circuit-breaker/actions/workflows/ci.yml)
[![CodeQL](https://img.shields.io/badge/CodeQL-required-blue)](https://github.com/faustbrian/go-circuit-breaker/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-100%25_required-blue)](CONTRIBUTING.md#verification)
[![Mutation](https://img.shields.io/badge/mutation-100%25_required-blue)](CONTRIBUTING.md#verification)
[![Documentation](https://img.shields.io/badge/docs-checked_in_CI-blue)](docs/)
[![Go Reference](https://pkg.go.dev/badge/github.com/faustbrian/go-circuit-breaker.svg)](https://pkg.go.dev/github.com/faustbrian/go-circuit-breaker)
[![Release](https://img.shields.io/github/v/release/faustbrian/go-circuit-breaker?sort=semver)](https://github.com/faustbrian/go-circuit-breaker/releases)
[![Go](https://img.shields.io/badge/go-1.26.6-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`circuit-breaker` is a protocol-neutral, bounded, concurrent circuit breaker
for Go. It owns admission and dependency-health state. Callers retain control of
timeouts, retries, fallbacks, request bodies, errors, and protocol policy.

Core uses only the standard library. There is no global registry, per-call
goroutine, hidden retry, operation timeout, or distributed coordinator.

## Five-minute quickstart

```go
package main

import (
	"context"
	"errors"
	"log"
	"time"

	breaker "github.com/faustbrian/go-circuit-breaker"
)

func main() {
	circuit, err := breaker.New(breaker.Config{
		Name:              "catalog",
		Window:            breaker.CountWindow{Size: 50},
		MinimumThroughput: 10,
		Opening: &breaker.OpeningRules{
			FailureRatio: 0.5,
		},
		OpenDuration: breaker.FixedOpenDuration(30 * time.Second),
		HalfOpen: &breaker.HalfOpenPolicy{
			MaxProbes:         3,
			RequiredSuccesses: 3,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	value, err := breaker.Execute(context.Background(), circuit,
		func(ctx context.Context) (string, error) {
			return callCatalog(ctx)
		})
	if errors.Is(err, breaker.ErrOpen) {
		// Apply a caller-owned fallback or return an availability response.
		return
	}
	_ = value
}

func callCatalog(context.Context) (string, error) { return "available", nil }
```

By default, nil errors are successes and non-nil errors are failures. Define a
classifier for HTTP status codes, RPC codes, local rejection, cancellation, or
any other protocol semantics.

## State model

```text
                     opening rule
        +--------------------------------------+
        |                                      v
     CLOSED <--- recovery threshold --- HALF-OPEN <--- interval --- OPEN
        ^                                      |
        +------------- reset ------------------+
                                               |
                                 failed recovery reopens
```

Administrative mode is an independent override: normal, force-open, disabled,
or isolated. Every policy and administrative transition creates a new
generation, so stale permits cannot mutate current health state. Their
successful completions still contribute exactly once to lifetime outcome
totals.

## Choose an API

- Use generic `Execute` for a context-aware function. It preserves the typed
  result and original operation error.
- Use `Acquire`, followed by exactly one `Complete` or `Cancel`, when the caller
  owns a multi-step lifecycle.
- Use `Snapshot` for bounded aggregate state and lifetime completion totals. It
  never contains results or operation errors.
- Use an `Observer` for transition events. Asynchronous delivery is bounded and
  must be stopped by its owner with `Breaker.Shutdown` or nonblocking `Close`.

## Documentation

- [API and defaults](docs/api.md)
- [Policies, windows, classification, and permits](docs/policies.md)
- [HTTP, RPC, database, queue, storage, and resilience composition](docs/composition.md)
- [Operations, observability, tuning, incidents, and troubleshooting](docs/operations.md)
- [State-machine specification, threat model, and linearization](docs/design.md)
- [Assurance, findings, ownership, and proof matrix](docs/assurance.md)
- [Verification, benchmarks, compatibility, and release evidence](docs/verification.md)
- [Security policy](SECURITY.md) and [contribution guide](CONTRIBUTING.md)

For ecosystem-wide selection and ownership guidance, see the versioned
[Golib ecosystem index](https://github.com/faustbrian/go-library-tools/blob/v1.4.0/docs/ecosystem/README.md)
and its [Resilience family](https://github.com/faustbrian/go-library-tools/blob/v1.4.0/docs/ecosystem/design-language.md#package-families-and-selection).

## Installation and support

```sh
go get github.com/faustbrian/go-circuit-breaker
make check
```

The minimum supported toolchain is Go 1.24. See [SUPPORT.md](SUPPORT.md) for the
compatibility policy. This project is licensed under the MIT License.
