package breaker

import (
	"context"
	"time"
)

// Execute admits and invokes operation, classifies its completion, and returns
// the original typed result and operation error. A rejected operation is never
// invoked. Panics are recorded as failures and re-panicked with the same value.
func Execute[T any](
	ctx context.Context,
	b *Breaker,
	operation func(context.Context) (T, error),
) (result T, operationErr error) {
	permit, err := b.Acquire(ctx)
	if err != nil {
		return result, err
	}
	started := b.config.clock.Now()

	defer func() {
		if panicValue := recover(); panicValue != nil {
			duration := elapsed(b.config.clock.Now(), started)
			_ = permit.Complete(OutcomeFailure, duration >= b.config.slowCallDuration)
			panic(panicValue)
		}
	}()

	result, operationErr = operation(ctx)
	duration := elapsed(b.config.clock.Now(), started)
	outcome := b.config.classifier(Completion{
		Result:   result,
		Err:      operationErr,
		Duration: duration,
	})
	completionErr := permit.Complete(outcome, duration >= b.config.slowCallDuration)
	if operationErr != nil {
		return result, operationErr
	}
	return result, completionErr
}

func elapsed(finished, started time.Time) time.Duration {
	duration := finished.Sub(started)
	if duration < 0 {
		return 0
	}
	return duration
}
