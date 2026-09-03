package tools

import (
	"fmt"
	"log/slog"

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
		return errRequired(_keyQuery)
	}

	return nil
}

// searchDocuments runs a full-text search across the organisation.
type searchDocuments struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (searchDocuments) Info() Info {
	return Info{
		Name:        NameSearchDocuments,
		Description: "Full-text search across every document in the organisation for blocks whose text matches the query. Returns hits with document_id, document_name, block_uid and text. Use it to find where a topic is discussed or which documents the user might mean; use list_documents when you know the title. A hit names the innermost block holding the text, which may sit below anything read_document_summary lists; read_block resolves it.",
		Properties: map[string]any{
			_keyQuery: stringProp("The full-text search query (typo-tolerant, matches block text)."),
			"limit": map[string]any{
				_keyType:        "integer",
				_keyDescription: "Maximum number of hits to return. Defaults to 20; cap is 50.",
			},
		},
		Required: []string{_keyQuery},
	}
}

// Traits reports a plain read.
func (searchDocuments) Traits() Traits {
	return Traits{}
}

// Title announces the query being run.
func (searchDocuments) Title(inp DescribeInput) (string, error) {
	var in searchDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return fmt.Sprintf("Searching for %q", in.Query), nil
}

// Execute searches the index and decorates hits with document names.
func (searchDocuments) Execute(inp Input) (string, error) {
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

	blocks, err := inp.SearchBlocks(in.Query, limit)
	if err != nil {
		return "", fmt.Errorf("search_documents: search: %w", err)
	}

	// the index stores block text keyed by (document, block uid) but not
	// display names; join names in from the document tree so the AI can
	// talk about hits without a follow-up lookup per document. Zero hits
	// have nothing to decorate, so the fetch is skipped.
	names := map[xid.ID]string{}

	if len(blocks) > 0 {
		tree, terr := inp.DocumentTree()
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
				names[d.ID] = d.DocumentName
			}
		}
	}

	hits := make([]searchHit, 0, len(blocks))

	for _, b := range blocks {
		hits = append(hits, searchHit{
			DocumentID:   b.DocumentID,
			DocumentName: names[b.DocumentID],
			BlockUID:     b.ID,
			Text:         b.Text,
		})
	}

	return result(searchResult{
		Hits: hits,
	})
}

// searchResult is what search_documents returns.
type searchResult struct {
	// Hits are the matching blocks, in relevance order.
	Hits []searchHit `json:"hits"`
}

// searchHit is one search_documents result row.
type searchHit struct {
	// DocumentID is the document containing the matching block.
	DocumentID xid.ID `json:"document_id"`

	// DocumentName is the document's display name, when resolvable from
	// the tree. Empty if the document vanished between the index update
	// and the search.
	DocumentName string `json:"document_name,omitempty"`

	// BlockUID is the matching block's uid attribute, usable with
	// read_block and the edit tools.
	BlockUID string `json:"block_uid"`

	// Text is the block's indexed text.
	Text string `json:"text"`
}
