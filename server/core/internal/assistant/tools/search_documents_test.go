package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_searchDocuments_InvokableRun(t *testing.T) {
	docID := xid.New()

	stubSearch := func(err error, blocks ...search.Block) *SearcherMock {
		return &SearcherMock{
			SearchDocumentBlocksFunc: func(_ context.Context, _, _ string, _ int) ([]search.Block, error) {
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
			Search: stubSearch(nil, search.Block{ID: "b1", DocumentID: docID, Text: "meow"}),
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
			Search: stubSearch(nil, search.Block{ID: "b1", DocumentID: docID, Text: "meow"}),
			Args:   `{"query":"cats","limit":5}`,
			Limit:  5,
			RespJSON: `{"hits":[{"document_id":"` + docID.String() + `",` +
				`"block_uid":"b1","text":"meow"}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{
				log:    slog.New(slog.DiscardHandler),
				db:     c.DB,
				search: c.Search,
				orgID:  "org",
			}

			res, err := (&searchDocuments{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)

			ff := c.Search.SearchDocumentBlocksCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Limit, ff[0].Limit)
		})
	}
}
