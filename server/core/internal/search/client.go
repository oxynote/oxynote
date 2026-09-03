// Package search indexes and searches documents through Meilisearch.
package search

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/meilisearch/meilisearch-go"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

//go:embed static/synonyms.json
var synonymsFile []byte

// ErrNotConfigured is returned when search is not configured on this
// deployment.
var ErrNotConfigured = errutil.New(http.StatusConflict, "search.not_configured", "search is not configured")

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

	// _setupTimeout bounds the index setup at boot.
	_setupTimeout = time.Minute

	// _writeTimeout bounds one batch of index writes, task waits included.
	_writeTimeout = 2 * time.Minute

	// _searchTimeout bounds one search request.
	_searchTimeout = 10 * time.Second
)

// _displayedAttributes are the entry fields search hits carry.
var _displayedAttributes = []string{
	"id",
	"organizationId",
	"documentId",
	"branchId",
	"branchName",
	"branchDefault",
	"type",
	_textAttribute,
}

// _filterableAttributes are the entry fields searches and removals filter
// by.
var _filterableAttributes = []string{
	"organizationId",
	"documentId",
	"branchId",
}

// Client is a Meilisearch client wrapper.
type Client struct {
	meiliMan meilisearch.ServiceManager
}

// NewClient creates a new Meilisearch client wrapper. A nil meiliMan means
// search is not configured on this deployment: the client is returned
// without touching Meilisearch and every call on it reports
// ErrNotConfigured.
func NewClient(
	ctx context.Context,
	meiliMan meilisearch.ServiceManager,
) (*Client, error) {
	c := &Client{
		meiliMan: meiliMan,
	}

	if !c.Configured() {
		return c, nil
	}

	if err := c.ensureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensuring documents index: %w", err)
	}

	return c, nil
}

// Configured reports whether search is configured on this deployment.
func (c *Client) Configured() bool {
	return c.meiliMan != nil
}

// ensureIndex ensures the documents index exists and runs with the
// declared settings.
func (c *Client) ensureIndex(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, _setupTimeout)
	defer cancel()

	_, err := c.meiliMan.GetIndexWithContext(ctx, _documentsIndex)

	switch merr, ok := errors.AsType[*meilisearch.Error](err); {
	case err == nil:
		// the index is already there.
	case ok && merr.MeilisearchApiError.Code == "index_not_found":
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

	// the settings are checked on every boot rather than only when the
	// index is created: an index whose settings task failed would otherwise
	// stay broken for good, and new synonyms would never reach an existing
	// deployment. They are sent only when they differ, since a searchable
	// or filterable attribute change re-indexes every entry.
	want, err := indexSettings()
	if err != nil {
		return err
	}

	have, err := c.meiliMan.Index(_documentsIndex).GetSettingsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting settings: %w", err)
	}

	if settingsMatch(have, want) {
		return nil
	}

	task, err := c.meiliMan.Index(_documentsIndex).UpdateSettingsWithContext(ctx, want)
	if err != nil {
		return fmt.Errorf("updating settings: %w", err)
	}

	if err = c.awaitTask(ctx, task); err != nil {
		return fmt.Errorf("updating settings: %w", err)
	}

	return nil
}

// indexSettings returns the settings the documents index runs with. Only
// the fields it sets are sent; everything else keeps Meilisearch's default.
func indexSettings() (*meilisearch.Settings, error) {
	var synonyms map[string][]string

	if err := json.Unmarshal(synonymsFile, &synonyms); err != nil {
		return nil, fmt.Errorf("unmarshaling synonyms: %w", err)
	}

	return &meilisearch.Settings{
		SearchableAttributes: []string{_textAttribute},
		DisplayedAttributes:  _displayedAttributes,
		FilterableAttributes: _filterableAttributes,
		Synonyms:             synonyms,
	}, nil
}

