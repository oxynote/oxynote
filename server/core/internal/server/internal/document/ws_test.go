package document

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionCtx returns a context carrying the standard test session.
func sessionCtx() context.Context {
	return auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// docTopicCtx returns a subscriber context for the given document topic.
func docTopicCtx(orgID string, documentID xid.ID) context.Context {
	return wsserver.NewTopicParamsContext(
		auth.AddSessionToContext(context.Background(), auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: orgID,
		}),
		map[string]string{"documentId": documentID.String()},
	)
}

func Test_Handler_BindTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}
	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)
	require.NotNil(t, hdl.tree.changeCallback)
}

func Test_Handler_NotifyTreeChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	// NotifyTreeChange before binding must be a safe no-op.
	hdl.NotifyTreeChange("org1", null.Value[xid.ID]{})

	tpc := &wsMock.Topic{}

	hdl.BindTreeChange(tpc)

	parentID := null.ValueFrom(xid.New())

	hdl.NotifyTreeChange("org1", parentID)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, TreeChangeMessage{ParentID: parentID}, pubs[0].Payload)

	assert.True(t, pubs[0].Filter(sessionCtx(), "topic"))
	assert.False(t, pubs[0].Filter(context.Background(), "topic"))
}

func Test_Handler_BindReviewersChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	tpc := &wsMock.Topic{}

	hdl.BindReviewersChange(tpc)
	require.NotNil(t, hdl.reviewers.changeCallback)

	hdl.reviewers.changeCallback("org1", _documentID)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)
	assert.Equal(t, struct{}{}, pubs[0].Payload)

	assert.True(t, pubs[0].Filter(docTopicCtx("org1", _documentID), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx("org1", xid.New()), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx("org2", _documentID), "topic"))
}

func Test_Handler_BindMaintainersChange(t *testing.T) {
	t.Parallel()

	hdl := &Handler{}

	tpc := &wsMock.Topic{}

	hdl.BindMaintainersChange(tpc)
	require.NotNil(t, hdl.maintainers.changeCallback)

	hdl.maintainers.changeCallback("org1", _documentID)

	pubs := tpc.PublishManyCalls()
	require.Len(t, pubs, 1)

	assert.True(t, pubs[0].Filter(docTopicCtx("org1", _documentID), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx("org1", xid.New()), "topic"))
	assert.False(t, pubs[0].Filter(docTopicCtx("org2", _documentID), "topic"))
}

func Test_Handler_BindMetadataChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		Published int
	}{
		"Branch fetch error on subscription": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return nil, errors.New("boom")
				},
			},
		},
		"Branch document fetch error skips the branch": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}, {BranchID: _branchID2}}, nil
				},
				FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, _ string) (*documentCore.Document, error) {
					if branchID == _branchID {
						return nil, errors.New("boom")
					}

					return storedDoc(), nil
				},
			},
			Published: 1,
		},
		"Initial state sync publishes each branch": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}, {BranchID: _branchID2}}, nil
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Published: 2,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			// deliver the OnSub callback immediately with a subscriber
			// context for the tested document.
			tpc := &wsMock.Topic{
				OnSubFunc: func(fn func(context.Context)) {
					fn(docTopicCtx("org1", _documentID))
				},
			}

			hdl.BindMetadataChange(tpc)
			require.NotNil(t, hdl.metadata.changeCallback)

			assert.Len(t, tpc.PublishOneCalls(), c.Published)

			if c.Published > 0 {
				meta, ok := tpc.PublishOneCalls()[0].Payload.(BranchMetadata)
				require.True(t, ok)
				assert.Equal(t, _branchID, meta.BranchID)
				assert.Equal(t, "Doc", meta.DocumentName)
			}

			// a metadata change publishes to matching subscribers only.
			hdl.metadata.changeCallback("org1", *storedDoc())

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, 1)

			meta, ok := pubs[0].Payload.(BranchMetadata)
			require.True(t, ok)
			assert.Equal(t, _branchID, meta.BranchID)

			assert.True(t, pubs[0].Filter(docTopicCtx("org1", _documentID), "topic"))
			assert.False(t, pubs[0].Filter(docTopicCtx("org1", xid.New()), "topic"))
			assert.False(t, pubs[0].Filter(docTopicCtx("org2", _documentID), "topic"))
		})
	}
}
