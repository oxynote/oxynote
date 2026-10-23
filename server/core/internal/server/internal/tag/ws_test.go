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

func Test_Handler_BindTreeChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Fire  func(*Handler)
		Allow []auth.Session
		Deny  []auth.Session
	}{
		"Organization change reaches every member": {
			Fire: func(hdl *Handler) {
				hdl.tree.changeCallback("org1")
			},
			Allow: []auth.Session{{UserID: "u1", ActiveOrganizationID: "org1"}, {UserID: "u2", ActiveOrganizationID: "org1"}},
			Deny:  []auth.Session{{UserID: "u1", ActiveOrganizationID: "org2"}},
		},
		"User change reaches only the user": {
			Fire: func(hdl *Handler) {
				hdl.tree.userChangeCallback("org1", "u1")
			},
			Allow: []auth.Session{{UserID: "u1", ActiveOrganizationID: "org1"}},
			Deny:  []auth.Session{{UserID: "u2", ActiveOrganizationID: "org1"}, {UserID: "u1", ActiveOrganizationID: "org2"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}
			tpc := &wsMock.Topic{}

			hdl.BindTreeChange(tpc)
			require.NotNil(t, hdl.tree.changeCallback)
			require.NotNil(t, hdl.tree.userChangeCallback)

			c.Fire(hdl)

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, 1)
			assert.Equal(t, TreeChangeMessage{}, pubs[0].Payload)
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))

			_, ok := pubs[0].Ctx.Deadline()
			assert.True(t, ok)

			for _, s := range c.Allow {
				assert.True(t, pubs[0].Filter(auth.AddSessionToContext(context.Background(), s), "topic"))
			}

			for _, s := range c.Deny {
				assert.False(t, pubs[0].Filter(auth.AddSessionToContext(context.Background(), s), "topic"))
			}
		})
	}
}

func Test_Handler_NotifyTreeChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		BindLate  bool
		Published int
	}{
		"Notification before binding": {
			BindLate: true,
		},
		"Successful publish": {
			Published: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}
			tpc := &wsMock.Topic{}

			if !c.BindLate {
				hdl.BindTreeChange(tpc)
			}

			hdl.NotifyTreeChange("org1")

			// a late bind proves the notification was dropped, not kept.
			if c.BindLate {
				hdl.BindTreeChange(tpc)
			}

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, c.Published)

			if c.Published == 0 {
				return
			}

			assert.Equal(t, TreeChangeMessage{}, pubs[0].Payload)

			member := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})

			assert.True(t, pubs[0].Filter(member, "topic"))
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))
		})
	}
}

func Test_Handler_notifyUserTreeChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		BindLate  bool
		Published int
	}{
		"Notification before binding": {
			BindLate: true,
		},
		"Successful publish": {
			Published: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}
			tpc := &wsMock.Topic{}

			if !c.BindLate {
				hdl.BindTreeChange(tpc)
			}

			hdl.notifyUserTreeChange("org1", "u1")

			// a late bind proves the notification was dropped, not kept.
			if c.BindLate {
				hdl.BindTreeChange(tpc)
			}

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, c.Published)

			if c.Published == 0 {
				return
			}

			assert.Equal(t, TreeChangeMessage{}, pubs[0].Payload)

			// the same organization is not enough: it has to be the same user
			owner := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})
			other := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u2",
				ActiveOrganizationID: "org1",
			})

			assert.True(t, pubs[0].Filter(owner, "topic"))
			assert.False(t, pubs[0].Filter(other, "topic"))
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))
		})
	}
}

func Test_Handler_BindBranchTagsChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindBranchTagsChange(tpc)
	require.NotNil(t, hdl.branchTags.changeCallback)

	hdl.branchTags.changeCallback("org1", _documentID, _branchID)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, BranchTagsChangeMessage{BranchID: _branchID}, pubs[0].Payload)

	_, ok := pubs[0].Ctx.Deadline()
	assert.True(t, ok)

	member := auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
	outsider := auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org2",
	})
	doc := wsserver.NewTopicParamsContext(member, map[string]string{"documentId": _documentID.String()})
	otherDoc := wsserver.NewTopicParamsContext(member, map[string]string{"documentId": xid.New().String()})
	otherOrg := wsserver.NewTopicParamsContext(outsider, map[string]string{"documentId": _documentID.String()})

	assert.True(t, pubs[0].Filter(doc, "topic"))
	assert.False(t, pubs[0].Filter(otherDoc, "topic"))
	assert.False(t, pubs[0].Filter(otherOrg, "topic"))
	assert.False(t, pubs[0].Filter(member, "topic"))
}

func Test_Handler_NotifyBranchTagsChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		BindLate  bool
		Published int
	}{
		"Notification before binding": {
			BindLate: true,
		},
		"Successful publish": {
			Published: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{}
			tpc := &wsMock.Topic{}

			if !c.BindLate {
				hdl.BindBranchTagsChange(tpc)
			}

			hdl.NotifyBranchTagsChange("org1", _documentID, _branchID)

			// a late bind proves the notification was dropped, not kept.
			if c.BindLate {
				hdl.BindBranchTagsChange(tpc)
			}

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, c.Published)

			if c.Published == 0 {
				return
			}

			assert.Equal(t, BranchTagsChangeMessage{BranchID: _branchID}, pubs[0].Payload)

			// only the subscribers of that document in the organization hear it
			member := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})
			doc := wsserver.NewTopicParamsContext(member, map[string]string{"documentId": _documentID.String()})
			otherDoc := wsserver.NewTopicParamsContext(member, map[string]string{"documentId": xid.New().String()})

			assert.True(t, pubs[0].Filter(doc, "topic"))
			assert.False(t, pubs[0].Filter(otherDoc, "topic"))
			assert.False(t, pubs[0].Filter(member, "topic"))
			assert.False(t, pubs[0].Filter(context.Background(), "topic"))
		})
	}
}
