package persist

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// History stores the conversation a session resumes from.
type History struct {
	log *slog.Logger

	// store holds the conversations, expiring with the session.
	store HistoryStore
}

// NewHistory creates a fresh instance of History.
func NewHistory(log *slog.Logger, store HistoryStore) *History {
	return &History{log: log, store: store}
}

// Load restores a conversation, treating an unreadable one as empty: a
// broken history should cost the user their context, not their ability
// to chat.
func (h *History) Load(ctx context.Context, key string) []*schema.Message {
	msgs, err := h.store.Get(ctx, key)

	switch {
	case err == nil:
		if msgs == nil {
			return nil
		}

		return *msgs
	case errors.Is(err, errutil.ErrNotFound):
		return nil
	default:
		h.log.Error("failed to load conversation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)

		return nil
	}
}

// Save persists the conversation.
func (h *History) Save(ctx context.Context, key string, msgs []*schema.Message) {
	if err := h.store.Set(ctx, key, msgs); err != nil {
		h.log.Error("failed to save conversation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// Clear forgets a conversation.
func (h *History) Clear(ctx context.Context, key string) {
	if err := h.store.Delete(ctx, key); err != nil {
		h.log.Error("failed to delete conversation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// HistoryStore is the persistence surface for conversation history,
// keyed per (organisation, user). The redkit ValueStore satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both HistoryStore history_store
type HistoryStore interface {
	// Get should return the stored conversation for the key, or
	// errutil.ErrNotFound when none exists yet.
	Get(ctx context.Context, key string) (*[]*schema.Message, error)

	// Set should persist the conversation under the key.
	Set(ctx context.Context, key string, value []*schema.Message) error

	// Delete should remove the stored conversation for the key.
	Delete(ctx context.Context, key string) error
}
