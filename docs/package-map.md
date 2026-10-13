# Package map

[Documentation index](README.md) · [Repository entry](../README.md)

This map follows the current source modules, not a synchronized release.
Use the repository README for published installation versions; an unpublished
source major is not installable merely because its package appears here.
Internal packages and non-releasable fixtures are intentionally excluded.

## github.com/faustbrian/go-circuit-breaker

`circuit-breaker` is a protocol-neutral, bounded, concurrent circuit breaker for Go. It owns admission and dependency-health state. Callers retain control of timeouts, retries, fallbacks, request bodies, errors, and protocol policy.

| Import path | Responsibility |
| --- | --- |
| [`github.com/faustbrian/go-circuit-breaker`](../doc.go) | Use for explicit process-local dependency-health admission with bounded rolling evidence and caller-owned execution policy. |
| [`github.com/faustbrian/go-circuit-breaker/breakertest`](../breakertest/clock.go) | Test support; see the linked source and module guide. |
| [`github.com/faustbrian/go-circuit-breaker/window`](../window/count.go) | Public window package; see the linked source and module guide. |

Choose the next guide in the [documentation index](README.md)
or return to the [repository entry](../README.md) for installation and ownership.
