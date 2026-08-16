// Package datasource provides HTTP handlers for data-source operations.
package datasource

import (
	"context"
	"log/slog"
	"net/http"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for data source operations.
type Handler struct {
	log      *slog.Logger
	db       DB
	executor Executor
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(log *slog.Logger, db DB) *Handler {
	return &Handler{
		log:      log,
		db:       db,
		executor: datasourceCore.NewExecutor(),
	}
}

// CreateDataSource handles the creation of a new data source.
func (h *Handler) CreateDataSource(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var input datasourceCore.CreateInput

	if err = httpserver.DecodeJSON(r, &input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds := datasourceCore.NewDataSource(input, session.ActiveOrganizationID)

	status, err := h.executor.TestConnection(r.Context(), *ds)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds.Status = status

	if err := ds.Status.Error(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.InsertDataSource(r.Context(), ds); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, ds, http.StatusCreated)
}

// FetchDataSource handles retrieving a single data source by ID.
func (h *Handler) FetchDataSource(w http.ResponseWriter, r *http.Request) {
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

	httpserver.Respond(h.log, w, ds, http.StatusOK)
}

// TestDataSourceConnection handles testing the connection of a data source.
func (h *Handler) TestDataSourceConnection(w http.ResponseWriter, r *http.Request) {
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

	prevStatus := ds.Status

	status, err := h.executor.TestConnection(r.Context(), *ds)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds.Status = status

	if prevStatus != ds.Status {
		if uerr := h.db.UpdateDataSource(r.Context(), ds); uerr != nil {
			h.log.Error("failed to update data source status", slog.Any("error", uerr))
		}
	}

	httpserver.Respond(
		h.log,
		w,
		struct {
			Status processor.ConnectionStatus `json:"status"`
		}{
			Status: ds.Status,
		},
		http.StatusOK,
	)
}

// FetchDataSources handles retrieving all data sources for an organization.
func (h *Handler) FetchDataSources(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	sources, err := h.db.FetchDataSources(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, sources, http.StatusOK)
}

// UpdateDataSource handles updating an existing data source.
func (h *Handler) UpdateDataSource(w http.ResponseWriter, r *http.Request) {
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

	var input datasourceCore.UpdateInput

	if err = httpserver.DecodeJSON(r, &input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = ds.ApplyUpdate(input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, err := h.executor.TestConnection(r.Context(), *ds)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds.Status = status

	if err := ds.Status.Error(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.UpdateDataSource(r.Context(), ds); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, ds, http.StatusOK)
}

// DeleteDataSource handles deleting a data source.
func (h *Handler) DeleteDataSource(w http.ResponseWriter, r *http.Request) {
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

	if err := h.db.DeleteDataSource(r.Context(), id, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// extractDataSourceID extracts the data source ID from the request parameters.
func (h *Handler) extractDataSourceID(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "dataSourceId")
}

// DB is an interface that handles communication with the data source database.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// InsertDataSource inserts a data source into the database.
	InsertDataSource(ctx context.Context, ds *datasourceCore.DataSource) error

	// FetchDataSource retrieves a data source by ID and organization ID.
	FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasourceCore.DataSource, error)

	// FetchDataSources retrieves all data sources for an organization.
	FetchDataSources(ctx context.Context, organizationID string) ([]datasourceCore.DataSource, error)

	// UpdateDataSource updates an existing data source in the database.
	UpdateDataSource(ctx context.Context, ds *datasourceCore.DataSource) error

	// DeleteDataSource removes a data source from the database.
	DeleteDataSource(ctx context.Context, id xid.ID, organizationID string) error
}

// Executor is an interface that runs data source operations.
//
//go:generate ../../../../scripts/codegen/mock -t internal Executor
type Executor interface {
	// TestConnection should test the connection to a data source.
	TestConnection(ctx context.Context, ds datasourceCore.DataSource) (processor.ConnectionStatus, error)

	// PrometheusQuery should execute a Prometheus query against a data source.
	PrometheusQuery(ctx context.Context, ds datasourceCore.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusQueryResult, error)

	// PrometheusMetadata should retrieve metadata from a Prometheus data
	// source.
	PrometheusMetadata(ctx context.Context, ds datasourceCore.DataSource) (processor.ConnectionStatus, *processor.PrometheusMetadataResult, error)

	// PrometheusLabelNames should retrieve label names from a Prometheus
	// data source.
	PrometheusLabelNames(ctx context.Context, ds datasourceCore.DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelNamesResult, error)

	// PrometheusLabelValues should retrieve label values for a specific
	// label from a Prometheus data source.
	PrometheusLabelValues(ctx context.Context, ds datasourceCore.DataSource, label string, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelValuesResult, error)

	// PrometheusSeries should retrieve series matching the given selectors
	// from a Prometheus data source.
	PrometheusSeries(ctx context.Context, ds datasourceCore.DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusSeriesResult, error)

	// MySQLQuery should execute a SQL query against a MySQL data source.
	MySQLQuery(ctx context.Context, ds datasourceCore.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.MySQLQueryResult, error)

	// PostgreSQLQuery should execute a SQL query against a PostgreSQL data
	// source.
	PostgreSQLQuery(ctx context.Context, ds datasourceCore.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PostgreSQLQueryResult, error)

	// SQLQueryLabels should execute a query with LIMIT 1 and return string
	// column names with example values.
	SQLQueryLabels(ctx context.Context, ds datasourceCore.DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, map[string]string, error)

	// SQLMetadata should retrieve all tables and their columns from a SQL
	// data source.
	SQLMetadata(ctx context.Context, ds datasourceCore.DataSource) (processor.ConnectionStatus, *processor.SQLMetadataResult, error)
}
