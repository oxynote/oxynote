package slack

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_CreateLinkURL(t *testing.T) {
	t.Parallel()

	t.Run("Unconfigured manager fails", func(t *testing.T) {
		t.Parallel()

		_, err := newDisabledManager(t).CreateLinkURL("user", "team", "org")
		assert.ErrorIs(t, err, ErrNotConfigured)
	})

	t.Run("Configured manager produces a link URL with an encrypted state", func(t *testing.T) {
		t.Parallel()

		man := newTestManager(t, nil, nil)

		rawURL, err := man.CreateLinkURL("slack-user", "team-1", "org-1")
		require.NoError(t, err)

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

func Test_Manager_VerifyLinkState(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Configured  bool
		State       func(t *testing.T) string
		ExpectedErr string
	}{
		"Unconfigured manager fails": {
			State: func(*testing.T) string {
				return "state"
			},
			ExpectedErr: ErrNotConfigured.Error(),
		},
		"Fresh state returns the link state": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, LinkState{
					SlackUserID: "slack-user",
					CreatedAt:   time.Now(),
				})
			},
		},
		"Expired state fails": {
			Configured: true,
			State: func(t *testing.T) string {
				return encryptState(t, LinkState{
					SlackUserID: "slack-user",
					CreatedAt:   time.Now().Add(-_linkStateTTL - time.Minute),
				})
			},
			ExpectedErr: ErrLinkStateExpired.Error(),
		},
		"Garbage state fails decryption": {
			Configured: true,
			State: func(*testing.T) string {
				return "not-a-state"
			},
			ExpectedErr: "failed to decrypt link state",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man := newTestManager(t, nil, nil)

			if !tc.Configured {
				man = newDisabledManager(t)
			}

			ls, err := man.VerifyLinkState(tc.State(t))

			if tc.ExpectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "slack-user", ls.SlackUserID)
		})
	}
}
