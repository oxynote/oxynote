package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/document"
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

// searchDocuments runs a full-text search across the organisation.
type searchDocuments struct{ *Input }

// Info returns the tool's model-facing description.
func (t *searchDocuments) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameSearchDocuments,
		"Full-text search across all documents in the organisation for blocks whose text matches the query. Returns hits with document_id, document_name, block_uid, text. Use this to find relevant context before editing or to surface candidate documents the user might be asking about.",
		map[string]any{
			"query": stringProp("The full-text search query (typo-tolerant, matches block text)."),
			"limit": map[string]any{
				_keyType:        "integer",
				_keyDescription: "Maximum number of hits to return. Defaults to 20; cap is 50.",
			},
		},
		"query",
	)
}

// Label announces the query being run.
func (t *searchDocuments) Label(_ context.Context, args json.RawMessage) string {
	var in struct {
		Query string `json:"query"`
	}

	t.parseToolArgs(args, &in)

	if in.Query == "" {
		return "Searching documents"
	}

	return fmt.Sprintf("Searching for %q", in.Query)
}

// InvokableRun searches the index and decorates hits with document names.
func (t *searchDocuments) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("search_documents: invalid input: %w", err)
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

	blocks, err := t.search.SearchDocumentBlocks(ctx, t.orgID, in.Query, limit)
	if err != nil {
		return "", fmt.Errorf("search_documents: search: %w", err)
	}

	// the index stores block text keyed by (document, block uid) but not
	// display names; join names in from the document tree so the AI can
	// talk about hits without a follow-up lookup per document. Zero hits
	// have nothing to decorate, so the fetch is skipped.
	names := map[string]string{}

	if len(blocks) > 0 {
		tree, terr := t.db.FetchDocumentTree(ctx, t.orgID)
		if terr != nil {
			// the names decorate the hits; losing them is not worth
			// failing the search over, but it should not pass unnoticed
			// either.
			t.log.Warn(
				"cannot fetch the document tree for search hit names",
				slog.String("error", terr.Error()),
			)
		} else {
			collectDocumentNames(tree, names)
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

// collectDocumentNames flattens the summary tree into an id → name map.
func collectDocumentNames(ss document.Summaries, out map[string]string) {
	for _, s := range ss {
		out[s.ID.String()] = s.DocumentName

		collectDocumentNames(s.Children, out)
	}
}
