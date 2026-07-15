# Composition and integration

Composition is ordered. The breaker observes exactly the dependency boundary
the caller places inside `Execute` or between permit acquisition/completion.

## Recommended order

```text
cache -> local validation -> rate limit -> breaker -> retry -> auth/sign -> transport
```

This order records one logical operation after its retry policy. Put retry
outside the breaker only when attempt-level dependency health is intentionally
desired. Never wrap the same operation with the same breaker twice.

- Cache hits and fallbacks normally bypass admission.
- Rate limiting normally precedes admission so local rejection consumes no
  half-open probe.
- Bulkhead rejection is normally ignored unless it signals dependency
  saturation by explicit policy.
- The caller owns timeout creation. Classify cancellation/deadline according to
  whether the dependency caused it.
- Authentication/signing may occur inside only when it is part of the remote
  attempt; local credential/configuration failures should usually be ignored.
- Telemetry failure must not affect process-local admission.

## HTTP

`go-http-client` should own status classification and canonical middleware
order. Core does not read or close bodies. The caller/transport must close every
received body, including responses classified as failure. Classification may
inspect status and headers but must not consume or retain the body.

The integration suite proves one 503 logical operation opens the circuit,
caller body ownership remains intact, and rejection does not invoke transport.

## RPC

Map transport-unavailable, resource-exhausted, and server-side deadline codes
to dependency failure as appropriate. Treat caller cancellation, local marshal
errors, and authentication configuration explicitly. Preserve the original RPC
error; classification changes breaker state, not the returned error.

## PostgreSQL and Valkey

Wrap the complete `QueryContext`, `ExecContext`, or command call. Connection,
protocol, and server availability failures are typical failures. Domain misses
such as `sql.ErrNoRows`, optimistic conflicts, and caller cancellation require
application policy. Do not hold a permit while rows are idle unless iteration
is deliberately part of the protected dependency operation; always close rows.

For Valkey, distinguish local pool/bulkhead exhaustion from remote connection or
server failure. Cache misses are normally successful dependency interactions.

## Queues and object storage

Broker unavailability and publish failures are typical queue failures. A local
bounded producer queue being full is normally ignored. For consumers, decide
whether the protected operation is receive, handler execution, or ack; never
record one delivery into the same breaker at multiple layers.

For object storage, classify transport/server failures separately from expected
not-found and precondition responses. Close response bodies/streams outside
core. Multipart operations need a caller-owned aggregate policy so one logical
operation is recorded once.

## Filesystems and arbitrary functions

Remote filesystem gateway failures may be dependency failures; local path
validation and permission policy often are not. Two-step permits fit callbacks,
streaming, or async APIs that cannot live in one function, provided every permit
is completed or canceled within its finite TTL.
