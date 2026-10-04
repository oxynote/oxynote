// Package hook provides HTTP handlers for document hooks.
package hook

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/document"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for document hook operations.
type Handler struct {
	log     *slog.Logger
	db      DB
	hookMan Manager

	hooks struct {
		changeCallback func(organizationID string, documentID, branchID xid.ID)
	}
}

// NewHandler creates a new handler instance with the provided logger,
// database and hook manager.
func NewHandler(log *slog.Logger, db DB, hookMan Manager) *Handler {
	return &Handler{
		log:     log,
		db:      db,
		hookMan: hookMan,
	}
}

// FetchDocumentHooks handles the retrieval of document hooks for a specific document branch.
// Requires a "branchId" query parameter to identify which branch's hooks to return.
func (h *Handler) FetchDocumentHooks(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractQueryID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if branchDoc.ID != documentID {
		httpserver.RespondError(h.log, w, hookCore.ErrBranchMismatch)
		return
	}

	hooks, err := h.db.FetchDocumentHooksByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		hooks,
		http.StatusOK,
	)
}

// CreateDocumentHook handles the creation of a new document hook.
func (h *Handler) CreateDocumentHook(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var hi hookCore.CreateInput

	if err = httpserver.DecodeJSON(r, &hi); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = hi.Type.Validate(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.hookMan.CreateHook(r.Context(), hi, documentID, session.ActiveOrganizationID, session.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		hk,
		http.StatusCreated,
	)
}

// UpdateDocumentHook handles the update of a document hook.
func (h *Handler) UpdateDocumentHook(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := httpserver.ExtractNamedID(r, "hookId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ui hookCore.UpdateInput

	if err = httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.hookMan.UpdateHook(r.Context(), id, documentID, session.ActiveOrganizationID, ui, session.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		hk,
		http.StatusOK,
	)
}

// ResetDocumentHook handles the reset of a document hook's state.
func (h *Handler) ResetDocumentHook(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := httpserver.ExtractNamedID(r, "hookId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.hookMan.ResetHook(r.Context(), id, documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		hk,
		http.StatusOK,
	)
}

// DeleteDocumentHook handles the deletion of a document hook by its ID.
func (h *Handler) DeleteDocumentHook(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := httpserver.ExtractNamedID(r, "hookId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = h.hookMan.DeleteHook(r.Context(), id, documentID, session.ActiveOrganizationID, session.UserID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusNoContent,
	)
}

// DB is an interface that handles communication with the document hooks
// database.
//
//go:generate ../../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentByBranchID should fetch the document joined against the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentHooksByBranchID should fetch all hooks for a specific branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hookCore.Hook, error)
}

// Manager runs hook writes.
//
//go:generate ../../../../../scripts/codegen/mock -t internal Manager manager
type Manager interface {
	// CreateHook should create the hook on the branch of the document,
	// credited to updatedBy.
	CreateHook(ctx context.Context, ci hookCore.CreateInput, documentID xid.ID, organizationID, updatedBy string) (*hookCore.Hook, error)

	// UpdateHook should replace the settings of the document's hook,
	// credited to updatedBy, and return the stored hook.
	UpdateHook(ctx context.Context, id, documentID xid.ID, organizationID string, ui hookCore.UpdateInput, updatedBy string) (*hookCore.Hook, error)

	// DeleteHook should tear the document's hook down and remove it,
	// credited to updatedBy.
	DeleteHook(ctx context.Context, id, documentID xid.ID, organizationID, updatedBy string) error

	// ResetHook should reset the document's hook and return the stored
	// hook.
	ResetHook(ctx context.Context, id, documentID xid.ID, organizationID string) (*hookCore.Hook, error)
}
