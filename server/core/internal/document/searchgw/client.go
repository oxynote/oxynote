// Package searchgw indexes and searches documents through Meilisearch.
package searchgw

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

//go:embed static/synonyms.json
var synonymsFile []byte

// _documentsIndex is the name of the documents index.
const _documentsIndex = "documents"

const (
	// _searchCropLength is the number of words kept around a match in
	// cropped search results.
	_searchCropLength = 5

	// _searchResultLimit caps the number of search hits returned.
	_searchResultLimit = 20
)

// Client is a Meilisearch client wrapper.
type Client struct {
	meiliMan meilisearch.ServiceManager
}

// NewClient creates a new Meilisearch client wrapper.
func NewClient(
	ctx context.Context,
	meiliMan meilisearch.ServiceManager,
) (*Client, error) {
	c := &Client{
		meiliMan: meiliMan,
	}

	if err := c.ensureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensuring documents index: %w", err)
	}

	return c, nil
}

// ensureIndex ensures the documents index exists.
func (c *Client) ensureIndex(ctx context.Context) error {
	_, err := c.meiliMan.GetIndexWithContext(ctx, _documentsIndex)

	var merr *meilisearch.Error

	switch {
	case err == nil:
		return nil
	case errors.As(err, &merr):
		if merr.MeilisearchApiError.Code == "index_not_found" {
			break
		}

		fallthrough
	default:
		return fmt.Errorf("getting documents index: %w", err)
	}

	_, err = c.meiliMan.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        _documentsIndex,
		PrimaryKey: "id",
	})
	if err != nil {
		return fmt.Errorf("creating documents index: %w", err)
	}

	_, err = c.meiliMan.Index(_documentsIndex).UpdateFilterableAttributesWithContext(ctx, &[]any{
		"organizationId",
	})
	if err != nil {
		return fmt.Errorf("updating filterable attributes: %w", err)
	}

	if err := c.setupSynonyms(ctx, _documentsIndex); err != nil {
		return fmt.Errorf("setting up synonyms: %w", err)
	}

	return nil
}

// setupSynonyms sets up synonyms for the documents index.
func (c *Client) setupSynonyms(ctx context.Context, index string) error {
	var synonyms map[string][]string

	if err := json.Unmarshal(synonymsFile, &synonyms); err != nil {
		return fmt.Errorf("unmarshaling synonyms: %w", err)
	}

	_, err := c.meiliMan.Index(index).UpdateSynonymsWithContext(ctx, &synonyms)
	if err != nil {
		return fmt.Errorf("updating synonyms: %w", err)
	}

	return nil
}

// SearchDocuments searches documents in the index.
func (c *Client) SearchDocuments(ctx context.Context, organizationID, query string) ([]byte, error) {
	res, err := c.meiliMan.Index(_documentsIndex).SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		AttributesToSearchOn:  []string{"text"},
		AttributesToCrop:      []string{"text"},
		AttributesToHighlight: []string{"text"},
		Filter:                fmt.Sprintf("organizationId = %q", organizationID),
		HighlightPreTag:       "<mark>",
		HighlightPostTag:      "</mark>",
		CropLength:            _searchCropLength,
		Limit:                 _searchResultLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching documents: %w", err)
	}

	hits := make([]json.RawMessage, 0, len(res.Hits))

	for _, hit := range res.Hits {
		hits = append(
			hits,
			hit["_formatted"],
		)
	}

	data, err := json.Marshal(hits)
	if err != nil {
		return nil, fmt.Errorf("marshaling search results: %w", err)
	}

	return data, nil
}

// SearchDocumentBlocks returns the raw matching blocks for the
// query, scoped to the organization. Unlike SearchDocuments it
// returns plain block values (no highlight markup or cropping) so
// callers — the AI assistant's search tool — can consume the text
// directly.
func (c *Client) SearchDocumentBlocks(ctx context.Context, organizationID, query string, limit int) ([]Block, error) {
	res, err := c.meiliMan.Index(_documentsIndex).SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		AttributesToSearchOn: []string{"text"},
		Filter:               fmt.Sprintf("organizationId = %q", organizationID),
		Limit:                int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("searching document blocks: %w", err)
	}

	blocks := make([]Block, 0, len(res.Hits))

	for _, hit := range res.Hits {
		var b Block

		if err := hit.DecodeInto(&b); err != nil {
			return nil, fmt.Errorf("decoding search hit: %w", err)
		}

		blocks = append(blocks, b)
	}

	return blocks, nil
}

// ReplaceDocumentBlocks replaces document blocks in the index based on the
// provided differences.
func (c *Client) ReplaceDocumentBlocks(ctx context.Context, bd BlocksDifference) error {
	if len(bd.Added) != 0 {
		_, err := c.meiliMan.Index(_documentsIndex).AddDocumentsWithContext(ctx, bd.Added, nil)
		if err != nil {
			return fmt.Errorf("adding documents: %w", err)
		}
	}

	if len(bd.Updated) != 0 {
		_, err := c.meiliMan.Index(_documentsIndex).UpdateDocumentsWithContext(ctx, bd.Updated, nil)
		if err != nil {
			return fmt.Errorf("updating documents: %w", err)
		}
	}

	if len(bd.Removed) != 0 {
		ids := make([]string, 0, len(bd.Removed))

		for _, block := range bd.Removed {
			ids = append(ids, block.ID)
		}

		_, err := c.meiliMan.Index(_documentsIndex).DeleteDocumentsWithContext(ctx, ids, nil)
		if err != nil {
			return fmt.Errorf("deleting documents: %w", err)
		}
	}

	return nil
}
