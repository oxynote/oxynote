package tag

import (
	"context"
	"testing"

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

func Test_Handler_BindTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	require.NotNil(t, hdl.tree.changeCallback)
	require.NotNil(t, hdl.tree.userChangeCallback)
}

func Test_Handler_NotifyTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// notifying before binding must be a safe no-op.
	hdl.NotifyTreeChange("org1")

	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	hdl.NotifyTreeChange("org1")

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

func Test_Handler_BindBranchTagsChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindBranchTagsChange(tpc)
	require.NotNil(t, hdl.branchTags.changeCallback)
}

func Test_Handler_NotifyBranchTagsChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// notifying before binding must be a safe no-op.
	hdl.NotifyBranchTagsChange("org1", _documentID, _branchID)

	tpc := &wsMock.Topic{}

	hdl.BindBranchTagsChange(tpc)
	hdl.NotifyBranchTagsChange("org1", _documentID, _branchID)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, BranchTagsChangeMessage{BranchID: _branchID}, pubs[0].Payload)

	// only the subscribers of that document in the organization hear it
	assert.True(t, pubs[0].Filter(docTopicCtx(_documentID), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx(xid.New()), "topic"))
	assert.False(t, pubs[0].Filter(sessionCtx(), "topic"))
	assert.False(t, pubs[0].Filter(context.Background(), "topic"))
}
