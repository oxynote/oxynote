package slack

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// ErrLinkStateExpired is returned when the link state has expired.
var ErrLinkStateExpired = errutil.New(http.StatusBadRequest, "slack.link_state_expired", "link state has expired")

// ErrInvalidLinkState is returned when the link state cannot be decrypted or
// decoded, which means it was tampered with or truncated.
var ErrInvalidLinkState = errutil.New(http.StatusBadRequest, "slack.invalid_link_state", "link state is invalid")

// _linkStateTTL is the time-to-live for the link state.
const _linkStateTTL = time.Minute * 15

// _linkStatePurpose binds link tokens to the linking flow; installation
// tokens are minted under the same signing secret and must not pass here.
const _linkStatePurpose = "slack-link"

// LinkState represents the state of a Slack user linking request.
type LinkState struct {
	// SlackUserID is the Slack user identifier.
	SlackUserID string `json:"slackUserId"`

	// TeamID is the Slack team identifier.
	TeamID string `json:"teamId"`

	// OrganizationID is the ID of the organization
	// where the Slack App is installed.
	OrganizationID string `json:"organizationId"`

	// CreatedAt is the time when the link request was created.
	CreatedAt time.Time `json:"createdAt"`
}

// Created reports when the link state was issued.
func (ls LinkState) Created() time.Time {
	return ls.CreatedAt
}

// CreateLinkURL generates a URL for linking a Slack user to an internal account.
func (m *Manager) CreateLinkURL(slackUserID, slackTeamID, organizationID string) (string, error) {
	if !m.Configured() {
		return "", ErrNotConfigured
	}

	token, err := state.Encode(LinkState{
		SlackUserID:    slackUserID,
		TeamID:         slackTeamID,
		OrganizationID: organizationID,
		CreatedAt:      timeutil.Now(),
	}, _linkStatePurpose, m.opt.InstallationSigningSecret)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(m.opt.RedirectURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse link user URL: %w", err)
	}

	q := u.Query()
	q.Set("state", token)
	q.Set("user", "1")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyLinkState verifies and decrypts the link state from the given state string.
func (m *Manager) VerifyLinkState(token string) (*LinkState, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	ls, err := state.Decode[LinkState](token, _linkStatePurpose, m.opt.InstallationSigningSecret, _linkStateTTL)
	if err != nil {
		if errors.Is(err, state.ErrExpired) {
			return nil, ErrLinkStateExpired
		}

		return nil, ErrInvalidLinkState
	}

	return &ls, nil
}
