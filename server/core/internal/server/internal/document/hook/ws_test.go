package hook

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/rs/xid"
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

// docTopicCtx returns a subscriber context for the given document topic.
func docTopicCtx(documentID xid.ID) context.Context {
	return wsserver.NewTopicParamsContext(sessionCtx(), map[string]string{"documentId": documentID.String()})
}

func Test_Handler_BindHooksChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindHooksChange(tpc)
	require.NotNil(t, hdl.hooks.changeCallback)
}

func Test_Handler_NotifyHooksChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// notifying before binding must be a safe no-op.
	hdl.NotifyHooksChange("org1", null.ValueFrom(_documentID), null.ValueFrom(_branchID))

	tpc := &wsMock.Topic{}

	hdl.BindHooksChange(tpc)

	// a hook whose branch or document is gone has no editor to redraw.
	hdl.NotifyHooksChange("org1", null.ValueFrom(_documentID), null.Value[xid.ID]{})
	hdl.NotifyHooksChange("org1", null.Value[xid.ID]{}, null.ValueFrom(_branchID))
	require.Empty(t, tpc.PublishManyCalls())

	hdl.NotifyHooksChange("org1", null.ValueFrom(_documentID), null.ValueFrom(_branchID))

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, HooksChangeMessage{BranchID: _branchID}, pubs[0].Payload)

	// only the subscribers of that document in the organization hear it
	assert.True(t, pubs[0].Filter(docTopicCtx(_documentID), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx(xid.New()), "topic"))
	assert.False(t, pubs[0].Filter(sessionCtx(), "topic"))
	assert.False(t, pubs[0].Filter(context.Background(), "topic"))
}
