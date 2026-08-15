package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_listDocuments(t *testing.T) {
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

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := m.listDocuments(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

func Test_Manager_getDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()
	parentID := xid.New()
	updated := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

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
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, _ xid.ID, _, _ string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful fetch with parent": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, id xid.ID, _, _ string) (*document.Document, error) {
					return &document.Document{
						Branch: document.Branch{
							BranchID:     branchID,
							BranchName:   "main",
							DocumentName: "Cat Facts",
							Icon:         "lucide:cat",
							Protected:    true,
							UpdatedAt:    updated,
						},
						ID:       id,
						ParentID: null.ValueFrom(parentID),
					}, nil
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			RespJSON: `{"id":"` + docID.String() + `","name":"Cat Facts","icon":"lucide:cat",` +
				`"parent_id":"` + parentID.String() + `","branch_id":"` + branchID.String() + `",` +
				`"branch_name":"main","protected":true,"updated_at":"2026-08-15T10:00:00Z"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := m.getDocument(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

func Test_Manager_readDocumentSummary(t *testing.T) {
	docID := xid.New()

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
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Successful summary": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{
						DocumentName: "Cat Facts",
						Content: document.RootBlock{
							Type: "doc",
							Content: []document.Block{
								pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("hello")),
							},
						},
					}, nil
				},
			},
			Args: `{"document_id":"` + docID.String() + `"}`,
			RespJSON: `{"document_id":"` + docID.String() + `","document_name":"Cat Facts",` +
				`"blocks":[{"uid":"p1","kind":"paragraph","text":"hello","depth":0}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := m.readDocumentSummary(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

func Test_Manager_readBlock(t *testing.T) {
	docID := xid.New()

	stubContent := func(blocks ...document.Block) *DBMock {
		return &DBMock{
			FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
				return document.Content{
					Content: document.RootBlock{Type: "doc", Content: blocks},
				}, nil
			},
		}
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
		"Invalid document id": {
			DB:   &DBMock{},
			Args: `{"document_id":"nope","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Missing block uid": {
			DB:   &DBMock{},
			Args: `{"document_id":"` + docID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Block not found": {
			DB:   stubContent(pmBlock(document.BlockNodeParagraph, "other", nil)),
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Unsupported block type fails to compact": {
			DB:   stubContent(pmBlock("weirdNode", "b1", nil)),
			Args: `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			Err:  assert.AnError,
		},
		"Successful read": {
			DB:       stubContent(pmBlock(document.BlockNodeParagraph, "b1", nil, pmText("hello"))),
			Args:     `{"document_id":"` + docID.String() + `","block_uid":"b1"}`,
			RespJSON: `{"type":"paragraph","uid":"b1","text":"hello"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			res, err := m.readBlock(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))
		})
	}
}

func Test_Manager_searchDocuments(t *testing.T) {
	docID := xid.New()

	stubSearch := func(err error, blocks ...searchgw.Block) *SearcherMock {
		return &SearcherMock{
			SearchDocumentBlocksFunc: func(_ context.Context, _, _ string, _ int) ([]searchgw.Block, error) {
				return blocks, err
			},
		}
	}

	cc := map[string]struct {
		DB       *DBMock
		Search   *SearcherMock
		Args     string
		Limit    int
		RespJSON string
		Err      error
	}{
		"Malformed args": {
			DB:     &DBMock{},
			Search: &SearcherMock{},
			Args:   `{broken`,
			Err:    assert.AnError,
		},
		"Missing query": {
			DB:     &DBMock{},
			Search: &SearcherMock{},
			Args:   `{}`,
			Err:    assert.AnError,
		},
		"Error returned by search.SearchDocumentBlocks": {
			DB:     &DBMock{},
			Search: stubSearch(assert.AnError),
			Args:   `{"query":"x"}`,
			Err:    assert.AnError,
		},
		"Default limit applied": {
			DB:       &DBMock{},
			Search:   stubSearch(nil),
			Args:     `{"query":"x"}`,
			Limit:    20,
			RespJSON: `{"hits":[]}`,
		},
		"Oversized limit clamped": {
			DB:       &DBMock{},
			Search:   stubSearch(nil),
			Args:     `{"query":"x","limit":999}`,
			Limit:    50,
			RespJSON: `{"hits":[]}`,
		},
		"Hits joined with document names": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
					return document.Summaries{{ID: docID, DocumentName: "Cat Facts"}}, nil
				},
			},
			Search: stubSearch(nil, searchgw.Block{ID: "b1", DocumentID: docID, Text: "meow"}),
			Args:   `{"query":"cats","limit":5}`,
			Limit:  5,
			RespJSON: `{"hits":[{"document_id":"` + docID.String() + `","document_name":"Cat Facts",` +
				`"block_uid":"b1","text":"meow"}]}`,
		},
		"Name join degrades when the tree fetch fails": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Search: stubSearch(nil, searchgw.Block{ID: "b1", DocumentID: docID, Text: "meow"}),
			Args:   `{"query":"cats","limit":5}`,
			Limit:  5,
			RespJSON: `{"hits":[{"document_id":"` + docID.String() + `",` +
				`"block_uid":"b1","text":"meow"}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				log:    slog.New(slog.DiscardHandler),
				db:     c.DB,
				search: c.Search,
				orgID:  "org",
			}

			res, err := m.searchDocuments(context.Background(), json.RawMessage(c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, string(res))

			ff := c.Search.SearchDocumentBlocksCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Limit, ff[0].Limit)
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

func Test_marshalResult(t *testing.T) {
	t.Parallel()

	// error
	_, err := marshalResult(make(chan int))
	require.Error(t, err)

	// success
	res, err := marshalResult(map[string]any{"ok": true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(res))
}
