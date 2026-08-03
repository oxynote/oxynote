// Package xync provides goroutine management types and functions.
package xync

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Supervisor handles goroutine creation and termination.
type Supervisor struct {
	wg         sync.WaitGroup
	sem        *semaphore.Weighted
	recoveryFn func(any)

	// base context is cancelled only on close
	baseCtx    context.Context
	baseCancel context.CancelFunc

	// active context is cancelled and recreated on every stop
	activeMu     sync.RWMutex
	activeCtx    context.Context
	activeCancel context.CancelFunc
}

// SupervisorOption is used to configure the supervisor.
type SupervisorOption func(*Supervisor)

// WithSupervisorBaseContext sets the base context that is passed to
// all goroutines. Note that the supervisor wraps the provided context in
// its own internal context that is used to cancel all goroutines.
func WithSupervisorBaseContext(ctx context.Context) SupervisorOption {
	return func(s *Supervisor) {
		s.baseCtx = ctx
	}
}

// WithSupervisorMaxActive sets the maximum number of active goroutines.
// When the limit is reached, all new goroutines remain idle and wait
// until their function is executed.
func WithSupervisorMaxActive(max int64) SupervisorOption {
	return func(s *Supervisor) {
		s.sem = semaphore.NewWeighted(max)
	}
}

// WithSupervisorRecovery sets a function that is called on
// each panic inside the Go() method.
func WithSupervisorRecovery(fn func(any)) SupervisorOption {
	return func(s *Supervisor) {
		s.recoveryFn = fn
	}
}

// NewSupervisor creates a fresh instance of the goroutine supervisor.
func NewSupervisor(opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		recoveryFn: func(a any) {}, // suppress
	}

	for _, opt := range opts {
		opt(s)
	}

	baseCtx := context.Background()

	if s.baseCtx != nil {
		baseCtx = s.baseCtx
	}

	s.baseCtx, s.baseCancel = context.WithCancel(baseCtx)
	s.activeCtx, s.activeCancel = context.WithCancel(s.baseCtx)

	return s
}

// Go executes the provided function on a new goroutine.
// The provided function must handle all context cancellation events and
// exit when required.
// Once closed, the supervisor passes a cancelled context to all new
// goroutines and their functions.
func (s *Supervisor) Go(fn func(context.Context)) {
	s.activeMu.RLock()
	ctx := s.activeCtx
	s.activeMu.RUnlock()

	s.wg.Add(1)

	go func() {
		defer func() {
			if v := recover(); v != nil {
				s.recoveryFn(v)
			}
		}()
		defer s.wg.Done()

		if s.sem != nil {
			// only context errors are returned here, however,
			// the fn() may want to know that the context
			// was cancelled and act accordingly.
			if err := s.sem.Acquire(ctx, 1); err == nil {
				defer s.sem.Release(1)
			}
		}

		fn(ctx)
	}()
}

// BaseContext returns the context that is associated with the supervisor
// and is cancelled only when Close is called.
func (s *Supervisor) BaseContext() context.Context {
	return s.baseCtx
}

// ActiveContext returns the context that is associated with the supervisor
// and is cancelled either when Stop or Close is called.
func (s *Supervisor) ActiveContext() context.Context {
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()

	return s.activeCtx
}

// Wait blocks until all active goroutines exit.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}

// Stop stops all currently active/idle goroutines.
func (s *Supervisor) Stop() {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	s.activeCancel()
	s.activeCtx, s.activeCancel = context.WithCancel(s.baseCtx)
}

// StopAndWait stops all currently active/idle goroutines and blocks until
// all of them exit.
func (s *Supervisor) StopAndWait() {
	s.Stop()
	s.Wait()
}

// Close stops all currently active/idle goroutines and ensures
// that all new goroutines that are created in the future are stopped as
// well.
func (s *Supervisor) Close() {
	s.baseCancel()
}

// CloseAndWait stops all currently active/idle goroutines, blocks until
// all of them exit and ensures that all new goroutines that are created in
// the future are stopped as well.
func (s *Supervisor) CloseAndWait() {
	s.Close()
	s.Wait()
}
