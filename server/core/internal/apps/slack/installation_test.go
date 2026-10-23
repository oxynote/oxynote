package slack

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_CreateExternalInstallationURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		Err          error
	}{
		"Unconfigured manager fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Configured manager produces an authorize URL with an encrypted state": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if c.Unconfigured {
				man = newDisabledManager(t)
			}

			rawURL, err := man.CreateExternalInstallationURL("org-1")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

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

			is, err := state.Decode[InstallationState](
				q.Get("state"),
				_installationStatePurpose,
				_testSigningSecret,
				_installationStateTTL,
			)
			require.NoError(t, err)

			assert.Equal(t, null.StringFrom("org-1"), is.OrganizationID)
			assert.False(t, is.TeamID.Valid)
			assert.WithinDuration(t, time.Now(), is.CreatedAt, time.Minute)
		})
	}
}

func Test_Manager_CreateInternalInstallationURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		Err          error
	}{
		"Unconfigured manager fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Configured manager produces a redirect URL with an encrypted state": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if c.Unconfigured {
				man = newDisabledManager(t)
			}

			rawURL, err := man.CreateInternalInstallationURL("team-1")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			assert.Equal(t, _testRedirectURL, u.Scheme+"://"+u.Host+u.Path)

			is, err := state.Decode[InstallationState](
				u.Query().Get("state"),
				_installationStatePurpose,
				_testSigningSecret,
				_installationStateTTL,
			)
			require.NoError(t, err)

			assert.Equal(t, null.StringFrom("team-1"), is.TeamID)
			assert.False(t, is.OrganizationID.Valid)
			assert.WithinDuration(t, time.Now(), is.CreatedAt, time.Minute)
		})
	}
}

func Test_Manager_VerifyInstallationState(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured  bool
		State       func(t *testing.T) string
		ExpectedOrg string
		Err         error
	}{
		"Unconfigured manager fails": {
			State: func(*testing.T) string {
				return "state"
			},
			Err: ErrNotConfigured,
		},
		"Fresh state returns the installation state": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					InstallationState{
						OrganizationID: null.StringFrom("org-1"),
						CreatedAt:      time.Now(),
					},
					_installationStatePurpose,
					_testSigningSecret,
				)
				require.NoError(t, err)

				return token
			},
			ExpectedOrg: "org-1",
		},
		"Expired state fails": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					InstallationState{
						OrganizationID: null.StringFrom("org-1"),
						CreatedAt:      time.Now().Add(-_installationStateTTL - time.Minute),
					},
					_installationStatePurpose,
					_testSigningSecret,
				)
				require.NoError(t, err)

				return token
			},
			Err: ErrInstallationStateExpired,
		},
		"Link state minted under the same secret fails": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					LinkState{
						SlackUserID:    "slack-user",
						TeamID:         "team-1",
						OrganizationID: "org-1",
						CreatedAt:      time.Now(),
					},
					_linkStatePurpose,
					_testSigningSecret,
				)
				require.NoError(t, err)

				return token
			},
			Err: ErrInvalidInstallationState,
		},
		"Garbage state fails decryption": {
			Configured: true,
			State: func(*testing.T) string {
				return "not-a-state"
			},
			Err: ErrInvalidInstallationState,
		},
		"Non-JSON state payload fails unmarshalling": {
			Configured: true,
			State: func(t *testing.T) string {
				state, err := cryptoutil.EncryptText("not json", []byte(_testSigningSecret))
				require.NoError(t, err)

				return state
			},
			Err: ErrInvalidInstallationState,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if !c.Configured {
				man = newDisabledManager(t)
			}

			is, err := man.VerifyInstallationState(c.State(t))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, null.StringFrom(c.ExpectedOrg), is.OrganizationID)
		})
	}
}
