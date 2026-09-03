// Package tag provides HTTP handlers for managing document tags.
package tag

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	tagCore "github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for tag operations.
type Handler struct {
	log *slog.Logger
	db  DB

	tree struct {
		changeCallback     func(organizationID string)
		userChangeCallback func(organizationID, userID string)
	}
}

// NewHandler creates a new tag handling instance.
func NewHandler(log *slog.Logger, db DB) *Handler {
	return &Handler{
		log: log.With("component", "tag-handler"),
		db:  db,
	}
}

// FetchTagTree handles the retrieval of the organization's tag tree.
func (h *Handler) FetchTagTree(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	tree, err := h.db.FetchTagTree(r.Context(), session.ActiveOrganizationID, session.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, tree, http.StatusOK)
}

// UpdateTagTree handles moving a tag to another position in the tree.
func (h *Handler) UpdateTagTree(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var data tagCore.SwapInput

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tree, err := h.db.FetchTagTree(r.Context(), session.ActiveOrganizationID, session.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	swapped, err := tree.Swap(data.ID, data.SortIndex)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = h.db.UpdateTagTree(r.Context(), swapped, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyTreeChange(session.ActiveOrganizationID)

	httpserver.Respond(h.log, w, swapped, http.StatusOK)
}

// CreateTag handles the creation of a new tag.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var inp tagCore.CreateInput

	if err := httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := inp.Validate(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	t := tagCore.NewTag(inp, session.ActiveOrganizationID, session.UserID)

	if err := h.db.InsertTag(r.Context(), t); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyTreeChange(session.ActiveOrganizationID)

	httpserver.Respond(h.log, w, t, http.StatusCreated)
}

// SetTagVisibility handles whether the caller keeps a tag out of their own
// sidebar. Nobody else's tree changes, so only the caller is told.
func (h *Handler) SetTagVisibility(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	tagID, err := httpserver.ExtractNamedID(r, "tagId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp tagCore.VisibilityInput

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.SetTagVisibility(
		r.Context(),
		session.ActiveOrganizationID,
		session.UserID,
		tagID,
		inp,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyUserTreeChange(session.ActiveOrganizationID, session.UserID)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// DeleteTag handles the removal of a tag and every assignment of it.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	tagID, err := httpserver.ExtractNamedID(r, "tagId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = h.db.DeleteTag(r.Context(), tagID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyTreeChange(session.ActiveOrganizationID)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// AssignDocumentTag handles making the path document carry a tag.
func (h *Handler) AssignDocumentTag(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp tagCore.AssignInput

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.AssignDocumentTag(r.Context(), session.ActiveOrganizationID, documentID, inp.TagID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyTreeChange(session.ActiveOrganizationID)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// UnassignDocumentTag handles stopping the path document carrying a tag.
func (h *Handler) UnassignDocumentTag(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tagID, err := httpserver.ExtractNamedID(r, "tagId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.UnassignDocumentTag(r.Context(), session.ActiveOrganizationID, documentID, tagID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifyTreeChange(session.ActiveOrganizationID)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// DB is an interface that handles communication with the tag database.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchTagTree should fetch an organization's tags together with the
	// documents carrying each of them, in their display order. Each
	// summary's Hidden should reflect the given user's own preference.
	FetchTagTree(ctx context.Context, organizationID, userID string) (tagCore.Summaries, error)

	// UpdateTagTree should rewrite the display order of an organization's
	// tags to the order of the given tree.
	UpdateTagTree(ctx context.Context, tree tagCore.Summaries, organizationID string) error

	// InsertTag should store a new tag at the end of its organization's tags.
	InsertTag(ctx context.Context, t tagCore.Tag) error

	// SetTagVisibility should record whether one user keeps a tag out of
	// their sidebar, leaving every other user's view untouched.
	SetTagVisibility(
		ctx context.Context,
		organizationID, userID string,
		id xid.ID,
		inp tagCore.VisibilityInput,
	) error

	// DeleteTag should remove a tag and every assignment of it.
	DeleteTag(ctx context.Context, id xid.ID, organizationID string) error

	// AssignDocumentTag should make a document carry a tag. Assigning a tag
	// the document already carries should change nothing.
	AssignDocumentTag(ctx context.Context, organizationID string, documentID, tagID xid.ID) error

	// UnassignDocumentTag should stop a document carrying a tag. Removing a
	// tag the document does not carry should change nothing.
	UnassignDocumentTag(ctx context.Context, organizationID string, documentID, tagID xid.ID) error
}
