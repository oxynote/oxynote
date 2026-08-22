package persist

import (
	"context"
	"errors"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// PendingConfirm describes a confirmation awaiting the user's answer.
// It is stored rather than held on the session: the run it belongs to
// lives in a checkpoint, so the question outlives both the connection
// that asked it and this process.
type PendingConfirm struct {
	// TurnID correlates the request with the client's response.
	TurnID string `json:"turn_id"`

	// InterruptIDs addresses every write paused by this confirmation.
	InterruptIDs []string `json:"interrupt_ids"`

	// Actions describes each paused write for the user.
	Actions []protocol.ConfirmAction `json:"actions"`
}

// Pendings stores the confirmation a conversation is waiting on.
type Pendings struct {
	log *slog.Logger

	// store holds the outstanding confirmations, expiring with the
	// conversation.
	store PendingStore
}

// NewPendings creates a fresh instance of Pendings.
func NewPendings(log *slog.Logger, store PendingStore) *Pendings {
	return &Pendings{log: log, store: store}
}

// Load returns the confirmation this conversation is waiting on, or nil
// when there is none. An unreadable record is treated as absent: the
// turn stays parked rather than resuming writes nobody approved.
func (p *Pendings) Load(ctx context.Context, key string) *PendingConfirm {
	pending, err := p.store.Get(ctx, pendingKey(key))

	switch {
	case err == nil:
		return pending
	case errors.Is(err, errutil.ErrNotFound):
		return nil
	default:
		p.log.Error("failed to load pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)

		return nil
	}
}

// Save records a confirmation the user has been asked and not yet
// answered.
func (p *Pendings) Save(ctx context.Context, key string, pending PendingConfirm) {
	if err := p.store.Set(ctx, pendingKey(key), pending); err != nil {
		p.log.Error("failed to save pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// Clear forgets an answered or abandoned confirmation.
func (p *Pendings) Clear(ctx context.Context, key string) {
	if err := p.store.Delete(ctx, pendingKey(key)); err != nil {
		p.log.Error("failed to clear pending confirmation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// pendingKey builds the key holding one conversation's outstanding
// confirmation.
func pendingKey(sessionKey string) string {
	return sessionKey + ":pending"
}

// PendingStore is the persistence surface for outstanding write
// confirmations, keyed per (organisation, user). The redkit ValueStore
// satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both PendingStore pending_store
type PendingStore interface {
	// Get should return the stored confirmation for the key, or
	// errutil.ErrNotFound when none is outstanding.
	Get(ctx context.Context, key string) (*PendingConfirm, error)

	// Set should persist the confirmation under the key.
	Set(ctx context.Context, key string, value PendingConfirm) error

	// Delete should remove the stored confirmation for the key.
	Delete(ctx context.Context, key string) error
}
