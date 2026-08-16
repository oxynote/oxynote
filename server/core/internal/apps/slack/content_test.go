package slack

import (
	"testing"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewMessage(t *testing.T) {
	t.Parallel()

	msg := NewMessage("org-1", "hello")

	require.NotNil(t, msg)
	assert.NotEqual(t, xid.NilID(), msg.ID)
	assert.Equal(t, "org-1", msg.OrganizationID)
	assert.Equal(t, "hello", msg.Text)
	assert.False(t, msg.CreatedAt.IsZero())
}

func Test_NewUserLink(t *testing.T) {
	t.Parallel()

	link := NewUserLink("slack-user", "team-1", "user-1")

	require.NotNil(t, link)
	assert.Equal(t, "slack-user", link.SlackUserID)
	assert.Equal(t, "team-1", link.TeamID)
	assert.Equal(t, "user-1", link.UserID)
	assert.True(t, link.Settings.Notifications)
	assert.False(t, link.CreatedAt.IsZero())
}

func Test_UserLinkSettings_Value(t *testing.T) {
	t.Parallel()

	val, err := UserLinkSettings{Notifications: true}.Value()

	require.NoError(t, err)
	assert.JSONEq(t, `{"notifications": true}`, string(val.([]byte)))
}

func Test_UserLinkSettings_Scan(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Value     any
		ExpectErr bool
		Expected  UserLinkSettings
	}{
		"Byte slice is decoded": {
			Value:    []byte(`{"notifications": true}`),
			Expected: UserLinkSettings{Notifications: true},
		},
		"String is decoded": {
			Value:    `{"notifications": true}`,
			Expected: UserLinkSettings{Notifications: true},
		},
		"Nil leaves the settings untouched": {
			Value: nil,
		},
		"Unsupported type fails": {
			Value:     42,
			ExpectErr: true,
		},
		"Malformed JSON fails": {
			Value:     []byte(`{not json`),
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var uls UserLinkSettings

			err := uls.Scan(tc.Value)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.Expected, uls)
		})
	}
}

// Value round-trips through Scan.
func Test_UserLinkSettings_RoundTrip(t *testing.T) {
	t.Parallel()

	orig := UserLinkSettings{Notifications: true}

	val, err := orig.Value()
	require.NoError(t, err)

	var decoded UserLinkSettings

	require.NoError(t, decoded.Scan(val))
	assert.Equal(t, orig, decoded)
}
