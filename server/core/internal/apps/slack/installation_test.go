package slack

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_CreateExternalInstallationURL(t *testing.T) {
	t.Parallel()

	t.Run("Unconfigured manager fails", func(t *testing.T) {
		t.Parallel()

		_, err := newDisabledManager(t).CreateExternalInstallationURL("org")
		assert.ErrorIs(t, err, ErrNotConfigured)
	})

	t.Run("Configured manager produces an authorize URL with an encrypted state", func(t *testing.T) {
		t.Parallel()

		rawURL, err := newTestManager(t, nil, nil).CreateExternalInstallationURL("org-1")
		require.NoError(t, err)

		u, err := url.Parse(rawURL)
		require.NoError(t, err)

		assert.Equal(t, "https", u.Scheme)
		assert.Equal(t, _slackHost, u.Host)
		assert.Equal(t, "/oauth/v2/authorize", u.Path)

		q := u.Query()
		assert.Equal(t, "id", q.Get("client_id"))
		assert.Equal(t, strings.Join(_botScopes, ","), q.Get("scope"))
		assert.Equal(t, strings.Join(_userScopes, ","), q.Get("user_scope"))
		assert.Equal(t, _testRedirectURL, q.Get("redirect_uri"))

		is := decryptInstallationState(t, q.Get("state"))
		assert.Equal(t, null.StringFrom("org-1"), is.OrganizationID)
		assert.False(t, is.TeamID.Valid)
		assert.WithinDuration(t, time.Now(), is.CreatedAt, time.Minute)
	})
}

func Test_Manager_CreateInternalInstallationURL(t *testing.T) {
	t.Parallel()

	t.Run("Unconfigured manager fails", func(t *testing.T) {
		t.Parallel()

		_, err := newDisabledManager(t).CreateInternalInstallationURL("team")
		assert.ErrorIs(t, err, ErrNotConfigured)
	})

	t.Run("Configured manager produces a redirect URL with an encrypted state", func(t *testing.T) {
		t.Parallel()

		rawURL, err := newTestManager(t, nil, nil).CreateInternalInstallationURL("team-1")
		require.NoError(t, err)

		u, err := url.Parse(rawURL)
		require.NoError(t, err)

		assert.Equal(t, _testRedirectURL, u.Scheme+"://"+u.Host+u.Path)

		is := decryptInstallationState(t, u.Query().Get("state"))
		assert.Equal(t, null.StringFrom("team-1"), is.TeamID)
		assert.False(t, is.OrganizationID.Valid)
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
		"Fresh state returns the installation state": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, InstallationState{
					OrganizationID: null.StringFrom("org-1"),
					CreatedAt:      time.Now(),
				})
			},
			ExpectedOrg: "org-1",
		},
		"Expired state fails": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, InstallationState{
					OrganizationID: null.StringFrom("org-1"),
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

			man := newTestManager(t, nil, nil)

			if !tc.Configured {
				man = newDisabledManager(t)
			}

			is, err := man.VerifyInstallationState(tc.State(t))

			if tc.ExpectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, null.StringFrom(tc.ExpectedOrg), is.OrganizationID)
		})
	}
}

// decryptInstallationState decrypts and decodes an installation state
// query parameter.
func decryptInstallationState(t *testing.T, state string) InstallationState {
	t.Helper()

	decrypted, err := cryptoutil.DecryptText(state, []byte(_testSigningSecret))
	require.NoError(t, err)

	var is InstallationState

	require.NoError(t, json.Unmarshal([]byte(decrypted), &is))

	return is
}
