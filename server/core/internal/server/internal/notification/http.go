// Package notification provides HTTP handlers for notification operations.
package notification

import (
	"context"
	"log/slog"
	"net/http"

	notificationCore "github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for notification operations.
type Handler struct {
	log      *slog.Logger
	db       DB
	notifier notificationCore.Receiver
}

// NewHandler creates a new notifications handling instance.
func NewHandler(
	log *slog.Logger,
	db DB,
	notifier notificationCore.Receiver,
) *Handler {
	return &Handler{
		log:      log.With("component", "notification-handler"),
		db:       db,
		notifier: notifier,
	}
}

// FetchManyNotifications fetches many notifications for the authenticated user.
func (h *Handler) FetchManyNotifications(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	qr, err := httpserver.ParseQuery(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	evts, pc, err := h.db.FetchManyNotifications(r.Context(), session.ActiveOrganizationID, session.UserID, qr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, struct {
		Notifications []*notificationCore.Notification `json:"notifications"`
		PageCount     uint64                           `json:"pageCount"`
	}{Notifications: evts, PageCount: pc}, http.StatusOK)
}

// FetchNotificationsCount fetches the count of notifications for the authenticated user.
func (h *Handler) FetchNotificationsCount(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var opts struct {
		Read bool `schema:"read"`
	}

	if err := httpserver.DecodeForm(r, &opts); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	count, err := h.db.FetchNotificationCount(r.Context(), session.ActiveOrganizationID, session.UserID, opts.Read)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, struct {
		Count uint64 `json:"count"`
	}{
		Count: count,
	}, http.StatusOK)
}

// MarkReadManyNotifications marks many notifications as read for the authenticated user.
func (h *Handler) MarkReadManyNotifications(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var data struct {
		IDs []xid.ID `json:"ids"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err := h.db.MarkReadByNotificationsIDs(r.Context(), session.ActiveOrganizationID, session.UserID, data.IDs)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// DB is an interface that handles communication with the notifications database.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchManyNotifications should fetch all notifications.
	FetchManyNotifications(ctx context.Context, organizationID, userID string, qr httpserver.Query) ([]*notificationCore.Notification, uint64, error)

	// FetchNotificationCount should fetch the total number of notifications.
	FetchNotificationCount(ctx context.Context, organizationID, userID string, read bool) (uint64, error)

	// MarkReadByNotificationsIDs should mark notifications as read by their IDs.
	// If no ids are provided, all notifications should be marked as read.
	MarkReadByNotificationsIDs(ctx context.Context, organizationID, userID string, ids []xid.ID) error
}
