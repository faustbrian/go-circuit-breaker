package breaker

import (
	"context"
	"sync/atomic"
	"time"
)

// TransitionReason identifies why a policy or administrative transition ran.
type TransitionReason uint8

const (
	// ReasonPolicyOpened reports closed health evidence satisfied opening rules.
	ReasonPolicyOpened TransitionReason = iota
	// ReasonOpenIntervalElapsed reports admission lazily observed open eligibility.
	ReasonOpenIntervalElapsed
	// ReasonHalfOpenRecovered reports the classified probe sample satisfied recovery.
	ReasonHalfOpenRecovered
	// ReasonHalfOpenFailed reports immediate/full-sample probe policy failed.
	ReasonHalfOpenFailed
	// ReasonForceOpen reports administrative force-open was selected.
	ReasonForceOpen
	// ReasonDisabled reports administrative disabled mode was selected.
	ReasonDisabled
	// ReasonIsolated reports administrative isolation was selected.
	ReasonIsolated
	// ReasonReleased reports normal policy admission was restored.
	ReasonReleased
	// ReasonReset reports state/window/schedule were reset to normal closed.
	ReasonReset
)

// String returns a stable transition label or "unknown" for unsupported values.
func (r TransitionReason) String() string {
	switch r {
	case ReasonPolicyOpened:
		return "policy-opened"
	case ReasonOpenIntervalElapsed:
		return "open-interval-elapsed"
	case ReasonHalfOpenRecovered:
		return "half-open-recovered"
	case ReasonHalfOpenFailed:
		return "half-open-failed"
	case ReasonForceOpen:
		return "force-open"
	case ReasonDisabled:
		return "disabled"
	case ReasonIsolated:
		return "isolated"
	case ReasonReleased:
		return "released"
	case ReasonReset:
		return "reset"
	default:
		return "unknown"
	}
}

// TransitionEvent contains copied before/after state from one committed
// transition, without operation data. Delivery need not follow generation order
// across transitioning callers; timestamps are observations, not ordering fences.
type TransitionEvent struct {
	// Before copies policy/admin state immediately before the transition.
	Before Snapshot
	// After copies policy/admin state after the committed transition.
	After Snapshot
	// Reason is the state/admin cause, not a retained operation error.
	Reason TransitionReason
	// Generation equals After.Generation, a local epoch rather than delivery order.
	Generation uint64
	// Timestamp is the transition Clock.Now observation and may repeat/move backward.
	Timestamp time.Time
}

// Observer receives copied transition events outside the state mutex. Errors and
// panics count as observer failures, without retaining error text or propagating
// them to protected work. Synchronous callbacks can overlap/reenter and delay
// callers; async callbacks are serialized by one worker. Borrowed callbacks must
// bound their own work; shutdown cannot interrupt them.
type Observer func(TransitionEvent) error

// EventDeliveryPolicy selects SynchronousEvents or AsynchronousEvents values,
// not their pointers. Nil with an observer selects async buffer 64/DropNewestEvent;
// any delivery policy without an observer is invalid.
type EventDeliveryPolicy interface{ eventDeliveryPolicy() }

// SynchronousEvents invokes the observer in the transitioning caller after the
// internal lock is released. Callbacks may overlap/reenter; neither Close nor
// Shutdown waits for or stops synchronous callbacks.
type SynchronousEvents struct{}

func (SynchronousEvents) eventDeliveryPolicy() {}

// EventOverflowPolicy controls a full asynchronous event queue.
type EventOverflowPolicy uint8

const (
	// DropNewestEvent refuses the incoming event when the async queue is full.
	DropNewestEvent EventOverflowPolicy = iota
	// DropOldestEvent evicts a queued event to retain the incoming transition.
	DropOldestEvent
)

// AsynchronousEvents delivers through one bounded worker queue. Delivery is
// serial in enqueue order, not necessarily committed generation order; overflow
// drops are explicit. Creator owns Close/Shutdown for the worker lifetime.
type AsynchronousEvents struct {
	// Buffer is positive through MaxEventBuffer, bounding queued events only;
	// an active callback and producer-local event snapshots are additional storage.
	Buffer int
	// Overflow selects drop-newest (zero) or drop-oldest; unknown values fail.
	Overflow EventOverflowPolicy
}

