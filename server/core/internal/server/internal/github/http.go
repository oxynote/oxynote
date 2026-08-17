// Package github provides HTTP handlers for managing Github-related operations.
package github

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	gogithub "github.com/google/go-github/v72/github"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/apps"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/retryutil"
)

var (
	// ErrMissingInstallationID is returned when the installation ID is missing from the request.
	ErrMissingInstallationID = errutil.New(http.StatusBadRequest, "github.missing_installation_id", "installation ID is required")

	// ErrMissingInstallationState is returned when the installation state is missing from the request.
	ErrMissingInstallationState = errutil.New(http.StatusBadRequest, "github.missing_installation_state", "installation state is required")

	// ErrOrganizationAlreadyConnected is returned when the organization is already connected.
	ErrOrganizationAlreadyConnected = errutil.New(http.StatusConflict, "github.organization_already_connected", "organization is already connected to a Github installation")

	// ErrCreatingClient is returned when there is an error creating the Github client.
	ErrCreatingClient = errutil.New(http.StatusInternalServerError, "github.creating_client", "failed to create Github client")

	// ErrResourceNotFound is returned when the requested resource is not found.
	ErrResourceNotFound = errutil.New(http.StatusNotFound, "github.resource_not_found", "resource not found")

	// ErrInvalidSignature is returned when a webhook fails signature verification.
	ErrInvalidSignature = errutil.New(http.StatusUnauthorized, "github.invalid_signature", "invalid webhook signature")

	// ErrInvalidPayload is returned when a webhook body cannot be parsed.
	ErrInvalidPayload = errutil.New(http.StatusBadRequest, "github.invalid_payload", "invalid webhook payload")
)

// _ctxKey is a type used for context keys to avoid collisions with other packages.
type _ctxKey string

// _ctxKeyPayload is the context key used to store the Github payload.
const _ctxKeyPayload _ctxKey = "github_payload"

// Handler holds dependencies required for Github related operations.
type Handler struct {
	log    *slog.Logger
	client *http.Client
	db     DB
	man    *github.Manager
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	client *http.Client,
	man *github.Manager,
) *Handler {
	return &Handler{
		log:    log,
		client: client,
		db:     db,
		man:    man,
	}
}

// RequireConfigured is a middleware that rejects requests with
// github.ErrNotConfigured when the GitHub App integration is not
// configured on this deployment.
func (h *Handler) RequireConfigured(next http.Handler) http.Handler {
	return apps.RequireConfigured(h.log, h.man.Configured, github.ErrNotConfigured)(next)
}

// VerifySignature is a middleware that verifies the Github request signature.
func (h *Handler) VerifySignature(next http.Handler) http.Handler {
	// respond with a bare 404 when the GitHub App is not configured: no
	// GitHub App can be pointing its webhooks at this deployment.
	guard := apps.RequireConfigured(h.log, h.man.Configured, errutil.NewPlain(http.StatusNotFound))

	return guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := gogithub.ValidatePayload(r, []byte(h.man.SignatureSecret()))
		if err != nil {
			httpserver.RespondError(h.log, w, ErrInvalidSignature)
			return
		}

		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(),
				_ctxKeyPayload,
				payload,
			),
		))
	}))
}

// FetchInstall handles the fetching of the Github installation URL.
func (h *Handler) FetchInstall(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = h.db.FetchGithubInstallationByOrganizationID(r.Context(), session.ActiveOrganizationID)

	switch {
	case errutil.IsNotFound(err):
		// OK.
	case err == nil:
		httpserver.RespondError(h.log, w, ErrOrganizationAlreadyConnected)
		return
	default:
		httpserver.RespondError(h.log, w, err)
		return
	}

	url, err := h.man.CreateInstallationURL(session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, struct {
		URL string `json:"url"`
	}{
		URL: url,
	}, http.StatusOK)
}

// CheckInstallation handles the check of the Github installation URL.
func (h *Handler) CheckInstallation(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	connected, err := apps.Connected(r.Context(), h.man.Configured(), func(ctx context.Context) error {
		_, ferr := h.db.FetchGithubInstallationByOrganizationID(ctx, session.ActiveOrganizationID)

		return ferr
	})
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	apps.RespondCapability(h.log, w, h.man.Configured(), connected)
}

