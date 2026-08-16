package github

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testSigningSecret is a 32-byte AES key for installation state tests.
var _testSigningSecret = strings.Repeat("k", 32)

// newTestManager creates a configured Manager backed by the test key pair
// and signing secret.
func newTestManager(t *testing.T, db DB) *Manager {
	t.Helper()

	man, err := NewManager(db, Options{
		AppID:                     123,
		AppSlug:                   "test-app",
		SignatureSecret:           "sig",
		PrivateKeyPath:            "testdata/test-key.pem",
		InstallationSigningSecret: _testSigningSecret,
	})
	require.NoError(t, err)

	return man
}

// encryptState encrypts the given payload as an installation state string.
func encryptState(t *testing.T, secret string, payload any) string {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	state, err := cryptoutil.EncryptText(string(data), []byte(secret))
	require.NoError(t, err)

	return state
}

func Test_Manager_CreateInstallationURL(t *testing.T) {
	t.Parallel()

	t.Run("Unconfigured manager fails", func(t *testing.T) {
		t.Parallel()

		man, err := NewManager(nil, Options{})
		require.NoError(t, err)

		_, err = man.CreateInstallationURL("org")
		assert.ErrorIs(t, err, ErrNotConfigured)
	})

	t.Run("Configured manager produces a URL with an encrypted state", func(t *testing.T) {
		t.Parallel()

		rawURL, err := newTestManager(t, nil).CreateInstallationURL("org-1")
		require.NoError(t, err)

		u, err := url.Parse(rawURL)
		require.NoError(t, err)

		assert.Equal(t, "https", u.Scheme)
		assert.Equal(t, "github.com", u.Host)
		assert.Equal(t, "/apps/test-app/installations/new", u.Path)

		decrypted, err := cryptoutil.DecryptText(u.Query().Get("state"), []byte(_testSigningSecret))
		require.NoError(t, err)

		var is InstallationState

		require.NoError(t, json.Unmarshal([]byte(decrypted), &is))
		assert.Equal(t, "org-1", is.OrganizationID)
		assert.WithinDuration(t, time.Now(), is.CreatedAt, time.Minute)
	})
}

func Test_Manager_VerifyInstallationState(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Configured  bool
		State       func(t *testing.T) string
		ExpectedErr string
		ExpectedOrg string
	}{
		"Unconfigured manager fails": {
			State: func(*testing.T) string {
				return "state"
			},
			ExpectedErr: ErrNotConfigured.Error(),
		},
		"Fresh state returns the organization ID": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, _testSigningSecret, InstallationState{
					OrganizationID: "org-1",
					CreatedAt:      time.Now(),
				})
			},
			ExpectedOrg: "org-1",
		},
		"Expired state fails": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, _testSigningSecret, InstallationState{
					OrganizationID: "org-1",
					CreatedAt:      time.Now().Add(-_installationStateTTL - time.Minute),
				})
			},
			ExpectedErr: "installation state has expired",
		},
		"Garbage state fails decryption": {
			Configured: true,
			State: func(*testing.T) string {
				return "not-a-state"
			},
			ExpectedErr: "failed to decrypt installation state",
		},
		"State encrypted with another key fails decryption": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, strings.Repeat("x", 32), InstallationState{
					OrganizationID: "org-1",
					CreatedAt:      time.Now(),
				})
			},
			ExpectedErr: "failed to decrypt installation state",
		},
		"Non-JSON state payload fails unmarshalling": {
			Configured: true,
			State: func(t *testing.T) string {
				state, err := cryptoutil.EncryptText("not json", []byte(_testSigningSecret))
				require.NoError(t, err)

				return state
			},
			ExpectedErr: "failed to unmarshal installation state",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil)

			if !tc.Configured {
				var err error

				man, err = NewManager(nil, Options{})
				require.NoError(t, err)
			}

			is, err := man.VerifyInstallationState(tc.State(t))

			if tc.ExpectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedOrg, is.OrganizationID)
		})
	}
}
