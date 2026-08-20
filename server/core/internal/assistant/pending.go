package assistant

import (
	"context"
	"errors"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// pendingConfirm describes a confirmation awaiting the user's answer.
// It is stored in Redis rather than on the session: the run it belongs
// to lives in a checkpoint, so the question outlives both the
// connection that asked it and this process.
type pendingConfirm struct {
	// TurnID correlates the request with the client's response.
	TurnID string `json:"turn_id"`

	// InterruptIDs addresses every write paused by this confirmation.
	InterruptIDs []string `json:"interrupt_ids"`

	// Actions describes each paused write for the user.
	Actions []protocol.ConfirmAction `json:"actions"`
}

// loadPending returns the confirmation this conversation is waiting on,
// or nil when there is none. An unreadable record is treated as absent:
// the turn stays parked rather than resuming writes nobody approved.
func (m *Manager) loadPending(ctx context.Context, key string) *pendingConfirm {
	pending, err := m.pendings.Get(ctx, createPendingKey(key))

	switch {
	case err == nil:
		return pending
	case errors.Is(err, errutil.ErrNotFound):
		return nil
	default:
		m.log.Error("failed to load pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)

		return nil
	}
}

// savePending records a confirmation the user has been asked and not
// yet answered.
func (m *Manager) savePending(ctx context.Context, key string, pending pendingConfirm) {
	if err := m.pendings.Set(ctx, createPendingKey(key), pending); err != nil {
		m.log.Error("failed to save pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// clearPending forgets an answered or abandoned confirmation.
func (m *Manager) clearPending(ctx context.Context, key string) {
	if err := m.pendings.Delete(ctx, createPendingKey(key)); err != nil {
		m.log.Error("failed to clear pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// createPendingKey builds the Redis key holding one conversation's
// outstanding confirmation.
func createPendingKey(sessionKey string) string {
	return sessionKey + ":pending"
}

// PendingStore is the persistence surface for outstanding write
// confirmations, keyed per (organisation, user). The redkit ValueStore
// satisfies it.
//
//go:generate ../../scripts/codegen/mock -t internal PendingStore pending_store
type PendingStore interface {
	// Get should return the stored confirmation for the key, or
	// errutil.ErrNotFound when none is outstanding.
	Get(ctx context.Context, key string) (*pendingConfirm, error)

	// Set should persist the confirmation under the key.
	Set(ctx context.Context, key string, value pendingConfirm) error

	// Delete should remove the stored confirmation for the key.
	Delete(ctx context.Context, key string) error
}
