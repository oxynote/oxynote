package tools

import (
	"bytes"
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

func Test_searchDocumentsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, searchDocumentsArgs{Query: "q"}, map[string]Args{
		"query": searchDocumentsArgs{},
	})
}

func Test_searchDocuments_Info(t *testing.T) {
	t.Parallel()

	info := searchDocuments{}.Info()

	assert.Equal(t, NameSearchDocuments, info.Name)
	assert.Equal(t, []string{"query"}, info.Required)
	assert.Contains(t, info.Properties, "limit")
}

func Test_searchDocuments_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, searchDocuments{}.Traits())
}

func Test_searchDocuments_Title(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	got, err := searchDocuments{}.Title(testInput(d, NameSearchDocuments, `{"query":"rate limit"}`))
	require.NoError(t, err)
	assert.Equal(t, `Searching for "rate limit"`, got)

	_, err = searchDocuments{}.Title(testInput(d, NameSearchDocuments, `{}`))
	require.Error(t, err)
}

func Test_searchDocuments_Execute(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	hitSearcher := func(limit *int) *SearcherMock {
		return &SearcherMock{
			SearchDocumentBlocksFunc: func(_ context.Context, _, _ string, l int) ([]search.Block, error) {
				if limit != nil {
					*limit = l
				}

				return []search.Block{{DocumentID: docID, ID: "b1", Text: "rate limiting"}}, nil
			},
		}
	}

	cc := map[string]struct {
		Searcher *SearcherMock
		DB       *DBMock
		Args     string
		Limit    int
		Result   string
		Logged   string
		Err      error
	}{
		"Malformed arguments": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{`,
			Err:      assert.AnError,
		},
		"Query is required": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{}`,
			Err:      assert.AnError,
		},
		"Error returned by search.SearchDocumentBlocks": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return nil, assert.AnError
				},
			},
			DB:   &DBMock{},
			Args: `{"query":"rate limit"}`,
			Err:  assert.AnError,
		},
		"Zero hits skip the name lookup": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return nil, nil
				},
			},
			DB:     &DBMock{},
			Args:   `{"query":"rate limit"}`,
			Limit:  _searchLimitDefault,
			Result: `{"hits":[]}`,
		},
		"Names are joined in": {
			Searcher: hitSearcher(nil),
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return document.Summaries{{ID: docID, DocumentName: "Runbook"}}, nil
				},
			},
			Args:  `{"query":"rate limit"}`,
			Limit: _searchLimitDefault,
			Result: `{"hits":[{"document_id":"` + docID.String() + `","document_name":"Runbook",` +
				`"block_uid":"b1","text":"rate limiting"}]}`,
		},
		"Losing the names does not fail the search": {
			Searcher: hitSearcher(nil),
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Args:   `{"query":"rate limit"}`,
			Limit:  _searchLimitDefault,
			Result: `{"hits":[{"document_id":"` + docID.String() + `","block_uid":"b1","text":"rate limiting"}]}`,
			Logged: "cannot fetch the document tree for search hit names",
		},
		"Requested limit is honoured": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5}`,
			Limit:    5,
			Result: `{"hits":[{"document_id":"` + docID.String() + `","block_uid":"b1",` +
				`"text":"rate limiting"}]}`,
		},
		"Oversized limit is clipped": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5000}`,
			Limit:    _searchLimitMax,
			Result: `{"hits":[{"document_id":"` + docID.String() + `","block_uid":"b1",` +
				`"text":"rate limiting"}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				buf   bytes.Buffer
				limit int
			)

			d := testDeps(c.DB, nil, nil)
			d.log = slog.New(slog.NewTextHandler(&buf, nil))
			d.search = &SearcherMock{
				SearchDocumentBlocksFunc: func(ctx context.Context, org, q string, l int) ([]search.Block, error) {
					limit = l

					return c.Searcher.SearchDocumentBlocks(ctx, org, q, l)
				},
			}

			res, err := searchDocuments{}.Execute(testInput(d, NameSearchDocuments, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
			assert.Equal(t, c.Limit, limit)

			if c.Logged == "" {
				assert.NotContains(t, buf.String(), "cannot fetch")

				return
			}

			assert.Contains(t, buf.String(), c.Logged)
		})
	}
}
