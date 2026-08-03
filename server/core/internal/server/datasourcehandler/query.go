package datasourcehandler

import (
	"log/slog"
	"net/http"

	"github.com/oxynote/heimdall/internal/datasource"
	"github.com/oxynote/heimdall/internal/datasource/processor"
	"github.com/oxynote/heimdall/internal/server/auth"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/oxynote/purse/util/errutil"
)

// QueryDataSource handles executing a query against a data source and returning
// a unified response format regardless of data source type.
func (h *Handler) QueryDataSource(w http.ResponseWriter, r *http.Request) {
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

	ct := processor.ChartType(r.URL.Query().Get("chartType"))
	if !ct.IsValid() {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "chart_type.invalid", "Invalid chart type. Must be one of: line, bar, gauge."))
		return
	}

	switch ds.Type {
	case datasource.TypePrometheus:
		h.queryPrometheusGeneric(w, r, ds, query, ct)
	case datasource.TypePostgreSQL:
		h.queryPostgreSQLGeneric(w, r, ds, query, ct)
	case datasource.TypeMariaDB, datasource.TypeMySQL:
		h.queryMySQLGeneric(w, r, ds, query, ct)
	default:
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "data_source.type_not_supported", "Generic query is not supported for this data source type."))
	}
}

// queryPrometheusGeneric executes a Prometheus query and returns a unified QueryResult.
func (h *Handler) queryPrometheusGeneric(w http.ResponseWriter, r *http.Request, ds *datasource.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.connector.PrometheusQuery(r.Context(), *ds, query, *tr)
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

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}

// queryMySQLGeneric executes a MySQL query and returns a unified QueryResult.
func (h *Handler) queryMySQLGeneric(w http.ResponseWriter, r *http.Request, ds *datasource.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.connector.MySQLQuery(r.Context(), *ds, query, *tr)
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

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}

// queryPostgreSQLGeneric executes a PostgreSQL query and returns a unified QueryResult.
func (h *Handler) queryPostgreSQLGeneric(w http.ResponseWriter, r *http.Request, ds *datasource.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.connector.PostgreSQLQuery(r.Context(), *ds, query, *tr)
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

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}
