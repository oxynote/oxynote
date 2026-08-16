package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
)

// githubHost is the GitHub host.
const _githubHost = "github.com"

// _installationStateTTL is the time-to-live for the installation state.
const _installationStateTTL = time.Minute * 15

// InstallationState represents the state of a GitHub App installation.
type InstallationState struct {
	// OrganizationID is the ID of the organization
	// where the GitHub App is installed.
	OrganizationID string `json:"organizationId"`

	// CreatedAt is the time when the installation was created.
	CreatedAt time.Time `json:"createdAt"`
}

// CreateInstallationURL generates the GitHub App installation URL for the
// specified organization ID.
func (m *Manager) CreateInstallationURL(organizationID string) (string, error) {
	if !m.Configured() {
		return "", ErrNotConfigured
	}

	u := url.URL{
		Scheme: "https",
		Host:   _githubHost,
		Path:   "/apps/" + m.opt.AppSlug + "/installations/new",
	}

	data, err := json.Marshal(InstallationState{
		OrganizationID: organizationID,
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
	q.Add("state", state)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyInstallationState verifies and decrypts the installation state
// from the given state string.
func (m *Manager) VerifyInstallationState(state string) (*InstallationState, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	decrypted, err := cryptoutil.DecryptText(state, []byte(m.opt.InstallationSigningSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt installation state: %w", err)
	}

	var is InstallationState

	if err := json.Unmarshal([]byte(decrypted), &is); err != nil {
		return nil, fmt.Errorf("failed to unmarshal installation state: %w", err)
	}

	if time.Since(is.CreatedAt) > _installationStateTTL {
		return nil, errors.New("installation state has expired")
	}

	return &is, nil
}
