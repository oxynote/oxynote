// Package hook provides HTTP handlers for document hooks.
package hook

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for document hook operations.
type Handler struct {
	log             *slog.Logger
	db              DB
	githubMan       *github.Manager
	webchangeClient *webchange.Client
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
) *Handler {
	return &Handler{
		log:             log,
		db:              db,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
	}
}

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

	var hi hookCore.CreateInput

	if err = httpserver.DecodeJSON(r, &hi); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = hi.Type.Validate(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if _, err = h.db.FetchDocumentByBranchID(r.Context(), hi.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hk, err := hookCore.NewHook(
		r.Context(),
		hi,
		documentID,
		hi.BranchID,
		session.ActiveOrganizationID,
		hookCore.NewInput(
			session.ActiveOrganizationID,
			h.githubMan,
			h.webchangeClient,
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

	var ui hookCore.UpdateInput

	if err = httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = hk.ApplyUpdate(
		r.Context(),
		ui,
		hookCore.NewInput(
			session.ActiveOrganizationID,
			h.githubMan,
			h.webchangeClient,
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

	err = hk.Reset(r.Context(), hookCore.NewInput(
		session.ActiveOrganizationID,
		h.githubMan,
		h.webchangeClient,
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

	err = hk.Delete(r.Context(), hookCore.NewInput(
		session.ActiveOrganizationID,
		h.githubMan,
		h.webchangeClient,
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

// extractDocumentParameter extracts the document ID from the request parameters.
func (h *Handler) extractDocumentParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "documentId")
}

// extractDocumentHookParameter extracts the document hook ID from the request parameters.
func (h *Handler) extractDocumentHookParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "hookId")
}

// DB is an interface that handles communication with the document hooks database.
//
//go:generate ../../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentByBranchID should fetch the document joined against the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// InsertDocumentHook should insert the document hook.
	InsertDocumentHook(ctx context.Context, hk hookCore.Hook) error

	// FetchDocumentHook should fetch the document hook for the given id.
	FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hookCore.Hook, error)

	// FetchDocumentHooksByBranchID should fetch all hooks for a specific branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hookCore.Hook, error)

	// UpdateDocumentHook should update the document hook.
	UpdateDocumentHook(ctx context.Context, hk hookCore.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID, organizationID string) error
}
