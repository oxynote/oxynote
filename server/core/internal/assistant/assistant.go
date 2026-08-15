// Package assistant runs the AI chat that lets the user interact
// with documents in their organisation. The Manager holds shared
// dependencies (Anthropic client, DB, edit-RPC client, Redis
// session store, metrics); each connected user gets a session that
// owns the per-turn agent loop.
package assistant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gomodule/redigo/redis"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redkit"
)

// _sessionExpiration is how long Redis retains the conversation
// history for an idle session. Sessions resume from this history
// on reconnect.
const _sessionExpiration = time.Hour * 24

// Manager holds dependencies shared across assistant sessions.
type Manager struct {
	log     *slog.Logger
	db      tools.DB
	search  tools.Searcher
	store   SessionStore
	client  *anthropic.Client
	applier tools.EditApplier
	tree    tools.TreeNotifier
	metrics *metrics
}

// NewManager constructs an assistant Manager. The editClient is the
// edit pipe to the Node hocuspocus service; the search
// client backs the search_documents tool; the tree notifier
// broadcasts sidebar refresh events after assistant-driven document
// tree mutations and is wired post-construction via SetTreeNotifier
// because the document handler that satisfies it is built later, inside
// server.NewServer.
func NewManager(
	log *slog.Logger,
	db tools.DB,
	pool *redis.Pool,
	apiKey string,
	fc metricutil.Factory,
	editClient tools.EditApplier,
	search tools.Searcher,
) *Manager {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &Manager{
		log:     log.With("component", "assistant"),
		db:      db,
		search:  search,
		store:   redkit.NewValueStore[[]anthropic.MessageParam](pool, _sessionExpiration),
		client:  &client,
		applier: editClient,
		metrics: newMetrics(fc),
	}
}

// SetTreeNotifier wires the tree-change notifier the assistant uses
// to broadcast sidebar refresh events after document tree mutations.
// Call this once during startup, after the document handler that satisfies
// tools.TreeNotifier is constructed. The setter is not safe for
// concurrent use with Chat; wire it before serving traffic.
func (m *Manager) SetTreeNotifier(tree tools.TreeNotifier) {
	m.tree = tree
}

// Chat runs an assistant chat over the given connection for the given
// organisation and user. It owns the read loop: inbound messages are
// dispatched to a fresh session until the connection reports io.EOF (a
// clean client close, returning nil) or fails with any other error.
// The session starts without an active document; the client sends a
// set_active_document message right after connect to tell the model
// which document is in view.
func (m *Manager) Chat(ctx context.Context, orgID, userID string, conn protocol.SessionConn) error {
	s, err := m.newSession(ctx, orgID, userID, conn)
	if err != nil {
		return err
	}

	defer s.Close() //nolint:errcheck // Close never returns a meaningful error

	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("reading client message: %w", err)
		}

		s.Process(ctx, msg)
	}
}

// newSession creates a new chat session bound to the given
// organisation, user, and message writer.
func (m *Manager) newSession(
	ctx context.Context,
	orgID, userID string,
	writer protocol.SessionWriter,
) (*session, error) {
	msg, err := m.store.Get(ctx, createSessionKey(orgID, userID))

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, errutil.ErrNotFound):
		msg = &[]anthropic.MessageParam{}
	default:
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}

	s := &session{
		man:      m,
		orgID:    orgID,
		userID:   userID,
		writer:   writer,
		supv:     xync.NewSupervisor(),
		messages: *msg,
		tools:    tools.NewManager(m.log, m.db, m.search, m.applier, m.tree, orgID, userID),
	}

	s.sendHistory(ctx)

	return s, nil
}

// saveMessages persists the current session messages to Redis.
func (m *Manager) saveMessages(ctx context.Context, orgID, userID string, messages []anthropic.MessageParam) {
	err := m.store.Set(ctx, createSessionKey(orgID, userID), messages)
	if err != nil {
		m.log.Error("failed to save session messages",
			slog.String("error", err.Error()),
			slog.String("org_id", orgID),
			slog.String("user_id", userID),
		)
	}
}

// deleteMessages removes the session messages from Redis, called
// when the user issues a reset.
func (m *Manager) deleteMessages(ctx context.Context, orgID, userID string) {
	err := m.store.Delete(ctx, createSessionKey(orgID, userID))
	if err != nil {
		m.log.Error("failed to delete session messages",
			slog.String("error", err.Error()),
			slog.String("org_id", orgID),
			slog.String("user_id", userID),
		)
	}
}

// stripToolMessages returns a copy of the messages with tool
// interactions removed. Assistant messages have tool_use blocks
// stripped (keeping only text); user messages that contain only
// tool_result blocks are dropped entirely. This keeps history
// small and avoids serialising stale tool round-trips back to
// Anthropic on the next user turn.
func stripToolMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	cleaned := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case anthropic.MessageParamRoleAssistant:
			var textBlocks []anthropic.ContentBlockParamUnion

			for _, block := range msg.Content {
				if block.OfText != nil {
					textBlocks = append(textBlocks, block)
				}
			}

			if len(textBlocks) > 0 {
				cleaned = append(cleaned, anthropic.MessageParam{
					Role:    msg.Role,
					Content: textBlocks,
				})
			}

		case anthropic.MessageParamRoleUser:
			hasText := false

			for _, block := range msg.Content {
				if block.OfText != nil {
					hasText = true
					break
				}
			}

			if hasText {
				cleaned = append(cleaned, msg)
			}
		}
	}

	return cleaned
}

// createSessionKey builds the Redis key for an organisation/user
// pair's conversation history.
func createSessionKey(orgID, userID string) string {
	return fmt.Sprintf("assistant:session:%s:%s", orgID, userID)
}

// buildHistory converts Anthropic message params into protocol
// history entries by extracting text content from user and
// assistant messages. Tool interactions are excluded. Assistant
// messages that also contain a tool_use block are intermediate
// narration ("Let me look at the document…") and are skipped —
// only the final assistant reply is restored into the chat.
func buildHistory(messages []anthropic.MessageParam) []protocol.HistoryEntry {
	entries := make([]protocol.HistoryEntry, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant && hasToolUse(msg) {
			continue
		}

		var text strings.Builder

		for _, block := range msg.Content {
			if block.OfText != nil {
				text.WriteString(block.OfText.Text)
			}
		}

		content := text.String()
		if content == "" {
			continue
		}

		entries = append(entries, protocol.HistoryEntry{
			Role:    string(msg.Role),
			Content: content,
		})
	}

	return entries
}

// hasToolUse reports whether the assistant message carries at
// least one tool_use block — used to identify intermediate
// narration that shouldn't surface in the restored chat history.
func hasToolUse(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			return true
		}
	}

	return false
}

// SessionStore is the persistence surface for conversation history,
// keyed per (organisation, user). The redkit ValueStore satisfies
// it.
//
//go:generate ../../scripts/codegen/mock -t internal SessionStore session_store
type SessionStore interface {
	// Get should return the stored messages for the key, or
	// errutil.ErrNotFound when no history exists yet.
	Get(ctx context.Context, key string) (*[]anthropic.MessageParam, error)

	// Set should persist the messages under the key.
	Set(ctx context.Context, key string, value []anthropic.MessageParam) error

	// Delete should remove the stored messages for the key.
	Delete(ctx context.Context, key string) error
}
