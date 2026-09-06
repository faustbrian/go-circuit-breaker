# Compatibility Policy

The root module is stable at v1 and follows Semantic Versioning. Releases use
root `vX.Y.Z` tags; there is no directory prefix.

Incompatible exported API or documented behavior changes require a new major
version. Minor and patch releases remain backward compatible within the
documented contract.

Compatibility includes exported Go APIs, error classification, serialization,
protocol behavior, persistence schemas, environment variables, command output,
resource ownership, ordering, retry/idempotency semantics, and documented
defaults. A compile-compatible change can still be behaviorally breaking.

Specification-backed modules MUST NOT diverge from their declared standards.
Ambiguities require documented decisions and stable tests. Deprecated APIs
follow [`DEPRECATION.md`](DEPRECATION.md).
