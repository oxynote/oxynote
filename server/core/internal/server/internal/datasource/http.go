// Package datasource provides HTTP handlers for data-source operations.
package datasource

import (
	"context"
	"log/slog"
	"net/http"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errcode"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// ErrQueryRequired is returned when the request carries no query to run
// against the data source.
var ErrQueryRequired = errutil.New(http.StatusBadRequest, errcode.DataSourceQueryRequired, "Query parameter is required.")

// Handler holds dependencies required for data source operations.
type Handler struct {
	log     *slog.Logger
	db      DB
	runners Runners
}

// NewHandler creates a new handler instance with the provided logger,
// database and runner manager.
func NewHandler(log *slog.Logger, db DB, runners Runners) *Handler {
	return &Handler{
		log:     log,
		db:      db,
		runners: runners,
	}
}

// CreateDataSource handles the creation of a new data source.
func (h *Handler) CreateDataSource(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var input datasourceCore.CreateInput

	if err := httpserver.DecodeJSON(r, &input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds := datasourceCore.NewDataSource(input, session.ActiveOrganizationID)

	status, err := h.runners.Runner(*ds).TestConnection(r.Context())
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

	httpserver.Respond(h.log, w, ds, http.StatusOK)
}

// TestDataSourceConnection handles testing the connection of a data source.
func (h *Handler) TestDataSourceConnection(w http.ResponseWriter, r *http.Request) {
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

	status, err := h.runners.Runner(*ds).TestConnection(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.persistDataSourceStatus(r, ds, status)

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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
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

	status, err := h.runners.Runner(*ds).TestConnection(r.Context())
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
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

// persistDataSourceStatus stores the connection status observed by the last
// executor call whenever it differs from the recorded one. A failure to store
// it is logged rather than returned, since the status is an observation about
// the data source and not the answer the caller asked for.
func (h *Handler) persistDataSourceStatus(
	r *http.Request,
	ds *datasourceCore.DataSource,
	status processor.ConnectionStatus,
) {
	if status == ds.Status {
		return
	}

	ds.Status = status

	if uerr := h.db.UpdateDataSource(r.Context(), ds); uerr != nil {
		h.log.Error("cannot update data source status", slog.String("error", uerr.Error()))
	}
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

// Runners hands out the runner for a data source. The
// datasource package's Manager satisfies it.
//
//go:generate ../../../../scripts/codegen/mock -t internal Runners runners
type Runners interface {
	// Runner should return the runner that operates the given data
	// source.
	Runner(ds datasourceCore.DataSource) datasourceCore.Runner
}
