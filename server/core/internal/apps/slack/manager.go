package slack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/cenkalti/backoff/v4"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	goslack "github.com/slack-go/slack"
)

var (
	// ErrAppNotFound is returned when a Slack app is not found.
	ErrAppNotFound = errutil.New(http.StatusNotFound, "slack.app_not_found", "slack app not found")

	// ErrAppNotConnected is returned when a Slack app is not connected to an organization.
	ErrAppNotConnected = errutil.New(http.StatusBadRequest, "slack.app_not_connected", "slack app is not connected to an organization")

	// ErrNotConfigured is returned when the Slack App integration is not configured on this deployment.
	ErrNotConfigured = errutil.New(http.StatusConflict, "slack.not_configured", "slack app is not configured")
)

// _maxRequestBodyBytes bounds a Slack request body read during signature
// verification.
const _maxRequestBodyBytes = 10 << 20

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

	// the secret is an AES key; a wrong-length one boots fine and then fails
	// every install, link and verify call with "invalid key size".
	if err := cryptoutil.ValidateKey(o.InstallationSigningSecret); err != nil {
		return fmt.Errorf("installation signing secret: %w", err)
	}

	return nil
}

// _maxNotificationRetries caps the retries of one Slack delivery.
const _maxNotificationRetries = 3

// _newBackoff creates the delivery retry strategy. Variable so tests can
// substitute a faster one.
var _newBackoff = func() backoff.BackOff {
	return backoff.NewExponentialBackOff()
}

// Manager represents a Slack App manager for handling Slack integrations.
type Manager struct {
	log     *slog.Logger
	db      DB
	int     Interpreter
	opt     Options
	cleanup func()
}

// NewManager creates a new Slack App manager with the given options. An empty
// client ID means the Slack App integration is not configured: the manager is
// still created, but Configured reports false, every method that talks to
// Slack returns ErrNotConfigured, and notifications are not forwarded to
// Slack.
func NewManager(
	log *slog.Logger,
	db DB,
	interp Interpreter,
	notifs notification.Receiver,
	opt Options,
) (*Manager, error) {
	m := &Manager{
		log: log.With("component", "slack-app-manager"),
		db:  db,
		int: interp,
		opt: opt,
	}

	if opt.ClientID == "" {
		return m, nil
	}

	if err := opt.Validate(); err != nil {
		return nil, err
	}

	m.cleanup = notifs.OnNotification(m.ProcessNotification)

	return m, nil
}

// Configured reports whether the Slack App integration is configured.
func (m *Manager) Configured() bool {
	return m.opt.ClientID != ""
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

	// the notification is retried on its way into the database, so a
	// transient Slack outage should not be the one hop that drops it. A
	// final failure is still swallowed: the notification exists either way
	// and the DM is the lesser copy of it.
	err = backoff.Retry(
		func() error {
			_, _, perr := client.PostMessageContext(
				ctx,
				ul.SlackUserID,
				goslack.MsgOptionText(msg.Text, false),
			)

			return perr
		},
		backoff.WithMaxRetries(
			backoff.WithContext(_newBackoff(), ctx),
			_maxNotificationRetries,
		),
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

// VerifyMiddleware verifies the Slack request signature.
func (m *Manager) VerifyMiddleware(r *http.Request) error {
	// an empty SignatureSecret would make the verifier accept signatures
	// computed over an empty key, so refuse outright rather than trusting
	// the route middleware to be the only gate.
	if !m.Configured() {
		return ErrNotConfigured
	}

	verifier, err := goslack.NewSecretsVerifier(r.Header, m.opt.SignatureSecret)
	if err != nil {
		return err
	}

	// the payloads verified here are form-encoded, so a read failure is a
	// transport problem rather than malformed JSON. The limit keeps the
	// internal surface bounded even when it is reached without the proxy.
	reqBody, err := io.ReadAll(io.LimitReader(r.Body, _maxRequestBodyBytes))
	if err != nil {
		return httpserver.ErrInvalidForm
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
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	resp, err := goslack.GetOAuthV2ResponseContext(
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
func (m *Manager) GetClient(ctx context.Context, teamID string) (*goslack.Client, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	app, err := m.db.FetchSlackAppByTeamID(ctx, teamID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, ErrAppNotFound
		}

		return nil, err
	}

	return goslack.New(app.Token), nil
}

// DB is an interface that handles communication with the database.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchSlackAppByTeamID fetches the slack app for a given team id.
	FetchSlackAppByTeamID(ctx context.Context, teamID string) (*App, error)

	// FetchSlackAppByOrganizationID retrieves the slack app for a given organization ID.
	FetchSlackAppByOrganizationID(ctx context.Context, organizationID string) (*App, error)

	// FetchSlackUserLinkByUserID fetches the slack user link for a given user id.
	FetchSlackUserLinkByUserID(ctx context.Context, userID, organizationID string) (*UserLink, error)
}

// Interpreter interprets notifications into human-readable messages.
//
//go:generate ../../../scripts/codegen/mock -t internal Interpreter
type Interpreter interface {
	// InterpretNotification interprets a notification and returns a human-readable message.
	InterpretNotification(ctx context.Context, n notification.Notification) (*interpreter.Message, error)
}
