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
		Description: "Full-text search of document titles and block text, across every branch of every document. Returns {documents: [{document_id, document_name, branch_id, branch_name, default, protected, title_match, hits: [{block_uid, text}]}]}, best match first. A title or block matches when it holds all but one of the query's words, typos allowed, so search for a title or a phrase, not both, and check document_name. title_match says the title matched. A hit is the innermost block holding the text, and get_document with its block_uid reads the element holding it. A block a fork copied unchanged is found on every branch holding it.",
		Properties: map[string]any{
			"query": map[string]any{"type": "string", "description": "The title or phrase to search for."},
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

	// the index holds neither document names nor branch protection; both
	// are joined in from the organisation's branches, so the model can
	// talk about hits and pick a branch to write to without a lookup per
	// document. Zero hits have nothing to decorate, so the fetch is skipped.
	branches := map[xid.ID][]document.BranchSummary{}

	if len(blocks) > 0 {
		found, berr := inp.FetchBranchesByDocument()
		if berr != nil {
			// the names and flags decorate the hits; losing them is not
			// worth failing the search over, but it should not pass
			// unnoticed either.
			inp.Warn(
				"cannot fetch the branches for search hits",
				slog.String("error", berr.Error()),
			)
		} else {
			branches = found
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

			entry := searchDocument{
				DocumentID: b.DocumentID,
				BranchID:   b.BranchID,
				BranchName: b.BranchName,
				Default:    b.BranchDefault,
				Hits:       []searchHit{},
			}

			// the document goes by its default branch's name, as in the
			// sidebar.
			for _, br := range branches[b.DocumentID] {
				if br.Default {
					entry.DocumentName = br.DocumentName
				}

				if br.BranchID == b.BranchID {
					entry.Protected = &br.Protected
				}
			}

			out.Documents = append(out.Documents, entry)
		}

		// the name's entry is not a block, so it is a flag rather than a
		// hit get_document could not read.
		if b.IsDocumentName() {
			out.Documents[at].TitleMatch = true

			continue
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
	// its branches. Empty if the document vanished between the index
	// update and the search.
	DocumentName string `json:"document_name,omitempty"`

	// BranchID is the branch the hits were indexed from, which is what a
	// follow-up read takes.
	BranchID xid.ID `json:"branch_id"`

	// BranchName is that branch's name.
	BranchName string `json:"branch_name"`

	// Default reports whether that branch is the document's default.
	Default bool `json:"default"`

	// Protected reports whether that branch refuses writes. It is left
	// out when the branches could not be read.
	Protected *bool `json:"protected,omitempty"`

	// TitleMatch reports whether the document's name matched the query.
	TitleMatch bool `json:"title_match,omitempty"`

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
