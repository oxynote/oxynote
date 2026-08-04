// Package ctxutil implements various functions for context manipulation and
// inspection.
package ctxutil

import (
	"context"
	"errors"
	"sync"
	"time"
)

// IsInterrupted checks if the error contains context interruption error.
func IsInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// multiCtx wraps multiple contexts and handles their cancellation.
type multiCtx struct {
	ctxs []context.Context

	mu   sync.RWMutex
	err  error
	done chan struct{}
}

// MultiContext combines multiple contexts into one.
// Canceling this context releases resources associated with it, so code
// should call cancel as soon as the operations running in this
// Context complete.
// Note that most context-related operations (e.g. value extraction) depend
// on the order of the privided contexts.
func MultiContext(ctxs ...context.Context) (context.Context, context.CancelFunc) {
	mctx := &multiCtx{
		ctxs: ctxs,
		done: make(chan struct{}),
	}

	mctx.run()

	return mctx, func() { mctx.cancel(context.Canceled) }
}

// Deadline returns the closest deadline.
func (m *multiCtx) Deadline() (deadline time.Time, ok bool) {
	for i := range m.ctxs {
		if !ok {
			deadline, ok = m.ctxs[i].Deadline()
			continue
		}

		d, ok1 := m.ctxs[i].Deadline()
		if !ok1 || !d.Before(deadline) {
			continue
		}

		deadline = d
	}

	return deadline, ok
}

// Done returns a channel that's closed when work done on behalf of this
// context is cancelled.
func (m *multiCtx) Done() <-chan struct{} {
	return m.done
}

// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
// After Err returns a non-nil error, successive calls to Err return the
// same error.
func (m *multiCtx) Err() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.err
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
// Note that all wrapped contexts are checked for this value in the
// order they are provided.
func (m *multiCtx) Value(key any) any {
	for i := range m.ctxs {
		v := m.ctxs[i].Value(key)
		if v != nil {
			return v
		}
	}

	return nil
}

// cancel cancels the context with the provided error.
func (m *multiCtx) cancel(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return
	}

	m.err = err

	close(m.done)
}

// run waits for each context's cancellation on separate goroutines.
func (m *multiCtx) run() {
	for i := range m.ctxs {
		go func() {
			done := m.ctxs[i].Done()
			if done == nil {
				return
			}

			select {
			case <-done:
				m.cancel(m.ctxs[i].Err())
			case <-m.done:
			}
		}()
	}
}
