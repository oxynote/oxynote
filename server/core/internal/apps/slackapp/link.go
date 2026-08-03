package slackapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/oxynote/purse/util/cryptoutil"
	"github.com/oxynote/purse/util/errutil"
)

var (
	// ErrLinkStateExpired is returned when the link state has expired.
	ErrLinkStateExpired = errutil.New(http.StatusBadRequest, "slack.link_state_expired", "link state has expired")
)

// _linkStateTTL is the time-to-live for the link state.
const _linkStateTTL = time.Minute * 15

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

// CreateLinkURL generates a URL for linking a Slack user to an internal account.
func (m *Manager) CreateLinkURL(slackUserID, slackTeamID, organizationID string) (string, error) {
	data, err := json.Marshal(LinkState{
		SlackUserID:    slackUserID,
		TeamID:         slackTeamID,
		OrganizationID: organizationID,
		CreatedAt:      time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal link state: %w", err)
	}

	state, err := cryptoutil.EncryptText(string(data), []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt link state: %w", err)
	}

	u, err := url.Parse(m.opt.RedirectURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse link user URL: %w", err)
	}

	q := u.Query()
	q.Set("state", state)
	q.Set("user", "1")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyLinkState verifies and decrypts the link state from the given state string.
func (m *Manager) VerifyLinkState(state string) (*LinkState, error) {
	decrypted, err := cryptoutil.DecryptText(state, []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt link state: %w", err)
	}

	var ls LinkState

	if err := json.Unmarshal([]byte(decrypted), &ls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal link state: %w", err)
	}

	if time.Since(ls.CreatedAt) > _linkStateTTL {
		return nil, ErrLinkStateExpired
	}

	return &ls, nil
}
