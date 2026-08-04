package slackapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/slack-go/slack"
)

var (
	// ErrAppNotFound is returned when a Slack app is not found.
	ErrAppNotFound = errutil.New(http.StatusNotFound, "slack.app_not_found", "slack app not found")

	// ErrAppNotConnected is returned when a Slack app is not connected to an organization.
	ErrAppNotConnected = errutil.New(http.StatusBadRequest, "slack.app_not_connected", "slack app is not connected to an organization")
)

// Options holds configuration options for the Slack manager.
type Options struct {
	// RedirectURL is the URL to redirect users to connect their Slack organization.
	RedirectURL string

	// ClientID is the Slack client ID for OAuth.
	ClientID string

	// ClientSecret is the Slack client secret for OAuth.
	ClientSecret string

	// SignatureSecret is the secret used to verify Slack request signatures.
	SignatureSecret string

	// InstallationSigningSecret is the secret used to sign installation tokens.
	InstallationSigningSecret string
}

// Validate checks if the required options are set for the Slack manager.
func (o Options) Validate() error {
	if o.ClientID == "" {
		return errors.New("client id is required")
	}

	if o.ClientSecret == "" {
		return errors.New("client secret is required")
	}

	if o.SignatureSecret == "" {
		return errors.New("signature secret is required")
	}

	if o.RedirectURL == "" {
		return errors.New("redirect URL is required")
	}

	if o.InstallationSigningSecret == "" {
		return errors.New("installation signing secret is required")
	}

	return nil
}

// Manager represents a Slack App manager for handling Slack integrations.
type Manager struct {
	log     *slog.Logger
	db      DB
	int     Interpreter
	opt     Options
	cleanup func()
}

// NewManager creates a new Slack App manager with the given options.
func NewManager(
	log *slog.Logger,
	db DB,
	int Interpreter,
	notifs notification.NotificationReceiver,
	opt Options,
) (*Manager, error) {
	if err := opt.Validate(); err != nil {
		return nil, err
	}

	m := &Manager{
		log: log.With("component", "slack-app-manager"),
		db:  db,
		int: int,
		opt: opt,
	}

	m.cleanup = notifs.OnNotification(m.ProcessNotification)

	return m, nil
}

// Close stops and closes all processes.
func (m *Manager) Close() error {
	if m.cleanup != nil {
		m.cleanup()
	}

	return nil
}

// ProcessNotification processes and sends a notification to the appropriate Slack user.
func (m *Manager) ProcessNotification(ctx context.Context, n notification.Notification) {
	ul, err := m.db.FetchSlackUserLinkByUserID(ctx, n.UserID, n.OrganizationID)
	if err != nil {
		if !errutil.IsNotFound(err) {
			m.log.Error(
				"failed to fetch slack user link",
				slog.String("userID", n.UserID),
				slog.String("error", err.Error()),
			)
		}

		return
	}

	if !ul.Settings.Notifications {
		return
	}

	client, err := m.GetClient(ctx, ul.TeamID)
	if err != nil {
		m.log.Error(
			"failed to get slack client",
			slog.String("userID", n.UserID),
			slog.String("teamID", ul.TeamID),
			slog.String("error", err.Error()),
		)

		return
	}

	msg, err := m.int.InterpretNotification(ctx, n)
	if err != nil {
		m.log.Error(
			"failed to interpret notification",
			slog.String("userID", n.UserID),
			slog.String("error", err.Error()),
		)

		return
	}

	_, _, err = client.PostMessageContext(
		ctx,
		ul.SlackUserID,
		slack.MsgOptionText(msg.Text, false),
	)
	if err != nil {
		m.log.Error(
			"failed to send slack notification",
			slog.String("userID", n.UserID),
			slog.String("teamID", ul.TeamID),
			slog.String("error", err.Error()),
		)
	}
}

// SignatureSecret returns the signature secret for verifying Slack request signatures.
func (m *Manager) VerifyMiddleware(r *http.Request) error {
	verifier, err := slack.NewSecretsVerifier(r.Header, m.opt.SignatureSecret)
	if err != nil {
		return err
	}

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		return httpserver.ErrInvalidJSON
	}

	// The body is read and replaced to ensure that the request
	// can be processed later.
	r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

	if _, err = verifier.Write(reqBody); err != nil {
		return err
	}

	if err = verifier.Ensure(); err != nil {
		return err
	}

	return nil
}

// ExchangeCode exchanges an OAuth code for an app access data.
func (m *Manager) ExchangeCode(ctx context.Context, code string) (*AppAccess, error) {
	resp, err := slack.GetOAuthV2ResponseContext(
		ctx,
		http.DefaultClient,
		m.opt.ClientID,
		m.opt.ClientSecret,
		code,
		"",
	)
	if err != nil {
		return nil, err
	}

	return &AppAccess{
		TeamID:      resp.Team.ID,
		AccessToken: resp.AccessToken,
		AppID:       resp.AppID,
	}, nil
}

// GetClient creates a Slack client for the given team ID.
func (m *Manager) GetClient(ctx context.Context, teamID string) (*slack.Client, error) {
	app, err := m.db.FetchSlackAppByTeamID(ctx, teamID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, ErrAppNotFound
		}

		return nil, err
	}

	return slack.New(app.Token), nil
}

// DB is an interface that handles communication with the database.
type DB interface {
	// FetchSlackAppByTeamID fetches the slack app for a given team id.
	FetchSlackAppByTeamID(ctx context.Context, teamID string) (*App, error)

	// FetchSlackAppByOrganizationID retrieves the slack app for a given organization ID.
	FetchSlackAppByOrganizationID(ctx context.Context, organizationID string) (*App, error)

	// FetchSlackUserLinkByUserID fetches the slack user link for a given user id.
	FetchSlackUserLinkByUserID(ctx context.Context, userID, organizationID string) (*UserLink, error)
}

// Interpreter interprets notifications into human-readable messages.
type Interpreter interface {
	// InterpretNotification interprets a notification and returns a human-readable message.
	InterpretNotification(ctx context.Context, n notification.Notification) (*interpreter.Message, error)
}
