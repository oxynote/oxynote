package slackapp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/purse/util/cryptoutil"
)

// _slackHost is the Slack host.
const _slackHost = "slack.com"

// _installationStateTTL is the time-to-live for the installation state.
const _installationStateTTL = time.Minute * 15

// botScopes are the OAuth scopes required for bot-level access.
var botScopes = []string{
	"app_mentions:read",
	"chat:write",
	"commands",
	"im:write",
	"users:read",
}

// userScopes are the OAuth scopes required for user-level access.
var userScopes = []string{
	"im:read",
}

// InstallationState represents the state of a Slack App installation.
type InstallationState struct {
	// TeamID specifies the team id which needs to be connected. It's
	// only present in case the installation is done internally.
	TeamID null.String `json:"teamId"`

	// OrganizationID is the ID of the organization
	// where the Slack App is installed.
	OrganizationID null.String `json:"organizationId"`

	// CreatedAt is the time when the installation was created.
	CreatedAt time.Time `json:"createdAt"`
}

// CreateExternalInstallationURL generates the Slack installation URL for the
// specified organization ID. This is used when installing Slack from the app.
func (m *Manager) CreateExternalInstallationURL(organizationID string) (string, error) {
	u := url.URL{
		Scheme: "https",
		Host:   _slackHost,
		Path:   "/oauth/v2/authorize",
	}

	data, err := json.Marshal(InstallationState{
		OrganizationID: null.StringFrom(organizationID),
		CreatedAt:      time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal installation state: %w", err)
	}

	state, err := cryptoutil.EncryptText(string(data), []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt installation state: %w", err)
	}

	q := u.Query()
	q.Set("client_id", m.opt.ClientID)
	q.Set("state", state)
	q.Set("scope", strings.Join(botScopes, ","))
	q.Set("user_scope", strings.Join(userScopes, ","))
	q.Set("redirect_uri", m.opt.RedirectURL)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// CreateInternalInstallationURL generates the Slack installation URL for the
// specified team ID. This is used when connecting from within Slack.
func (m *Manager) CreateInternalInstallationURL(teamID string) (string, error) {
	u, err := url.Parse(m.opt.RedirectURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse installation URL: %w", err)
	}

	data, err := json.Marshal(InstallationState{
		TeamID:    null.StringFrom(teamID),
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal installation state: %w", err)
	}

	state, err := cryptoutil.EncryptText(string(data), []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt installation state: %w", err)
	}

	q := u.Query()
	q.Set("state", state)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyInstallationState verifies and decrypts the installation state
// from the given state string.
func (m *Manager) VerifyInstallationState(state string) (*InstallationState, error) {
	decrypted, err := cryptoutil.DecryptText(state, []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt installation state: %w", err)
	}

	var is InstallationState

	if err := json.Unmarshal([]byte(decrypted), &is); err != nil {
		return nil, fmt.Errorf("failed to unmarshal installation state: %w", err)
	}

	if time.Since(is.CreatedAt) > _installationStateTTL {
		return nil, fmt.Errorf("installation state has expired")
	}

	return &is, nil
}
