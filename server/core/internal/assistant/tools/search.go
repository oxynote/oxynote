package tools

import (
	"fmt"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
)

const (
	// _searchLimitDefault is the cap applied when search_documents is
	// called without a limit argument.
	_searchLimitDefault = 20

	// _searchLimitMax is the hard cap; larger requested values are
	// silently clipped so the AI can't accidentally page through
	// thousands of rows.
	_searchLimitMax = 50
)

// searchDocumentsArgs is what search_documents is called with.
type searchDocumentsArgs struct {
	// Query is the text being searched for. Required.
	Query string `json:"query"`

	// Limit caps the number of hits. Zero takes the default.
	Limit int `json:"limit"`
}

// Validate checks the arguments are complete.
func (a searchDocumentsArgs) Validate() error {
	if a.Query == "" {
		return errRequired("query")
	}

	if err := search.ValidateQuery(a.Query); err != nil {
		return fmt.Errorf("query must be at most %d characters; search for the key terms instead", search.MaxQueryLength)
	}

	return nil
}

// searchDocuments runs a full-text search across the organisation.
type searchDocuments struct{}

// Info returns the tool's model-facing description.
func (searchDocuments) Info() Info {
	return Info{
		Name:        NameSearchDocuments,
		Description: "Full-text search for blocks whose text matches the query, across every branch of every document. Returns {documents: [{document_id, document_name, branch_id, branch_name, default, hits: [{block_uid, text}]}]}, best match first. A block a fork copied unchanged is found on every branch holding it. A hit is the innermost block holding the text; get_document with its block_uid reads the element holding it. Use list_documents to find a document by title.",
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "The text to search for; matching is typo-tolerant."},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional. The most hits to return: 20 by default, 50 at most.",
			},
		},
		Required: []string{"query"},
	}
}

// Title announces the query being run.
func (searchDocuments) Title(inp DescribeInput) string {
	var in searchDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	return fmt.Sprintf("Searching for %q", in.Query)
}

// Execute searches the index and decorates hits with document names.
func (searchDocuments) Execute(inp *input) (string, error) {
	var in searchDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	limit := in.Limit
	if limit <= 0 {
		limit = _searchLimitDefault
	}

	if limit > _searchLimitMax {
		limit = _searchLimitMax
	}

	blocks, err := inp.search.SearchDocumentBlocks(inp.Context(), inp.orgID, in.Query, limit)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	// the index stores block text keyed by (branch, block uid) but not
	// document names; join names in from the document tree so the AI can
	// talk about hits without a follow-up lookup per document. Zero hits
	// have nothing to decorate, so the fetch is skipped.
	names := map[xid.ID]document.Summary{}

	if len(blocks) > 0 {
		tree, terr := inp.FetchDocumentTree()
		if terr != nil {
			// the names decorate the hits; losing them is not worth
			// failing the search over, but it should not pass unnoticed
			// either.
			inp.Warn(
				"cannot fetch the document tree for search hit names",
				slog.String("error", terr.Error()),
			)
		} else {
			for _, d := range tree.Descendants() {
				names[d.ID] = d
			}
		}
	}

	// hits of one branch are listed under it once, in the order its
	// first hit ranked.
	var (
		out   = searchResult{Documents: []searchDocument{}}
		index = map[xid.ID]int{}
	)

	for _, b := range blocks {
		at, seen := index[b.BranchID]
		if !seen {
			at = len(out.Documents)
			index[b.BranchID] = at

			out.Documents = append(out.Documents, searchDocument{
				DocumentID:   b.DocumentID,
				DocumentName: names[b.DocumentID].DocumentName,
				BranchID:     b.BranchID,
				BranchName:   b.BranchName,
				Default:      b.BranchDefault,
			})
		}

		out.Documents[at].Hits = append(out.Documents[at].Hits, searchHit{
			BlockUID: b.UID(),
			Text:     b.Text,
		})
	}

	return result(out)
}

// searchResult is what search_documents returns.
type searchResult struct {
	// Documents are the document branches holding a match, in the order
	// of their best hit.
	Documents []searchDocument `json:"documents"`
}

// searchDocument is one document branch holding search hits.
type searchDocument struct {
	// DocumentID is the document containing the matching blocks.
	DocumentID xid.ID `json:"document_id"`

	// DocumentName is the document's display name, when resolvable from
	// the tree. Empty if the document vanished between the index update
	// and the search.
	DocumentName string `json:"document_name,omitempty"`

	// BranchID is the branch the hits were indexed from, which is what a
	// follow-up read takes.
	BranchID xid.ID `json:"branch_id"`

	// BranchName is that branch's name.
	BranchName string `json:"branch_name"`

	// Default reports whether that branch is the document's default.
	Default bool `json:"default"`

	// Hits are the matching blocks of the branch, in relevance order.
	Hits []searchHit `json:"hits"`
}

// searchHit is one matching block.
type searchHit struct {
	// BlockUID is the matching block's uid attribute, usable with
	// get_document and the edit tools.
	BlockUID string `json:"block_uid"`

	// Text is the block's indexed text.
	Text string `json:"text"`
}
