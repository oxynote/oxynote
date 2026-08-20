// Package assistant runs the AI chat that lets the user interact with
// documents in their organisation. The Manager holds shared
// dependencies (chat model, DB, edit-RPC client, Redis checkpoint and
// history stores, metrics); each connected user gets a session that
// drives one agent run per user message.
//
// No vendor SDK appears here: the model arrives as an eino interface
// built by the provider package, so the assistant behaves identically
// whichever provider the operator configured.
package assistant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redkit"
)

// _sessionExpiration is how long Redis retains an idle conversation.
// Conversations resume from this history, and a turn paused on a
// confirmation resumes from its checkpoint, for this long.
const _sessionExpiration = time.Hour * 24 * 7

// Manager holds dependencies shared across assistant sessions.
type Manager struct {
	log      *slog.Logger
	db       tools.DB
	search   tools.Searcher
	history  HistoryStore
	blobs    BlobStore
	pendings PendingStore
	model    model.ToolCallingChatModel
	summary  model.ToolCallingChatModel
	applier  tools.EditApplier
	tree     tools.TreeNotifier
	metrics  *metrics
}

// NewManager constructs an assistant Manager. The chatModel is built by
// the provider package from the operator's configuration; summaryModel
// backs context summarisation and may be the same model. The editClient
// is the edit pipe to the Node hocuspocus service; the search client
// backs the search_documents tool; providerName labels token metrics so
// usage stays readable across a provider change; the tree notifier broadcasts sidebar
// refresh events after assistant-driven document tree mutations and is
// wired post-construction via SetTreeNotifier because the document
// handler that satisfies it is built later, inside server.NewServer.
func NewManager(
	log *slog.Logger,
	db tools.DB,
	pool *redis.Pool,
	chatModel model.ToolCallingChatModel,
	summaryModel model.ToolCallingChatModel,
	fc metricutil.Factory,
	editClient tools.EditApplier,
	search tools.Searcher,
	providerName string,
) *Manager {
	if summaryModel == nil {
		summaryModel = chatModel
	}

	return &Manager{
		log:      log.With("component", "assistant"),
		db:       db,
		search:   search,
		history:  redkit.NewValueStore[[]*schema.Message](pool, _sessionExpiration),
		blobs:    redkit.NewBytesStore(pool, _sessionExpiration),
		pendings: redkit.NewValueStore[pendingConfirm](pool, _sessionExpiration),
		model:    chatModel,
		summary:  summaryModel,
		applier:  editClient,
		metrics:  newMetrics(fc, providerName),
	}
}

// SetTreeNotifier wires the tree-change notifier the assistant uses to
// broadcast sidebar refresh events after document tree mutations. Call
// this once during startup, after the document handler that satisfies
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

// createSessionKey builds the Redis key for an organisation/user pair's
// conversation. It doubles as the checkpoint id, so a paused turn and
// the history it belongs to are addressed the same way.
func createSessionKey(orgID, userID string) string {
	return fmt.Sprintf("assistant:session:%s:%s", orgID, userID)
}
