// Package search indexes and searches documents through Meilisearch.
package search

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

//go:embed static/synonyms.json
var synonymsFile []byte

// _documentsIndex is the name of the documents index.
const _documentsIndex = "documents"

// _textAttribute is the indexed block attribute searched, cropped, and
// highlighted.
const _textAttribute = "text"

const (
	// _searchCropLength is the number of words kept around a match in
	// cropped search results.
	_searchCropLength = 5

	// _searchResultLimit caps the number of search hits returned.
	_searchResultLimit = 20

	// _taskPollInterval is how often an asynchronous Meilisearch task is
	// polled while waiting for it to settle.
	_taskPollInterval = 50 * time.Millisecond
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
		// the index is already there.
	case errors.As(err, &merr) && merr.MeilisearchApiError.Code == "index_not_found":
		task, cerr := c.meiliMan.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
			Uid:        _documentsIndex,
			PrimaryKey: "id",
		})
		if cerr != nil {
			return fmt.Errorf("creating documents index: %w", cerr)
		}

		if cerr = c.awaitTask(ctx, task); cerr != nil {
			return fmt.Errorf("creating documents index: %w", cerr)
		}
	default:
		return fmt.Errorf("getting documents index: %w", err)
	}

	// the settings updates are idempotent and are therefore applied on every
	// boot. Applying them only when the index is created leaves an index
	// whose settings task failed broken for good — with organizationId not
	// filterable every search fails — and never rolls new synonyms out to an
	// existing deployment.
	task, err := c.meiliMan.Index(_documentsIndex).UpdateFilterableAttributesWithContext(ctx, &[]any{
		"organizationId",
		"documentId",
	})
	if err != nil {
		return fmt.Errorf("updating filterable attributes: %w", err)
	}

	if err = c.awaitTask(ctx, task); err != nil {
		return fmt.Errorf("updating filterable attributes: %w", err)
	}

	if err = c.setupSynonyms(ctx, _documentsIndex); err != nil {
		return fmt.Errorf("setting up synonyms: %w", err)
	}

	return nil
}

// awaitTask blocks until an asynchronous Meilisearch task settles and reports
// a rejected task as an error, so a failed index or settings update surfaces
// at boot instead of leaving the index quietly misconfigured.
func (c *Client) awaitTask(ctx context.Context, info *meilisearch.TaskInfo) error {
	task, err := c.meiliMan.WaitForTaskWithContext(ctx, info.TaskUID, _taskPollInterval)
	if err != nil {
		return fmt.Errorf("waiting for task %d: %w", info.TaskUID, err)
	}

	if task.Status != meilisearch.TaskStatusSucceeded {
		return fmt.Errorf("task %d finished with status %q", info.TaskUID, task.Status)
	}

	return nil
}

// setupSynonyms sets up synonyms for the documents index.
func (c *Client) setupSynonyms(ctx context.Context, index string) error {
	var synonyms map[string][]string

	if err := json.Unmarshal(synonymsFile, &synonyms); err != nil {
		return fmt.Errorf("unmarshaling synonyms: %w", err)
	}

	task, err := c.meiliMan.Index(index).UpdateSynonymsWithContext(ctx, &synonyms)
	if err != nil {
		return fmt.Errorf("updating synonyms: %w", err)
	}

	if err = c.awaitTask(ctx, task); err != nil {
		return fmt.Errorf("updating synonyms: %w", err)
	}

	return nil
}

// SearchDocuments searches documents in the index.
func (c *Client) SearchDocuments(ctx context.Context, organizationID, query string) ([]byte, error) {
	res, err := c.meiliMan.Index(_documentsIndex).SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		AttributesToSearchOn:  []string{_textAttribute},
		AttributesToCrop:      []string{_textAttribute},
		AttributesToHighlight: []string{_textAttribute},
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
		AttributesToSearchOn: []string{_textAttribute},
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

	if len(bd.RemovedDocuments) != 0 {
		ids := make([]string, 0, len(bd.RemovedDocuments))

		for _, id := range bd.RemovedDocuments {
			ids = append(ids, strconv.Quote(id.String()))
		}

		filter := fmt.Sprintf("documentId IN [%s]", strings.Join(ids, ", "))

		_, err := c.meiliMan.Index(_documentsIndex).DeleteDocumentsByFilterWithContext(ctx, filter, nil)
		if err != nil {
			return fmt.Errorf("deleting documents by filter: %w", err)
		}
	}

	if len(bd.RemovedOrganizations) != 0 {
		ids := make([]string, 0, len(bd.RemovedOrganizations))

		for _, id := range bd.RemovedOrganizations {
			ids = append(ids, strconv.Quote(id))
		}

		filter := fmt.Sprintf("organizationId IN [%s]", strings.Join(ids, ", "))

		_, err := c.meiliMan.Index(_documentsIndex).DeleteDocumentsByFilterWithContext(ctx, filter, nil)
		if err != nil {
			return fmt.Errorf("deleting organizations by filter: %w", err)
		}
	}

	return nil
}
