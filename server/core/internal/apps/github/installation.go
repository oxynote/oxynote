package github

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// githubHost is the GitHub host.
const _githubHost = "github.com"

// ErrInstallationStateExpired is returned when the installation state has
// expired. A user who leaves the install page open long enough is a client
// error, not a server fault worth paging anyone over.
var ErrInstallationStateExpired = errutil.New(http.StatusBadRequest, "github.installation_state_expired", "installation state has expired")

// ErrInvalidInstallationState is returned when the installation state cannot
// be decrypted or decoded, which means it was tampered with or truncated.
var ErrInvalidInstallationState = errutil.New(http.StatusBadRequest, "github.invalid_installation_state", "installation state is invalid")

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

// Created reports when the installation state was issued.
func (is InstallationState) Created() time.Time {
	return is.CreatedAt
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

	token, err := state.Encode(InstallationState{
		OrganizationID: organizationID,
		CreatedAt:      timeutil.Now(),
	}, m.opt.InstallationSigningSecret)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Add("state", token)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyInstallationState verifies and decrypts the installation state
// from the given state string.
func (m *Manager) VerifyInstallationState(token string) (*InstallationState, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	is, err := state.Decode[InstallationState](token, m.opt.InstallationSigningSecret, _installationStateTTL)
	if err != nil {
		if errors.Is(err, state.ErrExpired) {
			return nil, ErrInstallationStateExpired
		}

		return nil, ErrInvalidInstallationState
	}

	return &is, nil
}
