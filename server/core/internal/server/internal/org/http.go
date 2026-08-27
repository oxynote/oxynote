// Package org provides HTTP handlers for organization operations.
package org

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _organizationsLogoFolderFormat is the folder where organization logos are stored.
const _organizationsLogoFolderFormat = "organizations/%s/logo"

// ErrNoOrganizationMembers is returned when an organization has no members.
var ErrNoOrganizationMembers = errutil.New(http.StatusBadRequest, "organization.no_members", "organization has no members")

// Handler holds dependencies required for organization-related operations.
type Handler struct {
	log             *slog.Logger
	db              DB
	storer          Storer
	githubMan       *github.Manager
	webchangeClient *webchange.Client
	searchJobs      *search.Jobs
	logoLocation    string
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	searchJobs *search.Jobs,
	logoLocationFormat string,
) *Handler {
	return &Handler{
		log:             log,
		db:              db,
		storer:          storer,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		searchJobs:      searchJobs,
		logoLocation:    logoLocationFormat,
	}
}

// InitializeOrganization prepares the organization for document management.
func (h *Handler) InitializeOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := h.extractOrganizationParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	members, err := h.db.FetchOrganizationMembers(r.Context(), id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if len(members) == 0 {
		httpserver.RespondError(h.log, w, ErrNoOrganizationMembers)
		return
	}

	// the demo data source is inserted before the transaction opens: a
	// failed statement aborts a Postgres transaction, so logging and
	// carrying on inside one would only move the failure to the commit.
	// The welcome document drops its charts when the source is missing
	// rather than pointing them at an id that was never stored.
	dataSourceID := h.insertDemoDataSource(r.Context(), id)

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	doc := document.NewDocument(document.CreateInput{
		Name: "Welcome to Oxynote!",
		Icon: "mingcute:flag-4-fill",
	}, id, members[0])

	content, err := document.InitialDocumentContent(dataSourceID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc.Content = content

	if err = tx.InsertDocument(r.Context(), doc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.UpsertDocumentMaintainers(r.Context(), doc.ID, id, []string{members[0]}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDiff(nil, doc.Search())); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		doc,
		http.StatusCreated,
	)
}

// UploadOrganizationLogo handles the upload of an organization's logo.
func (h *Handler) UploadOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	file, err := httpserver.FormFile(w, r, "logo", storage.MaxUploadBytes, storage.ErrSizeLimitExceeded)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer file.Close() //nolint:errcheck // error provides no meaningful info

	logoFolder := fmt.Sprintf(_organizationsLogoFolderFormat, session.ActiveOrganizationID)

	err = h.storer.Upload(r.Context(), logoFolder, session.ActiveOrganizationID, file)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// Append a timestamp to the URL to prevent caching issues.
	logoLocation := h.logoLocation + "?v=" + timeutil.Now().Format("20060102150405")

	err = h.db.UpdateOrganizationLogo(r.Context(), session.ActiveOrganizationID, logoLocation)
	if err != nil {
		derr := h.storer.Delete(r.Context(), logoFolder, session.ActiveOrganizationID)
		if derr != nil {
			h.log.Error("deleting object after DB failure", slog.String("error", derr.Error()))
		}

		httpserver.RespondError(h.log, w, err)

		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusCreated,
		httpserver.LocationHeader(logoLocation),
	)
}

// RetrieveOrganizationLogo handles the retrieval of an organization's logo.
func (h *Handler) RetrieveOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	logoFolder := fmt.Sprintf(_organizationsLogoFolderFormat, session.ActiveOrganizationID)

	obj, found, err := h.storer.Retrieve(r.Context(), logoFolder, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !found {
		http.NotFound(w, r)
		return
	}

	defer obj.Body.Close() //nolint:errcheck // error provides no meaningful info

	httpserver.ServeObject(
		h.log,
		w,
		r,
		obj.ETag,
		obj.ContentType,
		obj.Body,
	)
}

// TeardownOrganization releases everything an organization owns outside of
// Postgres before the organization row itself is deleted. It runs while every
// row still exists, since the deletion cascades them away: hooks lose the
// state their external watchers are addressed by, and documents lose the ids
// their search entries are filtered by.
func (h *Handler) TeardownOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := h.extractOrganizationParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hooks, err := h.db.FetchDocumentHooksByOrganizationID(r.Context(), id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	inp := hook.NewInput(id, h.githubMan, h.webchangeClient)

	for _, hk := range hooks {
		if err = hk.Delete(r.Context(), inp); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDifference{
		RemovedOrganizations: []string{id},
	}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.DeleteSlackAppsByOrganizationID(r.Context(), id); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.DeleteGithubInstallationsByOrganizationID(r.Context(), id); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the logo is the organization's own object; the documents' files are
	// left to the file manager, which reclaims them once the cascade nulls
	// their foreign keys.
	err = h.storer.Delete(r.Context(), fmt.Sprintf(_organizationsLogoFolderFormat, id), id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusNoContent,
	)
}

// insertDemoDataSource inserts the demo Prometheus data source and reports
// its id. A failure is logged rather than returned: the demo content is a
// nicety, and an organization without it is still fully initialized.
func (h *Handler) insertDemoDataSource(ctx context.Context, organizationID string) null.Value[xid.ID] {
	ds := datasource.NewDataSource(datasource.CreateInput{
		Type:        datasource.TypePrometheus,
		Name:        "Demo",
		URL:         demo.URL,
		Credentials: processor.NewCredentials([]byte(`{}`)),
	}, organizationID)

	if err := h.db.InsertDataSource(ctx, ds); err != nil {
		h.log.Error("inserting demo data source", slog.String("error", err.Error()))

		return null.Value[xid.ID]{}
	}

	return null.ValueFrom(ds.ID)
}

// extractOrganizationParameter extracts the document ID from the request parameters.
func (h *Handler) extractOrganizationParameter(r *http.Request) (string, error) {
	return httpserver.ExtractParam(r, "organizationId")
}

// DB is an interface that combines sqlutil.DB and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx is an interface that combines sqlutil.Tx and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal Tx tx
type Tx interface {
	sqlutil.Tx
	DBAgent
}

// DBAgent is an interface that handles communication with the document database.
type DBAgent interface {
	// InsertDataSource inserts a data source into the database.
	InsertDataSource(ctx context.Context, ds *datasource.DataSource) error

	// InsertDocument should insert the document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// InsertDocumentSearchJob should insert the document search job.
	InsertDocumentSearchJob(ctx context.Context, diff search.BlocksDifference) error

	// FetchDocumentHooksByOrganizationID should return every hook of the
	// organization.
	FetchDocumentHooksByOrganizationID(ctx context.Context, organizationID string) ([]hook.Hook, error)

	// DeleteSlackAppsByOrganizationID should remove the organization's slack
	// apps together with the workspace tokens they hold.
	DeleteSlackAppsByOrganizationID(ctx context.Context, organizationID string) error

	// DeleteGithubInstallationsByOrganizationID should remove the
	// organization's github installations.
	DeleteGithubInstallationsByOrganizationID(ctx context.Context, organizationID string) error

	// UpdateOrganizationLogo should update the organization's logo URL.
	UpdateOrganizationLogo(ctx context.Context, organizationID, logo string) error

	// FetchOrganizationMembers should return all member user IDs for the organization.
	FetchOrganizationMembers(ctx context.Context, organizationID string) ([]string, error)

	// UpsertDocumentMaintainers should insert or update maintainers for a document.
	UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Upload uploads a new object.
	Upload(ctx context.Context, folder, id string, r io.Reader) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)

	// Delete deletes an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
