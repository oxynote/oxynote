package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_listDocuments_InvokableRun(t *testing.T) {
	parentID := xid.New()
	childID := xid.New()

	tree := document.Summaries{
		{
			ID:           parentID,
			DocumentName: "Root",
			Icon:         "lucide:file",
			Children: document.Summaries{
				{ID: childID, DocumentName: "Child", Icon: "lucide:cat"},
			},
		},
	}

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		RespJSON string
		Err      error
	}{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Invalid parent id": {
			DB:   &DBMock{},
			Args: `{"parent_id":"not-an-xid"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentTree": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentTreeByDocumentParentID": {
			DB: &DBMock{
				FetchDocumentTreeByDocumentParentIDFunc: func(_ context.Context, _ null.Value[xid.ID], _ string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"parent_id":"` + parentID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Full tree fetch": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
					return tree, nil
				},
			},
			Args: `{}`,
			RespJSON: `{"documents":[{"id":"` + parentID.String() + `","name":"Root","icon":"lucide:file",` +
				`"children":[{"id":"` + childID.String() + `","name":"Child","icon":"lucide:cat"}]}]}`,
		},
		"Scoped fetch by parent": {
			DB: &DBMock{
				FetchDocumentTreeByDocumentParentIDFunc: func(_ context.Context, pid null.Value[xid.ID], _ string) (document.Summaries, error) {
					if pid.V != parentID {
						return nil, assert.AnError
					}

					return tree[0].Children, nil
				},
			},
			Args:     `{"parent_id":"` + parentID.String() + `"}`,
			RespJSON: `{"documents":[{"id":"` + childID.String() + `","name":"Child","icon":"lucide:cat"}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := (&listDocuments{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

func Test_collectDocumentNames(t *testing.T) {
	t.Parallel()

	parentID := xid.New()
	childID := xid.New()

	out := map[string]string{}
	collectDocumentNames(document.Summaries{
		{
			ID:           parentID,
			DocumentName: "Root",
			Children:     document.Summaries{{ID: childID, DocumentName: "Child"}},
		},
	}, out)

	assert.Equal(t, map[string]string{
		parentID.String(): "Root",
		childID.String():  "Child",
	}, out)
}

func Test_summariesToTree(t *testing.T) {
	t.Parallel()

	// empty input yields nil
	assert.Nil(t, summariesToTree(nil))

	// nested summaries convert recursively
	parentID := xid.New()
	childID := xid.New()

	got := summariesToTree(document.Summaries{
		{
			ID:           parentID,
			DocumentName: "Root",
			Icon:         "lucide:file",
			Children:     document.Summaries{{ID: childID, DocumentName: "Child"}},
		},
	})

	assert.Equal(t, []docTreeNode{
		{
			ID:       parentID.String(),
			Name:     "Root",
			Icon:     "lucide:file",
			Children: []docTreeNode{{ID: childID.String(), Name: "Child"}},
		},
	}, got)
}
