package github

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

func Test_Manager_CreateInstallationURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		Err          error
	}{
		"Unconfigured manager fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Configured manager produces a URL with an encrypted state": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil)

			if c.Unconfigured {
				var err error

				man, err = NewManager(nil, Options{})
				require.NoError(t, err)
			}

			rawURL, err := man.CreateInstallationURL("org-1")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			assert.Equal(t, "https", u.Scheme)
			assert.Equal(t, "github.com", u.Host)
			assert.Equal(t, "/apps/test-app/installations/new", u.Path)

			is, err := state.Decode[InstallationState](
				u.Query().Get("state"),
				_installationStatePurpose,
				_testSigningSecret,
				_installationStateTTL,
			)
			require.NoError(t, err)

			assert.Equal(t, "org-1", is.OrganizationID)
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
		"Fresh state returns the organization ID": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					InstallationState{
						OrganizationID: "org-1",
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
						OrganizationID: "org-1",
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
		"Garbage state fails decryption": {
			Configured: true,
			State: func(*testing.T) string {
				return "not-a-state"
			},
			Err: ErrInvalidInstallationState,
		},
		"State encrypted with another key fails decryption": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					InstallationState{
						OrganizationID: "org-1",
						CreatedAt:      time.Now(),
					},
					_installationStatePurpose,
					strings.Repeat("x", 32),
				)
				require.NoError(t, err)

				return token
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

			man := newTestManager(t, nil)

			if !c.Configured {
				var err error

				man, err = NewManager(nil, Options{})
				require.NoError(t, err)
			}

			is, err := man.VerifyInstallationState(c.State(t))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.ExpectedOrg, is.OrganizationID)
		})
	}
}
