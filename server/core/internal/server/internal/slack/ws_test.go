package slack

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_BindPostMessage(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	tpc := &wsMock.Topic{}

	hdl.BindPostMessage(tpc)
	require.NotNil(t, hdl.message.postCallback)

	id := xid.New()

	hdl.message.postCallback("org1", id)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, Message{ID: id}, pubs[0].Payload)

	// the event carries a filter so only the owning organization is told.
	require.NotNil(t, pubs[0].Filter)

	assert.True(t, pubs[0].Filter(sessionCtx("org1"), ""))
	assert.False(t, pubs[0].Filter(sessionCtx("org2"), ""))
}

// sessionCtx builds a context carrying a session for the given organization.
func sessionCtx(organizationID string) context.Context {
	return auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: organizationID,
	})
}
