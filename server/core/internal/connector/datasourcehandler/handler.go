package datasourcehandler

import (
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/connector"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// Handler holds dependencies required for data source operations.
type Handler struct {
	log *slog.Logger
}

// NewHandler creates a new handler instance with the provided logger.
func NewHandler(log *slog.Logger) *Handler {
	return &Handler{
		log: log,
	}
}

// TestDataSourceConnection tests the connection to a data source via the connector.
func (h *Handler) TestDataSourceConnection(w http.ResponseWriter, r *http.Request) {
	var req connector.TestConnectionRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	cs, err := req.Runner.TestConnection(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, connector.Response{
		Status: cs,
	}, http.StatusOK)
}

// QueryPrometheusDataSource handles executing a query against a Prometheus data source.
func (h *Handler) QueryPrometheusDataSource(w http.ResponseWriter, r *http.Request) {
	var req connector.PrometheusQueryRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	prom, cs, err := req.Runner.Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := prom.QueryRange(r.Context(), req.Query, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// QueryPostgreSQLDataSource handles executing a SQL query against a PostgreSQL data source.
func (h *Handler) QueryPostgreSQLDataSource(w http.ResponseWriter, r *http.Request) {
	var req connector.PostgreSQLQueryRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	pg, cs, err := req.Runner.PostgreSQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := pg.Query(r.Context(), req.Query, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// QueryMySQLDataSource handles executing a SQL query against a MySQL data source.
func (h *Handler) QueryMySQLDataSource(w http.ResponseWriter, r *http.Request) {
	var req connector.MySQLQueryRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	mdb, cs, err := req.Runner.MySQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := mdb.Query(r.Context(), req.Query, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchSQLQueryLabels handles executing a query with LIMIT 1 and returning string column labels.
func (h *Handler) FetchSQLQueryLabels(w http.ResponseWriter, r *http.Request) {
	var req connector.SQLQueryLabelsRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	sqlDS, cs, err := req.Runner.SQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := sqlDS.QueryLabels(r.Context(), req.Query, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchSQLMetadata handles retrieving all tables and columns from a SQL data source.
func (h *Handler) FetchSQLMetadata(w http.ResponseWriter, r *http.Request) {
	var req connector.SQLMetadataRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	sql, cs, err := req.Runner.SQL(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := sql.Metadata(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchPrometheusDataSourceMetadata handles retrieving metadata from a Prometheus data source.
func (h *Handler) FetchPrometheusDataSourceMetadata(w http.ResponseWriter, r *http.Request) {
	var req connector.PrometheusMetadataRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	prom, cs, err := req.Runner.Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := prom.Metadata(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchPrometheusDataSourceLabelNames handles retrieving label names from a Prometheus data source.
func (h *Handler) FetchPrometheusDataSourceLabelNames(w http.ResponseWriter, r *http.Request) {
	var req connector.PrometheusLabelNamesRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	prom, cs, err := req.Runner.Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := prom.LabelNames(r.Context(), req.Matchers, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchPrometheusDataSourceLabelValues handles retrieving label values from a Prometheus data source.
func (h *Handler) FetchPrometheusDataSourceLabelValues(w http.ResponseWriter, r *http.Request) {
	var req connector.PrometheusLabelValuesRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	prom, cs, err := req.Runner.Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := prom.LabelValues(r.Context(), req.Label, req.Matchers, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}

// FetchPrometheusDataSourceSeries handles retrieving series from a Prometheus data source.
func (h *Handler) FetchPrometheusDataSourceSeries(w http.ResponseWriter, r *http.Request) {
	var req connector.PrometheusSeriesRequest

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	prom, cs, err := req.Runner.Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if cs != processor.ConnectionStatusSuccess {
		httpserver.Respond(h.log, w, connector.Response{
			Status: cs,
		}, http.StatusOK)

		return
	}

	result, err := prom.Series(r.Context(), req.Matchers, *req.TimeRange)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	resp, err := connector.NewResponse(cs, result)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, resp, http.StatusOK)
}
