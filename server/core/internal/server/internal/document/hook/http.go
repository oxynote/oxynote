// Package hook provides HTTP handlers for document hooks.
package hook

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/document"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// ErrBranchMismatch is returned when the requested branch does not belong to
// the document identified by the request path.
var ErrBranchMismatch = errutil.New(http.StatusNotFound, "document.branch_mismatch", "branch does not belong to the document")

// ErrHookMismatch is returned when the requested hook does not belong to the
// document identified by the request path.
var ErrHookMismatch = errutil.New(http.StatusNotFound, "document.hook_mismatch", "hook does not belong to the document")

// ErrBlockNotFound is returned when a hook is to be anchored to a block the
// branch's content does not hold.
var ErrBlockNotFound = errutil.New(http.StatusNotFound, "document.hook_block_not_found", "block not found in the branch")

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
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), hi.BranchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// a hook addressed under one document but attached to another's branch
	// is missed by that document's cleanup and cites the wrong one in its
	// notifications.
	if branchDoc.ID != documentID {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
		return
	}

	// a hook anchored to a block the branch does not hold is invisible in
	// the editor and soft-deleted by the next sweep, so it is refused
	// rather than created to vanish.
	if hi.BlockID.Valid {
		if _, ok := branchDoc.Content.FindByUID(hi.BlockID.String); !ok {
			httpserver.RespondError(h.log, w, ErrBlockNotFound)
			return
		}
	}

	hk, err := h.hookMan.CreateHook(r.Context(), hi, branchDoc.ID, session.ActiveOrganizationID, session.UserID)
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

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// a row whose document was deleted carries no document id at all and is
	// not addressable through any document path.
	if !hk.DocumentID.Valid || hk.DocumentID.V != documentID {
		httpserver.RespondError(h.log, w, ErrHookMismatch)
		return
	}

	var ui hookCore.UpdateInput

	if err = httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err = h.hookMan.UpdateHook(r.Context(), hk.ID, session.ActiveOrganizationID, ui, session.UserID)
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

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// a row whose document was deleted carries no document id at all and is
	// not addressable through any document path.
	if !hk.DocumentID.Valid || hk.DocumentID.V != documentID {
		httpserver.RespondError(h.log, w, ErrHookMismatch)
		return
	}

	hk, err = h.hookMan.ResetHook(r.Context(), hk.ID, session.ActiveOrganizationID)
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

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// a row whose document was deleted carries no document id at all and is
	// not addressable through any document path.
	if !hk.DocumentID.Valid || hk.DocumentID.V != documentID {
		httpserver.RespondError(h.log, w, ErrHookMismatch)
		return
	}

	if err = h.hookMan.DeleteHook(r.Context(), hk.ID, session.ActiveOrganizationID, session.UserID); err != nil {
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

	// FetchDocumentHook should fetch the document hook for the given id.
	FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hookCore.Hook, error)

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

	// UpdateHook should replace the hook's settings, credited to
	// updatedBy, and return the stored hook.
	UpdateHook(ctx context.Context, id xid.ID, organizationID string, ui hookCore.UpdateInput, updatedBy string) (*hookCore.Hook, error)

	// DeleteHook should tear the hook down and remove it, credited to
	// updatedBy.
	DeleteHook(ctx context.Context, id xid.ID, organizationID, updatedBy string) error

	// ResetHook should restore the hook's score and state and return the
	// stored hook.
	ResetHook(ctx context.Context, id xid.ID, organizationID string) (*hookCore.Hook, error)
}
