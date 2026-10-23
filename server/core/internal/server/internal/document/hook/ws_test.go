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

func Test_Handler_BindHooksChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindHooksChange(tpc)
	require.NotNil(t, hdl.hooks.changeCallback)
}

func Test_Handler_NotifyHooksChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Bound      bool
		DocumentID null.Value[xid.ID]
		BranchID   null.Value[xid.ID]
		Published  int
	}{
		"Unbound handler publishes nothing": {
			DocumentID: null.ValueFrom(_documentID),
			BranchID:   null.ValueFrom(_branchID),
		},
		"Null branch ID publishes nothing": {
			Bound:      true,
			DocumentID: null.ValueFrom(_documentID),
		},
		"Null document ID publishes nothing": {
			Bound:    true,
			BranchID: null.ValueFrom(_branchID),
		},
		"Bound handler publishes the change": {
			Bound:      true,
			DocumentID: null.ValueFrom(_documentID),
			BranchID:   null.ValueFrom(_branchID),
			Published:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}
			tpc := &wsMock.Topic{}

			if c.Bound {
				hdl.BindHooksChange(tpc)
			}

			hdl.NotifyHooksChange("org1", c.DocumentID, c.BranchID)

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, c.Published)

			if c.Published == 0 {
				return
			}

			assert.Equal(t, HooksChangeMessage{BranchID: _branchID}, pubs[0].Payload)

			ctx := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})

			// only the subscribers of that document in the organization hear it
			assert.True(t, pubs[0].Filter(wsserver.NewTopicParamsContext(ctx, map[string]string{"documentId": _documentID.String()}), "topic"))
			assert.False(t, pubs[0].Filter(wsserver.NewTopicParamsContext(ctx, map[string]string{"documentId": xid.New().String()}), "topic"))
			assert.False(t, pubs[0].Filter(ctx, "topic"))
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))
		})
	}
}
