package tag

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionCtx returns a subscriber context carrying a test session.
func sessionCtx() context.Context {
	return auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

func Test_Handler_BindTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	require.NotNil(t, hdl.tree.changeCallback)
	require.NotNil(t, hdl.tree.userChangeCallback)
}

func Test_Handler_notifyTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// notifying before binding must be a safe no-op.
	hdl.notifyTreeChange("org1")

	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	hdl.notifyTreeChange("org1")

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, TreeChangeMessage{}, pubs[0].Payload)

	assert.True(t, pubs[0].Filter(sessionCtx(), "topic"))
	assert.False(t, pubs[0].Filter(context.Background(), "topic"))
}

func Test_Handler_notifyUserTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// notifying before binding must be a safe no-op.
	hdl.notifyUserTreeChange("org1", "u1")

	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	hdl.notifyUserTreeChange("org1", "u1")

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, TreeChangeMessage{}, pubs[0].Payload)

	// the same organization is not enough: it has to be the same user
	assert.True(t, pubs[0].Filter(sessionCtx(), "topic"))
	assert.False(t, pubs[0].Filter(otherUserCtx(), "topic"))
	assert.False(t, pubs[0].Filter(context.Background(), "topic"))
}
