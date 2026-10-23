package slack

import (
	"net/url"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/state"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_CreateLinkURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		Err          error
	}{
		"Unconfigured manager fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Configured manager produces a link URL with an encrypted state": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if c.Unconfigured {
				man = newDisabledManager(t)
			}

			rawURL, err := man.CreateLinkURL("slack-user", "team-1", "org-1")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			assert.Equal(t, _testRedirectURL, u.Scheme+"://"+u.Host+u.Path)
			assert.Equal(t, "1", u.Query().Get("user"))

			ls, err := man.VerifyLinkState(u.Query().Get("state"))
			require.NoError(t, err)

			assert.Equal(t, "slack-user", ls.SlackUserID)
			assert.Equal(t, "team-1", ls.TeamID)
			assert.Equal(t, "org-1", ls.OrganizationID)
			assert.WithinDuration(t, time.Now(), ls.CreatedAt, time.Minute)
		})
	}
}

func Test_Manager_VerifyLinkState(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		State      func(t *testing.T) string
		Err        error
	}{
		"Unconfigured manager fails": {
			State: func(*testing.T) string {
				return "state"
			},
			Err: ErrNotConfigured,
		},
		"Fresh state returns the link state": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					LinkState{
						SlackUserID: "slack-user",
						CreatedAt:   time.Now(),
					},
					_linkStatePurpose,
					_testSigningSecret,
				)
				require.NoError(t, err)

				return token
			},
		},
		"Expired state fails": {
			Configured: true,
			State: func(t *testing.T) string {
				token, err := state.Encode(
					LinkState{
						SlackUserID: "slack-user",
						CreatedAt:   time.Now().Add(-_linkStateTTL - time.Minute),
					},
					_linkStatePurpose,
					_testSigningSecret,
				)
				require.NoError(t, err)

				return token
			},
			Err: ErrLinkStateExpired,
		},
		"Installation state minted under the same secret fails": {
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
			Err: ErrInvalidLinkState,
		},
		"Garbage state fails decryption": {
			Configured: true,
			State: func(*testing.T) string {
				return "not-a-state"
			},
			Err: ErrInvalidLinkState,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if !c.Configured {
				man = newDisabledManager(t)
			}

			ls, err := man.VerifyLinkState(c.State(t))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, "slack-user", ls.SlackUserID)
		})
	}
}
