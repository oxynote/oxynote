package datasourcehandler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/heimdall/internal/datasource"
	"github.com/oxynote/heimdall/internal/datasource/connector"
	"github.com/oxynote/heimdall/internal/datasource/processor"
	"github.com/oxynote/heimdall/internal/server/auth"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for data source operations.
type Handler struct {
	log       *slog.Logger
	db        DB
	connector *connector.Client
}

// NewHandler creates a new handler instance with the provided logger, database, and connector client.
func NewHandler(log *slog.Logger, db DB, connectorClient *connector.Client) *Handler {
	return &Handler{
		log:       log,
		db:        db,
		connector: connectorClient,
	}
}

// CreateDataSource handles the creation of a new data source.
func (h *Handler) CreateDataSource(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var input datasource.CreateInput

	if err := httpserver.DecodeJSON(r, &input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds := datasource.NewDataSource(input, session.ActiveOrganizationID)

	status, err := h.connector.TestConnection(r.Context(), *ds)
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

	status, err := h.connector.TestConnection(r.Context(), *ds)
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

	var input datasource.UpdateInput

	if err := httpserver.DecodeJSON(r, &input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := ds.ApplyUpdate(input); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	status, err := h.connector.TestConnection(r.Context(), *ds)
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
	raw, err := httpserver.ExtractParam(r, "dataSourceId")
	if err != nil {
		return xid.NilID(), err
	}

	id, err := xid.FromString(raw)
	if err != nil {
		return xid.NilID(), err
	}

	return id, nil
}

// DB is an interface that handles communication with the data source database.
type DB interface {
	// InsertDataSource inserts a data source into the database.
	InsertDataSource(ctx context.Context, ds *datasource.DataSource) error

	// FetchDataSource retrieves a data source by ID and organization ID.
	FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasource.DataSource, error)

	// FetchDataSources retrieves all data sources for an organization.
	FetchDataSources(ctx context.Context, organizationID string) ([]datasource.DataSource, error)

	// UpdateDataSource updates an existing data source in the database.
	UpdateDataSource(ctx context.Context, ds *datasource.DataSource) error

	// DeleteDataSource removes a data source from the database.
	DeleteDataSource(ctx context.Context, id xid.ID, organizationID string) error
}
