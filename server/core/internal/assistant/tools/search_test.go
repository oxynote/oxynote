package tools

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_searchDocuments_Info(t *testing.T) {
	t.Parallel()

	info := searchDocuments{}.Info()

	assert.Equal(t, Traits{Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameSearchDocuments, info.Name)
	assert.Equal(t, []string{"query"}, info.Required)
	assert.Contains(t, info.Properties, "limit")
}

func Test_searchDocuments_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool searchDocuments
		Err  error
	}{
		"Query is required": {
			Tool: searchDocuments{},
			Err:  errors.New("query is required"),
		},
		"Query over the length cap": {
			Tool: searchDocuments{Query: strings.Repeat("a", search.MaxQueryLength+1)},
			Err:  fmt.Errorf("query must be at most %d characters; search for the key terms instead", search.MaxQueryLength),
		},
		"Valid arguments": {
			Tool: searchDocuments{Query: "q"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_searchDocuments_Title(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `Searching for "rate limit"`, searchDocuments{Query: "rate limit"}.Title(&InputMock{}))
}

func Test_searchDocuments_Exec(t *testing.T) {
	t.Parallel()

	docID, branchID, mainID := xid.New(), xid.New(), xid.New()

	// the draft takes writes. The main branch is protected.
	branches := map[xid.ID][]document.BranchSummary{
		docID: {
			{BranchID: mainID, DocumentID: docID, BranchName: "main", DocumentName: "Runbook", Default: true, Protected: true},
			{BranchID: branchID, DocumentID: docID, BranchName: "draft", DocumentName: "Runbook draft"},
		},
	}
	hit := search.Block{DocumentID: docID, BranchID: branchID, BranchName: "draft", ID: branchID.String() + "-b1", Text: "rate limiting"}
	hitDocument := func() searchDocument {
		return searchDocument{
			DocumentID: docID,
			BranchID:   branchID,
			BranchName: "draft",
			Hits:       []searchHit{{BlockUID: "b1", Text: "rate limiting"}},
		}
	}

	cc := map[string]struct {
		Blocks   []search.Block
		Search   error
		Branches error
		Tool     searchDocuments
		Limit    int
		Result   any
		Warns    int
		Fetches  int
		Err      error
	}{
		"Error returned by Input.SearchDocumentBlocks": {
			Search: assert.AnError,
			Tool:   searchDocuments{Query: "rate limit"},
			Limit:  _searchLimitDefault,
			Err:    assert.AnError,
		},
		"Zero hits skip the name lookup": {
			Tool:   searchDocuments{Query: "rate limit"},
			Limit:  _searchLimitDefault,
			Result: searchDocumentsResult{Documents: []searchDocument{}},
		},
		// the document goes by its default branch's name, and the hit's
		// branch says whether it takes writes.
		"Names and protection are joined in": {
			Blocks:  []search.Block{hit},
			Tool:    searchDocuments{Query: "rate limit"},
			Limit:   _searchLimitDefault,
			Fetches: 1,
			Result: searchDocumentsResult{Documents: []searchDocument{func() searchDocument {
				d := hitDocument()
				d.DocumentName = "Runbook"
				d.Protected = new(false)

				return d
			}()}},
		},
		// the names decorate the hits, so losing them is warned about
		// rather than failing the search.
		"Error returned by Input.FetchBranchesByDocument": {
			Blocks:   []search.Block{hit},
			Branches: assert.AnError,
			Tool:     searchDocuments{Query: "rate limit"},
			Limit:    _searchLimitDefault,
			Fetches:  1,
			Warns:    1,
			Result:   searchDocumentsResult{Documents: []searchDocument{hitDocument()}},
		},
		// the name's entry is a flag on its branch, never a hit, so a
		// title-only match lists no hits.
		"Title match is flagged": {
			Blocks:  []search.Block{{DocumentID: docID, BranchID: mainID, BranchName: "main", BranchDefault: true, ID: mainID.String() + "-docname", Type: "document", Text: "Runbook"}},
			Tool:    searchDocuments{Query: "runbook"},
			Limit:   _searchLimitDefault,
			Fetches: 1,
			Result: searchDocumentsResult{Documents: []searchDocument{{
				DocumentID:   docID,
				DocumentName: "Runbook",
				BranchID:     mainID,
				BranchName:   "main",
				Default:      true,
				Protected:    new(true),
				TitleMatch:   true,
				Hits:         []searchHit{},
			}}},
		},
		// hits of one branch sit under it once, in the order its best hit
		// ranked. Another branch of the same document is its own entry.
		"Hits are grouped by branch": {
			Blocks: []search.Block{
				{DocumentID: docID, BranchID: branchID, BranchName: "draft", ID: branchID.String() + "-b1", Text: "one"},
				{DocumentID: docID, BranchID: mainID, BranchName: "main", BranchDefault: true, ID: mainID.String() + "-b2", Text: "two"},
				{DocumentID: docID, BranchID: branchID, BranchName: "draft", ID: branchID.String() + "-b3", Text: "three"},
			},
			Branches: assert.AnError,
			Tool:     searchDocuments{Query: "rate limit"},
			Limit:    _searchLimitDefault,
			Fetches:  1,
			Warns:    1,
			Result: searchDocumentsResult{Documents: []searchDocument{
				{DocumentID: docID, BranchID: branchID, BranchName: "draft", Hits: []searchHit{{BlockUID: "b1", Text: "one"}, {BlockUID: "b3", Text: "three"}}},
				{DocumentID: docID, BranchID: mainID, BranchName: "main", Default: true, Hits: []searchHit{{BlockUID: "b2", Text: "two"}}},
			}},
		},
		"Requested limit is honoured": {
			Tool:   searchDocuments{Query: "rate limit", Limit: 5},
			Limit:  5,
			Result: searchDocumentsResult{Documents: []searchDocument{}},
		},
		"Oversized limit is clipped": {
			Tool:   searchDocuments{Query: "rate limit", Limit: 5000},
			Limit:  _searchLimitMax,
			Result: searchDocumentsResult{Documents: []searchDocument{}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				SearchDocumentBlocksFunc: func(string, int) ([]search.Block, error) {
					return c.Blocks, c.Search
				},
				FetchBranchesByDocumentFunc: func() (map[xid.ID][]document.BranchSummary, error) {
					if c.Branches != nil {
						return nil, c.Branches
					}

					return branches, nil
				},
			}

			res, err := c.Tool.Exec(inp)
			testutil.AssertEqualError(t, c.Err, err)

			ff := inp.SearchDocumentBlocksCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Tool.Query, ff[0].Query)
			assert.Equal(t, c.Limit, ff[0].Limit)

			assert.Len(t, inp.FetchBranchesByDocumentCalls(), c.Fetches)
			assert.Len(t, inp.WarnCalls(), c.Warns)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
