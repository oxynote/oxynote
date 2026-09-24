// Package org provides HTTP handlers for organization operations.
package org

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	orgCore "github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// _defaultLogo is the Oxynote logo every organization starts with.
//
//go:embed default_logo.png
var _defaultLogo []byte

// ErrNoOrganizationMembers is returned when an organization has no members.
var ErrNoOrganizationMembers = errutil.New(http.StatusBadRequest, "organization.no_members", "organization has no members")

// Handler holds dependencies required for organization-related operations.
type Handler struct {
	log             *slog.Logger
	db              DB
	storer          Storer
	githubMan       *github.Manager
	webchangeClient *webchange.Client
	searchTrigger   SearchTrigger
	publicURL       string
}

// NewHandler creates a new handler instance with the provided logger and
// database. publicURL is the origin the Location of a logo is built on.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	searchTrigger SearchTrigger,
	publicURL string,
) *Handler {
	return &Handler{
		log:             log,
		db:              db,
		storer:          storer,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		searchTrigger:   searchTrigger,
		publicURL:       publicURL,
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

	// the default logo is a nicety like the demo content below, so a
	// failure is logged rather than returned.
	if _, err = h.storeLogo(r.Context(), id, _defaultLogo, "image/png"); err != nil {
		h.log.Error("cannot store default logo", slog.String("error", err.Error()))
	}

	// the demo data source is inserted before the transaction opens: a
	// failed statement aborts a Postgres transaction, so logging and
	// carrying on inside one would only move the failure to the commit.
	// The welcome document drops its charts when the source is missing
	// rather than pointing them at an id that was never stored. The
	// failure is logged rather than returned: the demo content is a
	// nicety, and an organization without it is still fully initialized.
	var dataSourceID null.Value[xid.ID]

	ds := datasource.NewDemoDataSource(id)

	if err = h.db.InsertDataSource(r.Context(), ds); err != nil {
		h.log.Error("inserting demo data source", slog.String("error", err.Error()))
	} else {
		dataSourceID = null.ValueFrom(ds.ID)
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	doc, err := document.NewWelcomeDocument(id, members[0], dataSourceID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.InsertDocument(r.Context(), doc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.UpsertDocumentMaintainers(r.Context(), doc.ID, id, []string{members[0]}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var productionID xid.ID

	for i, inp := range tag.SeedTags() {
		t := tag.NewTag(inp, id, members[0])

		if i == 0 {
			productionID = t.ID
		}

		if err = tx.InsertTag(r.Context(), t); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	if err = tx.AssignBranchTag(r.Context(), id, doc.ID, doc.BranchID, productionID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.InsertSearchJob(r.Context(), search.BranchScope(id, doc.ID, doc.BranchID)); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.searchTrigger.Trigger()

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

	file, err := httpserver.FormFile(w, r, "logo", storage.ImagePolicy.MaxUploadBytes(), storage.ErrSizeLimitExceeded)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer file.Close() //nolint:errcheck // error provides no meaningful info

	data, contentType, err := storage.ReadObject(file, storage.ImagePolicy)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	logoLocation, err := h.storeLogo(r.Context(), session.ActiveOrganizationID, data, contentType)
	if err != nil {
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

// storeLogo stores the organization's logo and points the organization at
// it, returning the logo's location. The object is removed again when the
// organization cannot be updated.
func (h *Handler) storeLogo(ctx context.Context, organizationID string, data []byte, contentType string) (string, error) {
	logoFolder := orgCore.LogoFolder(organizationID)

	if err := h.storer.Upload(ctx, logoFolder, organizationID, data, contentType); err != nil {
		return "", fmt.Errorf("uploading logo: %w", err)
	}

	logoLocation := httpserver.CacheBust(h.publicURL + orgCore.LogoPath)

	if err := h.db.UpdateOrganizationLogo(ctx, organizationID, logoLocation); err != nil {
		derr := h.storer.Delete(ctx, logoFolder, organizationID)
		if derr != nil {
			h.log.Error("deleting object after DB failure", slog.String("error", derr.Error()))
		}

		return "", fmt.Errorf("updating organization logo: %w", err)
	}

	return logoLocation, nil
}

// RetrieveOrganizationLogo handles the retrieval of an organization's logo.
func (h *Handler) RetrieveOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	logoFolder := orgCore.LogoFolder(session.ActiveOrganizationID)

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

	if err = tx.InsertSearchJob(r.Context(), search.OrganizationScope(id)); err != nil {
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

	h.searchTrigger.Trigger()

	// the logo is the organization's own object; the documents' files are
	// left to the file manager, which reclaims them once the cascade nulls
	// their foreign keys.
	err = h.storer.Delete(r.Context(), orgCore.LogoFolder(id), id)
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

// SearchTrigger runs the search-job worker once a job has committed.
//
//go:generate ../../../../scripts/codegen/mock -t internal SearchTrigger search_trigger
type SearchTrigger interface {
	// Trigger should run a search-job pass right away.
	Trigger()
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
	TagsDBAgent

	// InsertDataSource inserts a data source into the database.
	InsertDataSource(ctx context.Context, ds *datasource.DataSource) error

	// InsertDocument should insert the document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// InsertSearchJob should queue the search job's scope.
	InsertSearchJob(ctx context.Context, job search.Job) error

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

// TagsDBAgent is an interface that handles communication with the tag
// database, covering what seeding an organization needs.
type TagsDBAgent interface {
	// InsertTag should store a new tag at the end of its organization's tags.
	InsertTag(ctx context.Context, t tag.Tag) error

	// AssignBranchTag should make a document's branch carry a tag.
	AssignBranchTag(ctx context.Context, organizationID string, documentID, branchID, tagID xid.ID) error
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Upload should store the object's bytes under the given content
	// type.
	Upload(ctx context.Context, folder, id string, data []byte, contentType string) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)

	// Delete deletes an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
