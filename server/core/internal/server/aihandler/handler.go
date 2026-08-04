// Package aihandler provides an HTTP/WebSocket handler for AI-assisted
// document editing via the Anthropic Messages API.
package aihandler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/purse/http/httpserver"
)

// Handler holds dependencies for AI chat operations.
type Handler struct {
	log        *slog.Logger
	assistant  *assistant.Manager
	acceptOpts websocket.AcceptOptions
}

// NewHandler creates a new AI handler.
func NewHandler(
	log *slog.Logger,
	assistant *assistant.Manager,
	acceptOpts websocket.AcceptOptions,
) *Handler {
	return &Handler{
		log:        log.With("component", "aihandler"),
		assistant:  assistant,
		acceptOpts: acceptOpts,
	}
}

// HandleChat upgrades the connection to WebSocket and runs the AI chat loop.
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
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

	conn.SetReadLimit(1 << 20) // 1 MB

	ctx, cancel := context.WithCancel(context.Background())

	wr := &writer{
		log:    h.log,
		conn:   conn,
		cancel: cancel,
	}

	// The client tells us which document the user is currently
	// viewing via a set_active_document message immediately after
	// connect (and again whenever they navigate). Keeping that out
	// of the URL means the WS doesn't get reopened on every doc
	// switch, so an in-flight turn survives the navigation.
	session, err := h.assistant.NewSession(
		ctx,
		sess.ActiveOrganizationID,
		sess.UserID,
		wr,
	)
	if err != nil {
		h.log.Error("failed to create assistant session", slog.String("error", err.Error()))
		cancel()

		conn.Close(websocket.StatusInternalError, "failed to create session")

		return
	}

	defer func() {
		cancel()
		session.Close()
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway ||
				errors.Is(err, io.EOF) {
				return
			}

			h.log.Error("reading websocket", slog.String("error", err.Error()))

			return
		}

		session.Process(ctx, data)
	}
}

// writer provides a concurrency-safe way to write messages to
// the WebSocket connection.
type writer struct {
	mu     sync.Mutex
	log    *slog.Logger
	conn   *websocket.Conn
	cancel context.CancelFunc
}

// WriteJSON sends a message to the client. It is safe for concurrent use.
func (w *writer) WriteJSON(ctx context.Context, msg any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := wsjson.Write(ctx, w.conn, msg)
	if err != nil {
		w.log.Error("failed to write message to client", slog.String("error", err.Error()))
		w.cancel()
	}
}
