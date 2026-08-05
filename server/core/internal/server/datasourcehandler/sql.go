package datasourcehandler

import (
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// FetchSQLQueryLabels handles executing a query and returning string column labels with example values.
func (h *Handler) FetchSQLQueryLabels(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDataSourceID(r)
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
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "query.required", "Query parameter is required."))
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, labels, err := h.executor.SQLQueryLabels(r.Context(), *ds, query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if status != ds.Status {
		ds.Status = status

		if uerr := h.db.UpdateDataSource(r.Context(), ds); uerr != nil {
			h.log.Error("failed to update data source status", slog.Any("error", uerr))
		}
	}

	if ds.Status != processor.ConnectionStatusSuccess {
		httpserver.RespondError(h.log, w, status.Error())
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDataSourceID(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.executor.SQLMetadata(r.Context(), *ds)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if status != ds.Status {
		ds.Status = status

		if uerr := h.db.UpdateDataSource(r.Context(), ds); uerr != nil {
			h.log.Error("failed to update data source status", slog.Any("error", uerr))
		}
	}

	if ds.Status != processor.ConnectionStatusSuccess {
		httpserver.RespondError(h.log, w, status.Error())
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}
