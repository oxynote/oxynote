package tools

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, Traits{}, info.Traits)

	assert.Equal(t, NameSearchDocuments, info.Name)
	assert.Equal(t, []string{"query"}, info.Required)
	assert.Contains(t, info.Properties, "limit")
}

func Test_searchDocuments_Title(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	assert.Equal(t, `Searching for "rate limit"`, searchDocuments{}.Title(testInput(d, NameSearchDocuments, `{"query":"rate limit"}`)))
	assert.Empty(t, searchDocuments{}.Title(testInput(d, NameSearchDocuments, `{}`)))
}

func Test_searchDocuments_Execute(t *testing.T) {
	t.Parallel()

	docID, branchID, mainID := xid.New(), xid.New(), xid.New()

	branchFields := `"branch_id":"` + branchID.String() + `","branch_name":"draft","default":false,`
	hit := `"hits":[{"block_uid":"b1","text":"rate limiting"}]`

	hitSearcher := func(limit *int) *SearcherMock {
		return &SearcherMock{
			SearchDocumentBlocksFunc: func(_ context.Context, _, _ string, l int) ([]search.Block, error) {
				if limit != nil {
					*limit = l
				}

				return []search.Block{{
					DocumentID:    docID,
					BranchID:      branchID,
					BranchName:    "draft",
					BranchDefault: false,
					ID:            branchID.String() + "-b1",
					Text:          "rate limiting",
				}}, nil
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
		"Query over the cap": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{"query":"` + strings.Repeat("a", search.MaxQueryLength+1) + `"}`,
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
			Result: `{"documents":[]}`,
		},
		"Names are joined in": {
			Searcher: hitSearcher(nil),
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					return document.Summaries{{ID: docID, DocumentName: "Runbook"}}, nil
				},
			},
			Args:   `{"query":"rate limit"}`,
			Limit:  _searchLimitDefault,
			Result: `{"documents":[{"document_id":"` + docID.String() + `","document_name":"Runbook",` + branchFields + hit + `}]}`,
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
			Result: `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
			Logged: "cannot fetch the document tree for search hit names",
		},
		"Requested limit is honoured": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5}`,
			Limit:    5,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
		},
		// hits of one branch sit under it once, in the order its best hit
		// ranked; another branch of the same document is its own entry.
		"Hits are grouped by branch": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return []search.Block{
						{DocumentID: docID, BranchID: branchID, BranchName: "draft", ID: branchID.String() + "-b1", Text: "one"},
						{DocumentID: docID, BranchID: mainID, BranchName: "main", BranchDefault: true, ID: mainID.String() + "-b2", Text: "two"},
						{DocumentID: docID, BranchID: branchID, BranchName: "draft", ID: branchID.String() + "-b3", Text: "three"},
					}, nil
				},
			},
			DB:    &DBMock{},
			Args:  `{"query":"rate limit"}`,
			Limit: _searchLimitDefault,
			Result: `{"documents":[` +
				`{"document_id":"` + docID.String() + `",` + branchFields + `"hits":[{"block_uid":"b1","text":"one"},{"block_uid":"b3","text":"three"}]},` +
				`{"document_id":"` + docID.String() + `","branch_id":"` + mainID.String() + `","branch_name":"main","default":true,"hits":[{"block_uid":"b2","text":"two"}]}]}`,
		},
		"Oversized limit is clipped": {
			Searcher: hitSearcher(nil),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5000}`,
			Limit:    _searchLimitMax,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
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
