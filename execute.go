package breaker

import (
	"context"
	"time"
)

// Execute admits and invokes operation, classifies its completion, and returns
// the original typed result and operation error. A rejected operation is never
// invoked. T has no constraint beyond any; results remain caller-owned, including
// partial results accompanying errors. A nonnil ctx/constructed b/operation is
// required. Calls are synchronous: no timeout, retry, fallback or goroutine is
// added, and cancellation after admission is classified rather than force-stopped.
//
// The classifier runs outside the mutex on ephemeral Completion. Nonnil operation
// error takes precedence over a completion error; with nil operation error an
// invalid classification returns its typed error and cancels bookkeeping capacity.
// Panics in operation/classifier are recorded as failures and re-panicked with
// the same recovered value. Execute retains lifetime accounting after permit TTL;
// health mutation still requires its permit not already expired or become stale.
func Execute[T any](
	ctx context.Context,
	b *Breaker,
	operation func(context.Context) (T, error),
) (result T, operationErr error) {
	permit, err := b.acquire(ctx, true)
	if err != nil {
		return result, err
	}
	started, clockPanic := safeClockNow(b.config.clock)
	if clockPanic != nil {
		_ = permit.cancelAt(permit.admissionTime())
		panic(clockPanic)
	}

	defer func() {
		if panicValue := recover(); panicValue != nil {
			finished, finishPanic := safeClockNow(b.config.clock)
			if finishPanic != nil {
				finished = started
			}
			duration := elapsed(finished, started)
			_ = permit.completeAt(
				OutcomeFailure,
				duration >= b.config.slowCallDuration,
				finished,
				b.jitterSample(),
			)
			panic(panicValue)
		}
	}()

	result, operationErr = operation(ctx)
	finished, clockPanic := safeClockNow(b.config.clock)
	if clockPanic != nil {
		panic(clockPanic)
	}
	duration := elapsed(finished, started)
	outcome := b.config.classifier(Completion{
		Context:  ctx,
		Result:   result,
		Err:      operationErr,
		Duration: duration,
	})
	completionErr := permit.completeAt(
		outcome,
		duration >= b.config.slowCallDuration,
		finished,
		b.jitterSample(),
	)
	if completionErr != nil {
		_ = permit.cancelAt(finished)
	}
	if operationErr != nil {
		return result, operationErr
	}
	return result, completionErr
}

// Completion/cancellation recovery can finish bookkeeping at admission time when
// a borrowed clock panics, without masking the original operation panic.
func safeClockNow(clock Clock) (now time.Time, panicValue any) {
	defer func() { panicValue = recover() }()
	now = clock.Now()
	return now, nil
}

func elapsed(finished, started time.Time) time.Duration {
	return max(finished.Sub(started), 0)
}
