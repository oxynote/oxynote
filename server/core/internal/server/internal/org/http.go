// Package org provides HTTP handlers for organization operations.
package org

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// _organizationsLogoFolderFormat is the folder where organization logos are stored.
const _organizationsLogoFolderFormat = "organizations/%s/logo"

// ErrNoOrganizationMembers is returned when an organization has no members.
var ErrNoOrganizationMembers = errutil.New(http.StatusBadRequest, "organization.no_members", "organization has no members")

// Handler holds dependencies required for organization-related operations.
type Handler struct {
	log               *slog.Logger
	db                DB
	storer            Storer
	logoLocation      string
	demoPrometheusURL string
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	logoLocationFormat string,
	demoPrometheusURL string,
) *Handler {
	return &Handler{
		log:               log,
		db:                db,
		storer:            storer,
		logoLocation:      logoLocationFormat,
		demoPrometheusURL: demoPrometheusURL,
	}
}

// InitializeOrganization prepares the organization for document management.
func (h *Handler) InitializeOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := h.extractOrganizationParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.demoPrometheusURL == "" {
		h.log.Warn("skipping initial demo data source insertion since no URL is configured")

		httpserver.Respond(
			h.log,
			w,
			nil,
			http.StatusOK,
		)

		return
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	members, err := tx.FetchOrganizationMembers(r.Context(), id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if len(members) == 0 {
		httpserver.RespondError(h.log, w, ErrNoOrganizationMembers)
		return
	}

	ds := datasource.NewDataSource(datasource.CreateInput{
		Type:        datasource.TypePrometheus,
		Name:        "Demo",
		URL:         h.demoPrometheusURL,
		Credentials: []byte(`{}`),
	}, id)

	if err = tx.InsertDataSource(r.Context(), ds); err != nil {
		// NOTE: We log the error instead of returning it to avoid failing the entire
		// initialization process due to a single data source insertion failure.
		h.log.Error("inserting demo data source", slog.String("error", err.Error()))
	}

	doc := document.NewDocument(document.CreateInput{
		Name: "Welcome to Oxynote!",
		Icon: "mingcute:flag-4-fill",
	}, id, members[0])

	content, err := document.InitialDocumentContent(ds.ID)
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

	if err = tx.InsertDocumentSearchJob(r.Context(), search.BlocksDiff(nil, doc.Search())); err != nil {
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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	file, _, err := r.FormFile("logo")
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
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
	logoLocation := h.logoLocation + "?v=" + time.Now().Format("20060102150405")

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
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	if match := r.Header.Get("If-None-Match"); match == obj.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Content-Type", obj.ContentType)

	_, err = io.Copy(w, obj.Body)
	if err != nil {
		h.log.Error("streaming organization logo", slog.String("error", err.Error()))
		return
	}
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
