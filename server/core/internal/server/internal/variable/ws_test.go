package variable

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_BindChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	hdl.BindChange(&wsMock.Topic{})
	require.NotNil(t, hdl.changeCallback)
}

func Test_Handler_NotifyChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		UserID null.String
		Allow  []auth.Session
		Deny   []auth.Session
	}{
		"Organization change reaches every member": {
			Allow: []auth.Session{{UserID: "u1", ActiveOrganizationID: "org1"}, {UserID: "u2", ActiveOrganizationID: "org1"}},
			Deny:  []auth.Session{{UserID: "u1", ActiveOrganizationID: "org2"}},
		},
		"Value change reaches only the user": {
			UserID: null.StringFrom("u1"),
			Allow:  []auth.Session{{UserID: "u1", ActiveOrganizationID: "org1"}},
			Deny:   []auth.Session{{UserID: "u2", ActiveOrganizationID: "org1"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}

			// notifying before binding must be a safe no-op.
			hdl.NotifyChange("org1", c.UserID)

			tpc := &wsMock.Topic{}

			hdl.BindChange(tpc)
			hdl.NotifyChange("org1", c.UserID)

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, 1)
			assert.Equal(t, ChangeMessage{}, pubs[0].Payload)
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))

			for _, s := range c.Allow {
				assert.True(t, pubs[0].Filter(auth.AddSessionToContext(context.Background(), s), "topic"))
			}

			for _, s := range c.Deny {
				assert.False(t, pubs[0].Filter(auth.AddSessionToContext(context.Background(), s), "topic"))
			}
		})
	}
}
