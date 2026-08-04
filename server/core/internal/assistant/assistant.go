// Package assistant runs the AI chat that lets the user interact
// with documents in their organisation. The Manager holds shared
// dependencies (Anthropic client, DB, edit-RPC client, Redis
// session store, metrics); each connected user gets a Session that
// owns the per-turn agent loop.
package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gomodule/redigo/redis"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/internal/document/liveedit"
	"github.com/oxynote/purse/redkit"
	"github.com/oxynote/purse/util/errutil"
	"github.com/oxynote/purse/util/metricutil"
)

// _sessionExpiration is how long Redis retains the conversation
// history for an idle session. Sessions resume from this history
// on reconnect.
const _sessionExpiration = time.Hour * 24

// Manager holds dependencies shared across assistant sessions.
type Manager struct {
	log      *slog.Logger
	db       tools.DB
	search   tools.Searcher
	pool     *redkit.ValueStore[[]anthropic.MessageParam]
	client   *anthropic.Client
	liveedit *liveedit.Client
	tree     tools.TreeNotifier
	metrics  *metrics
}

// NewManager constructs an assistant Manager. The liveEditClient is
// the live-edit pipe to the Node hocuspocus service; the search
// client backs the search_documents tool; the tree notifier
// broadcasts sidebar refresh events after assistant-driven document
// tree mutations and is wired post-construction via SetTreeNotifier
// because the dochandler that satisfies it is built later, inside
// server.NewServer.
func NewManager(
	log *slog.Logger,
	db tools.DB,
	pool *redis.Pool,
	apiKey string,
	fc metricutil.Factory,
	liveEditClient *liveedit.Client,
	search tools.Searcher,
) *Manager {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &Manager{
		log:      log.With("component", "assistant"),
		db:       db,
		search:   search,
		pool:     redkit.NewValueStore[[]anthropic.MessageParam](pool, _sessionExpiration),
		client:   &client,
		liveedit: liveEditClient,
		metrics:  newMetrics(fc),
	}
}

// SetTreeNotifier wires the tree-change notifier the assistant uses
// to broadcast sidebar refresh events after document tree mutations.
// Call this once during startup, after the dochandler that satisfies
// tools.TreeNotifier is constructed. The setter is not safe for
// concurrent use with NewSession; wire it before serving traffic.
func (m *Manager) SetTreeNotifier(tree tools.TreeNotifier) {
	m.tree = tree
}

// NewSession creates a new assistant session bound to the given
// organisation and user. The session starts without an active
// document; the client sends a set_active_document message right
// after connect to tell the model which document is in view.
func (m *Manager) NewSession(
	ctx context.Context,
	orgID, userID string,
	writer protocol.SessionWriter,
) (*Session, error) {
	msg, err := m.pool.Get(ctx, createSessionKey(orgID, userID))

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, errutil.ErrNotFound):
		msg = &[]anthropic.MessageParam{}
	default:
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}

	session := &Session{
		man:      m,
		orgID:    orgID,
		userID:   userID,
		writer:   writer,
		supv:     xync.NewSupervisor(),
		messages: *msg,
		tools:    tools.NewManager(m.log, m.db, m.search, m.liveedit, m.tree, orgID, userID),
	}

	session.sendHistory(ctx)

	return session, nil
}

// saveMessages persists the current session messages to Redis.
func (m *Manager) saveMessages(ctx context.Context, orgID, userID string, messages []anthropic.MessageParam) {
	err := m.pool.Set(ctx, createSessionKey(orgID, userID), messages)
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
	err := m.pool.Delete(ctx, createSessionKey(orgID, userID))
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
