package timeutil

import (
	"context"
	"time"
)

// PeriodicExec runs a function on a fixed interval until the context it was
// started with is cancelled.
type PeriodicExec struct {
	interval  time.Duration
	offset    time.Duration
	fn        func(ctx context.Context)
	recovery  func(v any)
	immediate bool
}

// NewPeriodicExec creates a fresh periodic executor.
//
// The offset staggers the first run only, so that several executors sharing
// an interval do not all fire at the same instant; immediate runs the
// function once before any waiting at all. The recovery function contains a
// panic raised by a single run, leaving the schedule itself intact — a nil
// one lets the panic propagate to whoever owns the goroutine.
func NewPeriodicExec(
	interval time.Duration,
	offset time.Duration,
	fn func(ctx context.Context),
	recovery func(v any),
	immediate bool,
) *PeriodicExec {
	return &PeriodicExec{
		interval:  interval,
		offset:    offset,
		fn:        fn,
		recovery:  recovery,
		immediate: immediate,
	}
}

// Start runs the function until the context is cancelled. It blocks, so the
// caller owns the goroutine it runs on.
func (pe *PeriodicExec) Start(ctx context.Context) {
	if pe.immediate {
		pe.exec(ctx)
	}

	tm := time.NewTimer(pe.interval + pe.offset)
	defer tm.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.C:
			pe.exec(ctx)
			tm.Reset(pe.interval)
		}
	}
}

// exec runs the function once, containing a panic when a recovery function
// is configured.
func (pe *PeriodicExec) exec(ctx context.Context) {
	if pe.recovery != nil {
		defer func() {
			if v := recover(); v != nil {
				pe.recovery(v)
			}
		}()
	}

	pe.fn(ctx)
}
