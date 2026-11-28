// Package activity provides HTTP handlers for branch activity logs.
package activity

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/document"
	activityCore "github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// ErrBranchMismatch is returned when the requested branch does not belong
// to the document identified by the request path.
var ErrBranchMismatch = errutil.New(http.StatusNotFound, "document.branch_mismatch", "branch does not belong to the document")

// Handler holds dependencies required for activity log operations.
type Handler struct {
	log *slog.Logger
	db  DB
}

// NewHandler creates a new handler instance with the provided logger and
// database.
func NewHandler(log *slog.Logger, db DB) *Handler {
	return &Handler{
		log: log,
		db:  db,
	}
}

// FetchDocumentActivity handles the listing of a page of a branch's
// activity log.
func (h *Handler) FetchDocumentActivity(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var qr activityCore.Query

	if err := httpserver.DecodeForm(r, &qr); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if doc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
		return
	}

	page, err := h.db.FetchDocumentActivityEntries(r.Context(), branchID, session.ActiveOrganizationID, qr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, page, http.StatusOK)
}

// DB is an interface that handles communication with the activity log
// database.
//
//go:generate ../../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentByBranchID should fetch the document joined against
	// the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentActivityEntries should fetch a page of the branch's
	// entries, newest first.
	FetchDocumentActivityEntries(
		ctx context.Context,
		branchID xid.ID,
		organizationID string,
		qr activityCore.Query,
	) (activityCore.Page, error)
}