// ConnectOrganization handles the connection of a Github organization to the application.
func (h *Handler) ConnectOrganization(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	rawInstallationID := r.URL.Query().Get("installation_id")
	if rawInstallationID == "" {
		httpserver.RespondError(h.log, w, ErrMissingInstallationID)
		return
	}

	rawInstallationState := r.URL.Query().Get("state")
	if rawInstallationState == "" {
		httpserver.RespondError(h.log, w, ErrMissingInstallationState)
		return
	}

	// TODO: Handle request to mark installation as pending (required approval).

	installationID, err := strconv.ParseInt(rawInstallationID, 10, 64)
	if err != nil {
		httpserver.RespondError(h.log, w, ErrMissingInstallationID)
		return
	}

	is, err := h.man.VerifyInstallationState(rawInstallationState)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if is.OrganizationID != session.ActiveOrganizationID {
		httpserver.RespondError(h.log, w, ErrMissingInstallationState)
		return
	}

	var organizationID null.String

	rerr := retryutil.UntilFound(r.Context(), func(ctx context.Context) error {
		organizationID, err = h.db.FetchGithubInstallationOrganizationID(ctx, installationID)

		return err
	})
	if rerr != nil {
		httpserver.RespondError(h.log, w, rerr)
		return
	}

	if organizationID.Valid {
		httpserver.RespondError(h.log, w, ErrOrganizationAlreadyConnected)
		return
	}

	err = h.db.UpdateGithubInstallationOrganizationID(r.Context(), installationID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// HandleEvent handles incoming Github events.
func (h *Handler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	payload, ok := r.Context().Value(_ctxKeyPayload).([]byte)
	if !ok || payload == nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidJSON)
		return
	}

	event, err := gogithub.ParseWebHook(gogithub.WebHookType(r), payload)
	if err != nil {
		httpserver.RespondError(h.log, w, ErrInvalidPayload)
		return
	}

	h.log.Debug("received github event", slog.Any("event", event))

	if event, ok := event.(*gogithub.InstallationEvent); ok {
		installationID := event.GetInstallation().GetID()

		switch event.GetAction() {
		case "created":
			err := h.db.InsertGithubInstallation(r.Context(), installationID)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		case "deleted":
			err := h.db.DeleteGithubInstallation(r.Context(), installationID)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		}
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// FetchRepositories retrieves Github repositories files.
func (h *Handler) FetchRepositories(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.man.GetInstallationClient(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	repos, err := client.FetchRepositories(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, repos, http.StatusOK)
}

// FetchRepositoryBranches retrieves Github repository branches.
func (h *Handler) FetchRepositoryBranches(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	repository, err := httpserver.ExtractParam(r, "repository")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.man.GetInstallationClient(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branches, err := client.FetchRepositoryBranches(r.Context(), repository)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, branches, http.StatusOK)
}

// FetchIssues retrieves Github issues.
func (h *Handler) FetchIssues(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var data struct {
		Q string `schema:"q"`

		// Repository is the repository to search issues.
		Repository string `schema:"repository"`
	}

	err = httpserver.DecodeForm(r, &data)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.man.GetInstallationClient(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	issues, err := client.FetchIssues(r.Context(), data.Q, data.Repository)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, issues, http.StatusOK)
}

// DisconnectOrganization handles the disconnection of a Github organization from the application.
func (h *Handler) DisconnectOrganization(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = h.db.FetchGithubInstallationByOrganizationID(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.UnassignGithubInstallationOrganization(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// FetchRepositoryTree retrieves Github repository files.
func (h *Handler) FetchRepositoryTree(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	repository, err := httpserver.ExtractParam(r, "repository")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var data struct {
		// Branch is the branch to search files.
		Branch string `schema:"branch"`
	}

	err = httpserver.DecodeForm(r, &data)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.man.GetInstallationClient(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tree, err := client.FetchRepositoryTree(r.Context(), repository, data.Branch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, tree, http.StatusOK)
}

// DB is an interface that handles communication with the document database.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	github.DB

	// InsertGithubInstallation inserts a new Github installation into the database.
	InsertGithubInstallation(ctx context.Context, installationID int64) error

	// FetchGithubInstallationOrganizationID retrieves the organization ID for a given Github installation.
	FetchGithubInstallationOrganizationID(ctx context.Context, installationID int64) (null.String, error)

	// UpdateGithubInstallationOrganizationID updates the organization ID for a given Github installation.
	UpdateGithubInstallationOrganizationID(ctx context.Context, installationID int64, organizationID string) error

	// DeleteGithubInstallation deletes a Github installation from the database.
	DeleteGithubInstallation(ctx context.Context, installationID int64) error

	// UnassignGithubInstallationOrganization removes the organization association from a Github installation.
	UnassignGithubInstallationOrganization(ctx context.Context, organizationID string) error
}
