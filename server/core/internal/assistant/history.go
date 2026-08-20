package assistant

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// loadMessages restores a conversation, treating an unreadable one as
// empty: a broken history should cost the user their context, not their
// ability to chat.
func (m *Manager) loadMessages(ctx context.Context, key string) []*schema.Message {
	msgs, err := m.history.Get(ctx, key)

	switch {
	case err == nil:
		if msgs == nil {
			return nil
		}

		return *msgs
	case errors.Is(err, errutil.ErrNotFound):
		return nil
	default:
		m.log.Error("failed to load conversation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)

		return nil
	}
}

// saveMessages persists the conversation.
func (m *Manager) saveMessages(ctx context.Context, key string, msgs []*schema.Message) {
	if err := m.history.Set(ctx, key, msgs); err != nil {
		m.log.Error("failed to save conversation",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// buildHistory converts the stored conversation into the entries the
// client restores into its chat pane.
//
// Only what a person said or was told is shown. Tool calls and their
// results stay in the model's context, where they let it remember what
// it already read and changed, but they are machinery rather than
// conversation. An assistant message accompanying a tool call is
// intermediate narration ("Let me look at that document…") and is
// skipped for the same reason.
func buildHistory(messages []*schema.Message) []protocol.HistoryEntry {
	entries := make([]protocol.HistoryEntry, 0, len(messages))

	for _, msg := range messages {
		if msg == nil || msg.Content == "" {
			continue
		}

		switch msg.Role {
		case schema.User:
			entries = append(entries, protocol.HistoryEntry{
				Role:    string(schema.User),
				Content: msg.Content,
			})
		case schema.Assistant:
			if len(msg.ToolCalls) > 0 {
				continue
			}

			entries = append(entries, protocol.HistoryEntry{
				Role:    string(schema.Assistant),
				Content: msg.Content,
			})
		case schema.System, schema.Tool:
			// not part of the visible conversation.
		}
	}

	return entries
}

// HistoryStore is the persistence surface for conversation history,
// keyed per (organisation, user). The redkit ValueStore satisfies it.
//
//go:generate ../../scripts/codegen/mock -t internal HistoryStore history_store
type HistoryStore interface {
	// Get should return the stored conversation for the key, or
	// errutil.ErrNotFound when none exists yet.
	Get(ctx context.Context, key string) (*[]*schema.Message, error)

	// Set should persist the conversation under the key.
	Set(ctx context.Context, key string, value []*schema.Message) error

	// Delete should remove the stored conversation for the key.
	Delete(ctx context.Context, key string) error
}
