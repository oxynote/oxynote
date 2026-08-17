package datasource

import (
	"net/http"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// QueryDataSource handles executing a query against a data source and returning
// a unified response format regardless of data source type.
func (h *Handler) QueryDataSource(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	ct := processor.ChartType(r.URL.Query().Get("chartType"))
	if !ct.IsValid() {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "chart_type.invalid", "Invalid chart type. Must be one of: line, bar, gauge."))
		return
	}

	switch ds.Type {
	case datasourceCore.TypePrometheus:
		h.queryPrometheusGeneric(w, r, ds, query, ct)
	case datasourceCore.TypePostgreSQL:
		h.queryPostgreSQLGeneric(w, r, ds, query, ct)
	case datasourceCore.TypeMariaDB, datasourceCore.TypeMySQL:
		h.queryMySQLGeneric(w, r, ds, query, ct)
	default:
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "data_source.type_not_supported", "Generic query is not supported for this data source type."))
	}
}

// queryPrometheusGeneric executes a Prometheus query and returns a unified QueryResult.
func (h *Handler) queryPrometheusGeneric(w http.ResponseWriter, r *http.Request, ds *datasourceCore.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.executor.PrometheusQuery(r.Context(), *ds, query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !h.syncDataSourceStatus(w, r, ds, status) {
		return
	}

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}

// queryMySQLGeneric executes a MySQL query and returns a unified QueryResult.
func (h *Handler) queryMySQLGeneric(w http.ResponseWriter, r *http.Request, ds *datasourceCore.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.executor.MySQLQuery(r.Context(), *ds, query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !h.syncDataSourceStatus(w, r, ds, status) {
		return
	}

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}

// queryPostgreSQLGeneric executes a PostgreSQL query and returns a unified QueryResult.
func (h *Handler) queryPostgreSQLGeneric(w http.ResponseWriter, r *http.Request, ds *datasourceCore.DataSource, query string, ct processor.ChartType) {
	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, result, err := h.executor.PostgreSQLQuery(r.Context(), *ds, query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !h.syncDataSourceStatus(w, r, ds, status) {
		return
	}

	if result == nil {
		httpserver.Respond(h.log, w, &processor.QueryResult{Status: processor.QueryStatusNoData}, http.StatusOK)
		return
	}

	httpserver.Respond(h.log, w, result.Transform(ct), http.StatusOK)
}
