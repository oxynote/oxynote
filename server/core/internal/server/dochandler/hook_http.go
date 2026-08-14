package dochandler

import (
	"context"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// FetchDocumentHooks handles the retrieval of document hooks for a specific document branch.
// Requires a "branchId" query parameter to identify which branch's hooks to return.
func (h *Handler) FetchDocumentHooks(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := xid.FromString(r.URL.Query().Get("branchId"))
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var hi hook.CreateInput

	if err = httpserver.DecodeJSON(r, &hi); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if _, err = h.db.FetchDocumentByBranchID(r.Context(), hi.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := hook.NewHook(
		r.Context(),
		hi,
		documentID,
		hi.BranchID,
		session.ActiveOrganizationID,
		hook.NewInput(
			session.ActiveOrganizationID,
			h.githubMan,
			h.webchangesClient,
		),
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.InsertDocumentHook(r.Context(), *hk); err != nil {
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentHookParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ui hook.UpdateInput

	if err = httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = hk.ApplyUpdate(
		r.Context(),
		ui,
		hook.NewInput(
			session.ActiveOrganizationID,
			h.githubMan,
			h.webchangesClient,
		),
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.UpdateDocumentHook(r.Context(), *hk); err != nil {
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentHookParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = hk.Reset(r.Context(), hook.NewInput(
		session.ActiveOrganizationID,
		h.githubMan,
		h.webchangesClient,
	))
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.UpdateDocumentHook(r.Context(), *hk); err != nil {
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentHookParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := h.db.FetchDocumentHook(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = hk.Delete(r.Context(), hook.NewInput(
		session.ActiveOrganizationID,
		h.githubMan,
		h.webchangesClient,
	))
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.DeleteDocumentHook(r.Context(), hk.ID, session.ActiveOrganizationID); err != nil {
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

// extractDocumentHookParameter extracts the document hook ID from the request parameters.
func (h *Handler) extractDocumentHookParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "hookId")
}

// HooksDBAgent is an interface that handles communication with the document hooks database.
type HooksDBAgent interface {
	// FetchDocumentByBranchID should fetch the document joined against the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// InsertDocumentHook should insert the document hook.
	InsertDocumentHook(ctx context.Context, hk hook.Hook) error

	// FetchDocumentHook should fetch the document hook for the given id.
	FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hook.Hook, error)

	// FetchDocumentHooksByDocumentID should fetch all hooks for a document across all branches.
	// Used for full document cleanup (e.g. on document deletion).
	FetchDocumentHooksByDocumentID(ctx context.Context, documentID xid.ID, organizationID string) ([]hook.Hook, error)

	// FetchDocumentHooksByBranchID should fetch all hooks for a specific branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error)

	// UpdateDocumentHook should update the document hook.
	UpdateDocumentHook(ctx context.Context, hk hook.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID, organizationID string) error

	// SoftDeleteDocumentHooksByBranchID should mark all hooks for a branch as soft-deleted
	// so the hook manager job can handle external resource cleanup.
	SoftDeleteDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error
}
