package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	cc := map[string]struct {
		Args searchDocumentsArgs
		Err  error
	}{
		"Query is required": {
			Args: searchDocumentsArgs{},
			Err:  errors.New("query is required"),
		},
		"Query over the length cap": {
			Args: searchDocumentsArgs{Query: strings.Repeat("a", search.MaxQueryLength+1)},
			Err:  fmt.Errorf("query must be at most %d characters; search for the key terms instead", search.MaxQueryLength),
		},
		"Valid arguments": {
			Args: searchDocumentsArgs{Query: "q"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
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

	cc := map[string]struct {
		Args   string
		Result string
	}{
		"Query is announced":           {Args: `{"query":"rate limit"}`, Result: `Searching for "rate limit"`},
		"Missing query makes no title": {Args: `{}`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, searchDocuments{}.Title(d.newInput(context.Background(), NameSearchDocuments, json.RawMessage(c.Args))))
		})
	}
}

func Test_searchDocuments_Execute(t *testing.T) {
	t.Parallel()

	docID, branchID, mainID := xid.New(), xid.New(), xid.New()

	branchFields := `"branch_id":"` + branchID.String() + `","branch_name":"draft","default":false,`
	knownFields := branchFields + `"protected":false,`

	// the draft takes writes; the main branch is protected.
	stubBranches := func() *DBMock {
		return &DBMock{
			FetchDocumentBranchesByOrganizationIDFunc: func(context.Context, string) ([]document.BranchSummary, error) {
				return []document.BranchSummary{
					{BranchID: mainID, DocumentID: docID, BranchName: "main", DocumentName: "Runbook", Default: true, Protected: true},
					{BranchID: branchID, DocumentID: docID, BranchName: "draft", DocumentName: "Runbook draft"},
				}, nil
			},
		}
	}
	hit := `"hits":[{"block_uid":"b1","text":"rate limiting"}]`

	hitSearcher := func() *SearcherMock {
		return &SearcherMock{
			SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
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
		Searches int
		Fetches  int
		Err      error
	}{
		"Malformed arguments": {
			Searcher: hitSearcher(),
			DB:       &DBMock{},
			Args:     `{`,
			Searches: 0,
			Fetches:  0,
			Err:      assert.AnError,
		},
		"Query is required": {
			Searcher: hitSearcher(),
			DB:       &DBMock{},
			Args:     `{}`,
			Searches: 0,
			Fetches:  0,
			Err:      assert.AnError,
		},
		"Query over the cap": {
			Searcher: hitSearcher(),
			DB:       &DBMock{},
			Args:     `{"query":"` + strings.Repeat("a", search.MaxQueryLength+1) + `"}`,
			Searches: 0,
			Fetches:  0,
			Err:      assert.AnError,
		},
		"Error returned by search.SearchDocumentBlocks": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return nil, assert.AnError
				},
			},
			DB:       &DBMock{},
			Args:     `{"query":"rate limit"}`,
			Searches: 1,
			Fetches:  0,
			Err:      assert.AnError,
		},
		"Zero hits skip the name lookup": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return nil, nil
				},
			},
			DB:       &DBMock{},
			Args:     `{"query":"rate limit"}`,
			Limit:    _searchLimitDefault,
			Result:   `{"documents":[]}`,
			Searches: 1,
			Fetches:  0,
		},
		// the document goes by its default branch's name, and the hit's
		// branch says whether it takes writes.
		"Names and protection are joined in": {
			Searcher: hitSearcher(),
			DB:       stubBranches(),
			Args:     `{"query":"rate limit"}`,
			Limit:    _searchLimitDefault,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `","document_name":"Runbook",` + knownFields + hit + `}]}`,
			Searches: 1,
			Fetches:  1,
		},
		"Error returned by db.FetchDocumentBranchesByOrganizationID": {
			Searcher: hitSearcher(),
			DB: &DBMock{
				FetchDocumentBranchesByOrganizationIDFunc: func(context.Context, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Args:     `{"query":"rate limit"}`,
			Limit:    _searchLimitDefault,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
			Logged:   "cannot fetch the branches for search hits",
			Searches: 1,
			Fetches:  1,
		},
		// the name's entry is a flag on its branch, never a hit, so a
		// title-only match lists no hits.
		"Title match is flagged": {
			Searcher: &SearcherMock{
				SearchDocumentBlocksFunc: func(context.Context, string, string, int) ([]search.Block, error) {
					return []search.Block{
						{DocumentID: docID, BranchID: mainID, BranchName: "main", BranchDefault: true, ID: mainID.String() + "-docname", Type: "document", Text: "Runbook"},
					}, nil
				},
			},
			DB:    stubBranches(),
			Args:  `{"query":"runbook"}`,
			Limit: _searchLimitDefault,
			Result: `{"documents":[{"document_id":"` + docID.String() + `","document_name":"Runbook","branch_id":"` + mainID.String() + `",` +
				`"branch_name":"main","default":true,"protected":true,"title_match":true,"hits":[]}]}`,
			Searches: 1,
			Fetches:  1,
		},
		"Requested limit is honoured": {
			Searcher: hitSearcher(),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5}`,
			Limit:    5,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
			Searches: 1,
			Fetches:  1,
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
			Searches: 1,
			Fetches:  1,
		},
		"Oversized limit is clipped": {
			Searcher: hitSearcher(),
			DB:       &DBMock{},
			Args:     `{"query":"rate limit","limit":5000}`,
			Limit:    _searchLimitMax,
			Result:   `{"documents":[{"document_id":"` + docID.String() + `",` + branchFields + hit + `}]}`,
			Searches: 1,
			Fetches:  1,
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

			res, err := searchDocuments{}.Execute(d.newInput(context.Background(), NameSearchDocuments, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.Searcher.SearchDocumentBlocksCalls(), c.Searches)
			assert.Len(t, c.DB.FetchDocumentBranchesByOrganizationIDCalls(), c.Fetches)

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
