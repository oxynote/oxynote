package tools

import (
	"errors"
	"fmt"
	"log/slog"
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

// searchDocuments runs a full-text search across the organisation.
type searchDocuments struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (searchDocuments) Info() Info {
	return Info{
		Name:        NameSearchDocuments,
		Description: "Full-text search across all documents in the organisation for blocks whose text matches the query. Returns hits with document_id, document_name, block_uid, text. Use this to find relevant context before editing or to surface candidate documents the user might be asking about.",
		Properties: map[string]any{
			"query": stringProp("The full-text search query (typo-tolerant, matches block text)."),
			"limit": map[string]any{
				_keyType:        "integer",
				_keyDescription: "Maximum number of hits to return. Defaults to 20; cap is 50.",
			},
		},
		Required: []string{"query"},
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

	if in.Query == "" {
		return "Searching documents", nil
	}

	return fmt.Sprintf("Searching for %q", in.Query), nil
}

// Execute searches the index and decorates hits with document names.
func (searchDocuments) Execute(inp Input) (string, error) {
	var in searchDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Query == "" {
		return "", errors.New("search_documents: query is required")
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
	names := map[string]string{}

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
				names[d.ID.String()] = d.DocumentName
			}
		}
	}

	hits := make([]searchHit, 0, len(blocks))

	for _, b := range blocks {
		hits = append(hits, searchHit{
			DocumentID:   b.DocumentID.String(),
			DocumentName: names[b.DocumentID.String()],
			BlockUID:     b.ID,
			Text:         b.Text,
		})
	}

	return result(struct {
		Hits []searchHit `json:"hits"`
	}{
		Hits: hits,
	})
}

// searchHit is one search_documents result row.
type searchHit struct {
	// DocumentID is the document containing the matching block.
	DocumentID string `json:"document_id"`

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