// settingsMatch reports whether the index already runs with every field
// want sets. Attribute lists are compared as sets, since Meilisearch does
// not promise their order back.
func settingsMatch(have, want *meilisearch.Settings) bool {
	sameSet := func(a, b []string) bool {
		a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))

		return slices.Equal(a, b)
	}

	return sameSet(have.SearchableAttributes, want.SearchableAttributes) &&
		sameSet(have.DisplayedAttributes, want.DisplayedAttributes) &&
		sameSet(have.FilterableAttributes, want.FilterableAttributes) &&
		maps.EqualFunc(have.Synonyms, want.Synonyms, sameSet)
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
		return fmt.Errorf(
			"task %d finished with status %q: %s (%s)",
			info.TaskUID,
			task.Status,
			task.Error.Message,
			task.Error.Code,
		)
	}

	return nil
}

// settle reports the outcome of one index write: the request error when the
// call itself failed, otherwise the task's result once Meilisearch has
// applied it.
func (c *Client) settle(ctx context.Context, what string, task *meilisearch.TaskInfo, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	if err := c.awaitTask(ctx, task); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	return nil
}

// SearchDocuments searches documents in the index.
func (c *Client) SearchDocuments(ctx context.Context, organizationID, query string) ([]byte, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, _searchTimeout)
	defer cancel()

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
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, _searchTimeout)
	defer cancel()

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
// provided differences. Every write is awaited, so a batch Meilisearch
// rejects is reported rather than silently dropped.
func (c *Client) ReplaceDocumentBlocks(ctx context.Context, bd BlocksDifference) error {
	if !c.Configured() {
		return ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, _writeTimeout)
	defer cancel()

	idx := c.meiliMan.Index(_documentsIndex)

	// every entry is complete, so added and updated ones alike are sent as
	// one add-or-replace request.
	if docs := slices.Concat(bd.Added, bd.Updated); len(docs) != 0 {
		task, err := idx.AddDocumentsWithContext(ctx, docs, nil)

		if err = c.settle(ctx, "adding documents", task, err); err != nil {
			return err
		}
	}

	if len(bd.Removed) != 0 {
		ids := make([]string, 0, len(bd.Removed))

		for _, block := range bd.Removed {
			ids = append(ids, block.ID)
		}

		task, err := idx.DeleteDocumentsWithContext(ctx, ids, nil)

		if err = c.settle(ctx, "deleting documents", task, err); err != nil {
			return err
		}
	}

	if len(bd.RemovedBranches) != 0 {
		ids := make([]string, 0, len(bd.RemovedBranches))

		for _, br := range bd.RemovedBranches {
			ids = append(ids, br.BranchID.String())
		}

		if err := c.deleteByFilter(ctx, idx, "branchId", ids); err != nil {
			return fmt.Errorf("deleting branches by filter: %w", err)
		}
	}

	if len(bd.RemovedDocuments) != 0 {
		ids := make([]string, 0, len(bd.RemovedDocuments))

		for _, id := range bd.RemovedDocuments {
			ids = append(ids, id.String())
		}

		if err := c.deleteByFilter(ctx, idx, "documentId", ids); err != nil {
			return fmt.Errorf("deleting documents by filter: %w", err)
		}
	}

	if len(bd.RemovedOrganizations) != 0 {
		if err := c.deleteByFilter(ctx, idx, "organizationId", bd.RemovedOrganizations); err != nil {
			return fmt.Errorf("deleting organizations by filter: %w", err)
		}
	}

	return nil
}

// deleteByFilter removes every entry whose field is one of the ids and
// waits for the removal to apply.
func (c *Client) deleteByFilter(ctx context.Context, idx meilisearch.IndexManager, field string, ids []string) error {
	quoted := make([]string, 0, len(ids))

	for _, id := range ids {
		quoted = append(quoted, strconv.Quote(id))
	}

	filter := fmt.Sprintf("%s IN [%s]", field, strings.Join(quoted, ", "))

	task, err := idx.DeleteDocumentsByFilterWithContext(ctx, filter, nil)

	return c.settle(ctx, "deleting by "+field, task, err)
}
