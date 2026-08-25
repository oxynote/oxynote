// Package slack provides HTTP handlers for managing Slack-related operations.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/apps"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/retryutil"
	"github.com/rs/xid"
	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

var (
	// ErrMissingCode is returned when the Slack code is missing in the request.
	ErrMissingCode = errutil.New(http.StatusBadRequest, "slack.missing_code", "missing code in request")

	// ErrMissingInstallationState is returned when the installation state is missing from the request.
	ErrMissingInstallationState = errutil.New(http.StatusBadRequest, "slack.missing_installation_state", "installation state is required")

	// ErrOrganizationAlreadyConnected is returned when the Slack organization is already connected.
	ErrOrganizationAlreadyConnected = errutil.New(http.StatusBadRequest, "slack.organization_already_connected", "organization already connected")

	// ErrOpeningView is returned when there is an error opening a Slack view.
	ErrOpeningView = errutil.New(http.StatusInternalServerError, "slack.opening_view", "error opening view")
)

// Handler holds dependencies required for Slack related operations.
type Handler struct {
	log    *slog.Logger
	client *http.Client
	db     DB
	man    *slack.Manager

	message struct {
		postCallback func(organizationID string, id xid.ID)
	}
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(log *slog.Logger, db DB, client *http.Client, man *slack.Manager) *Handler {
	return &Handler{
		log:    log,
		client: client,
		db:     db,
		man:    man,
	}
}

// RequireConfigured is a middleware that rejects requests with
// slack.ErrNotConfigured when the Slack App integration is not configured
// on this deployment.
func (h *Handler) RequireConfigured(next http.Handler) http.Handler {
	return apps.RequireConfigured(h.log, h.man.Configured, slack.ErrNotConfigured)(next)
}

// VerifySignature is a middleware that verifies the Slack request signature.
func (h *Handler) VerifySignature(next http.Handler) http.Handler {
	// respond with a bare 404 when the Slack App is not configured: no
	// Slack App can be pointing its callbacks at this deployment.
	guard := apps.RequireConfigured(h.log, h.man.Configured, errutil.NewPlain(http.StatusNotFound))

	return guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.man.VerifyMiddleware(r); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		next.ServeHTTP(w, r)
	}))
}

// InstallApp handles the installation of a Slack app by exchanging the
// code for an access token.
func (h *Handler) InstallApp(w http.ResponseWriter, r *http.Request) {
	if !h.man.Configured() {
		httpserver.RespondError(h.log, w, errutil.NewPlain(http.StatusNotFound))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpserver.RespondError(h.log, w, ErrMissingCode)
		return
	}

	appAccess, err := h.man.ExchangeCode(r.Context(), code)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.InsertSlackApp(r.Context(), slack.App{
		TeamID: appAccess.TeamID,
		Token:  appAccess.AccessToken,
	})
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	//nolint:gosec // the fixed slack:// scheme and OAuth-provided ids cannot redirect to an attacker origin
	http.Redirect(
		w,
		r,
		fmt.Sprintf("slack://app?team=%s&id=%s&tab=about", appAccess.TeamID, appAccess.AppID),
		http.StatusFound,
	)
}

