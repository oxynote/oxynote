package datasource

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// FetchDataSourceLogs handles reading one page of the lines a log query
// matches in a window, newest first.
func (h *Handler) FetchDataSourceLogs(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		httpserver.RespondError(h.log, w, ErrQueryRequired)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	page, err := processor.ParseLogsPage(r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	qq, err := h.substitute(r, session, query)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Logs(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := client.Logs(r.Context(), qq[0], *tr, page)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}

// FetchDataSourceLogVolume handles counting the lines a log query matches
// per time bucket, split by level.
func (h *Handler) FetchDataSourceLogVolume(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		httpserver.RespondError(h.log, w, ErrQueryRequired)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	qq, err := h.substitute(r, session, query)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Logs(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := client.LogsVolume(r.Context(), qq[0], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}

// FetchDataSourceLogFields handles listing the field names the lines of a
// window carry, narrowed by an optional log query.
func (h *Handler) FetchDataSourceLogFields(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	qq, err := h.substitute(r, session, r.URL.Query().Get("q"))
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Logs(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := client.LogFields(r.Context(), qq[0], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}

// FetchDataSourceLogFieldValues handles listing the values one field takes
// in the lines of a window, narrowed by an optional log query.
func (h *Handler) FetchDataSourceLogFieldValues(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	field, err := httpserver.ExtractParam(r, "field")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	qq, err := h.substitute(r, session, r.URL.Query().Get("q"))
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Logs(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := client.LogFieldValues(r.Context(), field, qq[0], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}
