// Package datasource provides HTTP handlers for data-source operations.
package datasource

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// ErrQueryRequired is returned when the request carries no query to run
// against the data source.
var ErrQueryRequired = errutil.New(http.StatusBadRequest, "query.required", "Query parameter is required.")

// _variableQueryPrefix heads a query parameter that gives the variable
// whose key follows it a value in place of the stored one.
const _variableQueryPrefix = "var-"

// Handler holds dependencies required for data source operations.
type Handler struct {
	log       *slog.Logger
	db        DB
	runners   Runners
	variables VariableManager
}

// NewHandler creates a new handler instance with the provided logger,
// database, runner manager and variable manager.
func NewHandler(log *slog.Logger, db DB, runners Runners, variables VariableManager) *Handler {
	return &Handler{
		log:       log,
		db:        db,
		runners:   runners,
		variables: variables,
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

	if err := input.Validate(); err != nil {
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

	res, rerr := newDataSourceResponse(ds)
	if rerr != nil {
		// NOCOV: the input was validated, so its credentials can be read.
		httpserver.RespondError(h.log, w, rerr)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusCreated)
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

	res, err := newDataSourceResponse(ds)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
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

	res := make([]dataSourceResponse, 0, len(sources))

	for i := range sources {
		item, rerr := newDataSourceResponse(&sources[i])
		if rerr != nil {
			httpserver.RespondError(h.log, w, rerr)
			return
		}

		res = append(res, item)
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
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

	h.runners.Evict(ds.ID)

	res, rerr := newDataSourceResponse(ds)
	if rerr != nil {
		httpserver.RespondError(h.log, w, rerr)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
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

	h.runners.Evict(id)

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

	// the kept runner remembers the test before this one. A source someone
	// just fixed would otherwise keep failing until that failure expires.
	h.runners.Evict(ds.ID)
}

// substitute returns the texts with the session user's variables
// substituted. A var-<key> query parameter on the request replaces the
// stored value of that variable. The tree is fetched only when a text
// references one.
func (h *Handler) substitute(r *http.Request, session auth.Session, texts ...string) ([]string, error) {
	if !variable.HasReference(texts...) {
		return texts, nil
	}

	vars, err := h.variables.FetchTree(r.Context(), session.ActiveOrganizationID, session.UserID)
	if err != nil {
		return nil, err
	}

	overrides := make(map[string]string)

	for name, values := range r.URL.Query() {
		if key, ok := strings.CutPrefix(name, _variableQueryPrefix); ok {
			overrides[key] = values[0]
		}
	}

	return vars.Substitute(overrides, texts...), nil
}

// dataSourceResponse is a data source as the API returns it: with which of
// its credentials are set, never their values.
type dataSourceResponse struct {
	datasourceCore.DataSource

	// Configured specifies the credentials that are set.
	Configured map[string]bool `json:"configured,omitempty"`
}

// newDataSourceResponse builds the API response for the data source.
func newDataSourceResponse(ds *datasourceCore.DataSource) (dataSourceResponse, error) {
	configured, err := ds.Credentials.Configured()
	if err != nil {
		return dataSourceResponse{}, fmt.Errorf("error reading configured credentials: %w", err)
	}

	return dataSourceResponse{
		DataSource: *ds,
		Configured: configured,
	}, nil
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

	// Evict should close the connections of the data source the id
	// names, so the next call connects with what is stored then.
	Evict(id xid.ID)
}

// VariableManager hands out the variables a query references.
//
//go:generate ../../../../scripts/codegen/mock -t internal VariableManager variable_manager
type VariableManager interface {
	// FetchTree should return the organization's variables, each with
	// the value the user set.
	FetchTree(ctx context.Context, organizationID, userID string) (variable.Tree, error)
}
