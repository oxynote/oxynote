// Package assistant provides an HTTP/WebSocket handler for AI-assisted
// document editing over a WebSocket.
package assistant

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// _maxMessageBytes caps a single inbound WebSocket message (1 MB).
const _maxMessageBytes = 1 << 20

// Handler holds dependencies for AI chat operations.
type Handler struct {
	log        *slog.Logger
	assistant  Manager
	acceptOpts websocket.AcceptOptions
}

// NewHandler creates a new AI handler.
func NewHandler(
	log *slog.Logger,
	assistantMan Manager,
	acceptOpts websocket.AcceptOptions,
) *Handler {
	return &Handler{
		log:        log.With("component", "assistant-handler"),
		assistant:  assistantMan,
		acceptOpts: acceptOpts,
	}
}

// HandleChat upgrades the connection to WebSocket and hands it to the
// assistant, which owns the chat loop until the client disconnects.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	// refused before the upgrade so the client gets a readable status
	// and code instead of a failed websocket handshake.
	if !h.assistant.Configured() {
		httpserver.RespondError(h.log, w, assistantCore.ErrNotConfigured)
		return
	}

	sess, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	conn, err := websocket.Accept(w, r, &h.acceptOpts)
	if err != nil {
		h.log.Error("websocket accept failed", slog.String("error", err.Error()))
		return
	}

	conn.SetReadLimit(_maxMessageBytes)

	// the chat loop deliberately outlives the HTTP request lifecycle:
	// WithoutCancel keeps the session values while detaching from the
	// request's cancellation.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	wc := &wsConn{
		log:    h.log,
		conn:   conn,
		cancel: cancel,
	}

	err = h.assistant.Chat(ctx, sess.ActiveOrganizationID, sess.UserID, wc)
	if err != nil {
		h.log.Error("assistant chat failed", slog.String("error", err.Error()))

		conn.Close(websocket.StatusInternalError, "chat failed") //nolint:errcheck,gosec // error provides no meaningful info

		return
	}

	conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck,gosec // error provides no meaningful info
}

// wsConn adapts a WebSocket connection to the transport-agnostic
// protocol.SessionConn contract.
type wsConn struct {
	// log reports write failures.
	log *slog.Logger

	// cancel ends the chat context when the connection dies.
	cancel context.CancelFunc

	// mu serialises writes on the connection.
	mu   sync.Mutex
	conn *websocket.Conn
}

// Read blocks for the next client message, translating a clean client
// close into io.EOF.
func (w *wsConn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := w.conn.Read(ctx)
	if err != nil {
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
			websocket.CloseStatus(err) == websocket.StatusGoingAway ||
			errors.Is(err, io.EOF) {
			return nil, io.EOF
		}

		return nil, err
	}

	return data, nil
}

// WriteJSON sends a message to the client. It is safe for concurrent
// use; a failed write cancels the chat context to unblock the read
// loop.
func (w *wsConn) WriteJSON(ctx context.Context, msg any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := wsjson.Write(ctx, w.conn, msg)
	if err != nil {
		w.log.Error("failed to write message to client", slog.String("error", err.Error()))
		w.cancel()
	}
}

// Manager runs AI chat conversations for authenticated users.
//
//go:generate ../../../../scripts/codegen/mock -t internal Manager
type Manager interface {
	// Configured should report whether the assistant is configured on
	// this deployment.
	Configured() bool

	// Chat should run the chat loop over the connection until the
	// client disconnects, returning nil on a clean close.
	Chat(ctx context.Context, orgID, userID string, conn protocol.SessionConn) error
}
