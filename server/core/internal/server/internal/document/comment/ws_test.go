package comment

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_BindCommentsChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	tpc := &wsMock.Topic{}

	hdl.BindCommentsChange(tpc)
	require.NotNil(t, hdl.comments.changeCallback)

	msg := ChangeMessage{
		Type:      ChangeTypeCreated,
		CommentID: _commentID,
	}

	hdl.comments.changeCallback("org1", _documentID, msg)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, msg, pubs[0].Payload)

	session := auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}

	// subscriber viewing the right document in the right organization.
	match := wsserver.NewTopicParamsContext(
		auth.AddSessionToContext(context.Background(), session),
		map[string]string{"documentId": _documentID.String()},
	)
	assert.True(t, pubs[0].Filter(match, "topic"))

	// subscriber viewing a different document.
	wrongDoc := wsserver.NewTopicParamsContext(
		auth.AddSessionToContext(context.Background(), session),
		map[string]string{"documentId": _commentID.String()},
	)
	assert.False(t, pubs[0].Filter(wrongDoc, "topic"))

	// subscriber from another organization.
	wrongOrg := wsserver.NewTopicParamsContext(
		auth.AddSessionToContext(context.Background(), auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: "org2",
		}),
		map[string]string{"documentId": _documentID.String()},
	)
	assert.False(t, pubs[0].Filter(wrongOrg, "topic"))
}
