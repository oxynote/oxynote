package datasource

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// FetchSQLQueryLabels handles executing a query and returning string column labels with example values.
func (h *Handler) FetchSQLQueryLabels(w http.ResponseWriter, r *http.Request) {
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

	client, err := h.runners.Runner(*ds).SQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	labels, err := client.QueryLabels(r.Context(), query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if labels == nil {
		labels = make(map[string]string)
	}

	httpserver.Respond(h.log, w, struct {
		Labels map[string]string `json:"labels"`
	}{
		Labels: labels,
	}, http.StatusOK)
}

// FetchSQLMetadata handles retrieving all tables and columns from a SQL data source.
func (h *Handler) FetchSQLMetadata(w http.ResponseWriter, r *http.Request) {
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

	client, err := h.runners.Runner(*ds).SQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.Metadata(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}