func (AsynchronousEvents) eventDeliveryPolicy() {}

type observerRuntime struct {
	observer Observer
	mode     observerMode
	buffer   int
	overflow EventOverflowPolicy
}

type observerMode uint8

const (
	observerDisabled observerMode = iota
	observerSynchronous
	observerAsynchronous
)

type observerCounters struct {
	failures atomic.Uint64
	dropped  atomic.Uint64
}

func (b *Breaker) startObserver() {
	policy := b.config.observer
	switch policy.mode {
	case observerDisabled, observerSynchronous:
		return
	}
	b.eventChannel = make(chan TransitionEvent, policy.buffer)
	b.eventStop = make(chan struct{})
	b.eventDone = make(chan struct{})
	go b.runObserver()
}

func (b *Breaker) runObserver() {
	defer close(b.eventDone)
	for {
		select {
		case event := <-b.eventChannel:
			b.observe(event)
		case <-b.eventStop:
			for {
				select {
				case event := <-b.eventChannel:
					b.observe(event)
				default:
					return
				}
			}
		}
	}
}

func (b *Breaker) observe(event TransitionEvent) {
	failed := false
	func() {
		defer func() {
			if recover() != nil {
				failed = true
			}
		}()
		if err := b.config.observer.observer(event); err != nil {
			failed = true
		}
	}()
	if failed {
		b.observerCounters.failures.Add(1)
	}
}

func (b *Breaker) dispatch(events []TransitionEvent) {
	for _, event := range events {
		switch b.config.observer.mode {
		case observerSynchronous:
			b.observe(event)
		case observerAsynchronous:
			b.enqueue(event)
		}
	}
}

// eventMu serializes all producers and Close. The queue itself remains open so
// a sender cannot race a channel close; eventStop signals draining instead. After
// drop-oldest frees a slot no other producer can refill it under this lock.
func (b *Breaker) enqueue(event TransitionEvent) {
	b.eventMu.Lock()
	defer b.eventMu.Unlock()
	if b.eventClosed.Load() {
		b.observerCounters.dropped.Add(1)
		return
	}
	select {
	case b.eventChannel <- event:
		return
	default:
	}
	switch b.config.observer.overflow {
	case DropNewestEvent:
		b.observerCounters.dropped.Add(1)
		return
	}
	select {
	case <-b.eventChannel:
		b.observerCounters.dropped.Add(1)
	default:
	}
	b.eventChannel <- event
}

func (b *Breaker) takeEventsLocked() []TransitionEvent {
	events := b.pendingEvents
	b.pendingEvents = nil
	return events
}

func (b *Breaker) unlockAndDispatch() {
	events := b.takeEventsLocked()
	b.mu.Unlock()
	b.dispatch(events)
}

// Close requests asynchronous observer shutdown without waiting for callbacks.
// It is idempotent, callback-safe, and does not change breaker admission or state.
// Accepted async events are drained; later dispatches count as dropped. It does
// not wait for an active callback, stop synchronous observation or release caller
// operations/permits. A constructed nonnil Breaker is required.
func (b *Breaker) Close() error {
	switch b.config.observer.mode {
	case observerDisabled, observerSynchronous:
		return nil
	}
	b.eventCloseOnce.Do(func() {
		b.eventMu.Lock()
		b.eventClosed.Store(true)
		close(b.eventStop)
		b.eventMu.Unlock()
	})
	return nil
}

// Shutdown requests asynchronous observer shutdown and waits for queued
// callbacks to finish, or returns ctx.Err without interrupting a blocked callback.
// A nonnil context is required for async mode. Repeated waits are allowed;
// disabled/synchronous observation returns immediately without waiting for callers.
// It must not be called from the async worker itself (self-wait); use Close there.
func (b *Breaker) Shutdown(ctx context.Context) error {
	_ = b.Close()
	if b.eventDone == nil {
		return nil
	}
	select {
	case <-b.eventDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