// FetchInstall handles the fetching of the Slack installation URL.
func (h *Handler) FetchInstall(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = h.db.FetchSlackAppByOrganizationID(r.Context(), session.ActiveOrganizationID)

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

	url, err := h.man.CreateExternalInstallationURL(session.ActiveOrganizationID)
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

// CheckInstallation handles the check of the Slack installation URL.
func (h *Handler) CheckInstallation(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	connected, err := apps.Connected(r.Context(), h.man.Configured(), func(ctx context.Context) error {
		_, ferr := h.db.FetchSlackAppByOrganizationID(ctx, session.ActiveOrganizationID)

		return ferr
	})
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	apps.RespondCapability(h.log, w, h.man.Configured(), connected)
}

// ConnectOrganization handles the connection of a Slack organization to the application.
func (h *Handler) ConnectOrganization(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	rawInstallationState := r.URL.Query().Get("state")
	if rawInstallationState == "" {
		httpserver.RespondError(h.log, w, ErrMissingInstallationState)
		return
	}

	is, err := h.man.VerifyInstallationState(rawInstallationState)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if is.OrganizationID.Valid {
		h.connectExternalOrganization(w, r, session.ActiveOrganizationID, is)
		return
	}

	var app *slack.App

	rerr := retryutil.UntilFound(r.Context(), func(ctx context.Context) error {
		app, err = h.db.FetchSlackAppByTeamID(ctx, is.TeamID.String)

		return err
	})
	if rerr != nil {
		httpserver.RespondError(h.log, w, rerr)
		return
	}

	if app.OrganizationID.Valid {
		httpserver.RespondError(h.log, w, ErrOrganizationAlreadyConnected)
		return
	}

	err = h.db.UpdateSlackAppOrganizationID(
		r.Context(),
		is.TeamID.String,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// FetchMessages retrieves Slack messages.
func (h *Handler) FetchMessages(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	messages, err := h.db.FetchSlackMessages(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, messages, http.StatusOK)
}

// DisconnectOrganization handles the disconnection of a Slack organization from the application.
func (h *Handler) DisconnectOrganization(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = h.db.FetchSlackAppByOrganizationID(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.UnassignSlackAppOrganization(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// HandleEvent handles slack events.
func (h *Handler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidJSON)
		return
	}

	// Signature is verified by the middleware.
	payload, err := slackevents.ParseEvent(data, slackevents.OptionNoVerifyToken())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	switch payload.Type {
	case slackevents.URLVerification:
		event, ok := payload.Data.(*slackevents.EventsAPIURLVerificationEvent)
		if !ok {
			httpserver.RespondError(h.log, w, httpserver.ErrInvalidJSON)
			return
		}

		// slackevents.ChallengeResponse has no json tag and would
		// marshal the field as "Challenge", but Slack's URL
		// verification expects a lowercase "challenge" attribute.
		httpserver.Respond(
			h.log,
			w,
			struct {
				Challenge string `json:"challenge"`
			}{
				Challenge: event.Challenge,
			},
			http.StatusOK,
		)

		return
	case slackevents.CallbackEvent:
		switch event := payload.InnerEvent.Data.(type) {
		case *slackevents.ImOpenEvent:
			err := h.handleChannelOpenEvent(
				r.Context(),
				payload.TeamID,
				event.User,
				event.Channel,
			)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		case *slackevents.AppUninstalledEvent:
			err := h.db.DeleteSlackApp(r.Context(), payload.TeamID)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		}
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// HandleCommand handles slack interaction commands.
func (h *Handler) HandleCommand(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	var payload goslack.InteractionCallback

	err := json.Unmarshal([]byte(r.FormValue("payload")), &payload)
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidJSON)
		return
	}

	app, err := h.db.FetchSlackAppByTeamID(r.Context(), payload.Team.ID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client := goslack.New(app.Token)

	if !app.OrganizationID.Valid { //nolint:nestif // the branching is sequential and readable
		var user *goslack.User

		user, err = client.GetUserInfo(payload.User.ID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if !user.IsAdmin && !user.IsOwner {
			err = h.sendEphemeralResponse(
				r.Context(),
				payload.ResponseURL,
				"Oxynote is not connected to this Slack organization. Please ask an admin to connect it.",
			)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		} else {
			var url string

			url, err = h.man.CreateInternalInstallationURL(payload.Team.ID)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}

			_, err = client.OpenViewContext(
				r.Context(),
				payload.TriggerID,
				connectOrganizationModal(url),
			)
			if err != nil {
				httpserver.RespondError(h.log, w, ErrOpeningView)
				return
			}
		}

		httpserver.Respond(h.log, w, nil, http.StatusNoContent)

		return
	}

	switch payload.Type {
	case goslack.InteractionTypeMessageAction:
		_, err = client.OpenViewContext(
			r.Context(),
			payload.TriggerID,
			initialMessageSavingModal(payload.Message.Text),
		)
		if err != nil {
			httpserver.RespondError(h.log, w, ErrOpeningView)
			return
		}
	case goslack.InteractionTypeViewSubmission:
		msg := slack.NewMessage(
			app.OrganizationID.String,
			payload.View.State.Values["message_input_block"]["message_input_block_action"].Value,
		)

		if err := h.db.InsertSlackMessage(r.Context(), *msg); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if h.message.postCallback != nil {
			h.message.postCallback(msg.OrganizationID, msg.ID)
		}
	default:
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// sendEphemeralResponse sends an ephemeral response to a Slack interaction.
func (h *Handler) sendEphemeralResponse(ctx context.Context, responseURL, text string) error {
	body := struct {
		Text string `json:"text"`
	}{
		Text: text,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal response body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responseURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response: %s", resp.Status)
	}

	return nil
}

// DB is an interface that handles communication with the document database.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	AppDB
	UserLinkDB
}

// AppDB is an interface that handles communication with the slack app and
// message database.
type AppDB interface {
	// InsertSlackApp inserts a slack app into the database, keeping an
	// existing organization binding over the incoming one.
	InsertSlackApp(ctx context.Context, app slack.App) error

	// InsertSlackMessage inserts a slack message into the database.
	InsertSlackMessage(ctx context.Context, msg slack.Message) error

	// UpdateSlackAppOrganizationID assigns an unclaimed slack workspace to
	// an organization, returning errutil.ErrNotFound when it is already
	// claimed.
	UpdateSlackAppOrganizationID(ctx context.Context, teamID, organizationID string) error

	// FetchSlackAppByTeamID fetches the slack app for a given team id.
	FetchSlackAppByTeamID(ctx context.Context, teamID string) (*slack.App, error)

	// FetchSlackAppByOrganizationID retrieves the slack app for a given organization ID.
	FetchSlackAppByOrganizationID(ctx context.Context, organizationID string) (*slack.App, error)

	// FetchSlackMessages retrieves Slack messages for a given organization ID from the database.
	FetchSlackMessages(ctx context.Context, organizationID string) ([]slack.Message, error)

	// DeleteSlackApp deletes a slack app from the database.
	DeleteSlackApp(ctx context.Context, teamID string) error

	// UnassignSlackAppOrganization removes the organization association from a Slack app.
	UnassignSlackAppOrganization(ctx context.Context, organizationID string) error
}

// UserLinkDB is an interface that handles communication with the slack user
// link database.
type UserLinkDB interface {
	// InsertSlackUserLink inserts a Slack user link in the database.
	InsertSlackUserLink(ctx context.Context, link slack.UserLink) error

	// UpdateSlackUserLink updates an existing Slack user link in the database.
	UpdateSlackUserLink(ctx context.Context, link slack.UserLink) error

	// FetchSlackUserLink retrieves a Slack user link by Slack user ID and team ID.
	FetchSlackUserLink(ctx context.Context, slackUserID, teamID string) (*slack.UserLink, error)

	// FetchSlackUserLinkByUserID retrieves a Slack user link by user ID.
	FetchSlackUserLinkByUserID(ctx context.Context, userID, organizationID string) (*slack.UserLink, error)

	// DeleteSlackUserLink removes a Slack user link from the database.
	DeleteSlackUserLink(ctx context.Context, slackUserID, teamID string) error

	// FetchOrganizationMembers retrieves the members of an organization.
	FetchOrganizationMembers(ctx context.Context, organizationID string) ([]string, error)
}

// connectExternalOrganization completes the externally initiated install
// flow, exchanging the OAuth code and claiming the workspace for the
// organization named in the installation state.
func (h *Handler) connectExternalOrganization(
	w http.ResponseWriter,
	r *http.Request,
	organizationID string,
	is *slack.InstallationState,
) {
	if is.OrganizationID.String != organizationID {
		httpserver.RespondError(h.log, w, ErrMissingInstallationState)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpserver.RespondError(h.log, w, ErrMissingCode)
		return
	}

	appAccess, err := h.man.ExchangeCode(r.Context(), code)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the workspace may already belong to another organization; the
	// upsert would otherwise hand it over silently.
	existing, ferr := h.db.FetchSlackAppByTeamID(r.Context(), appAccess.TeamID)

	switch {
	case ferr == nil:
		if existing.OrganizationID.Valid &&
			existing.OrganizationID.String != is.OrganizationID.String {
			httpserver.RespondError(h.log, w, ErrOrganizationAlreadyConnected)

			return
		}
	case errutil.IsNotFound(ferr):
		// a first installation.
	default:
		httpserver.RespondError(h.log, w, ferr)

		return
	}

	if err = h.db.InsertSlackApp(r.Context(), slack.App{
		TeamID:         appAccess.TeamID,
		Token:          appAccess.AccessToken,
		OrganizationID: is.OrganizationID,
	}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
}
