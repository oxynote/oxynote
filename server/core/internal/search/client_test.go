package search

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexNotFoundErr builds the meilisearch error ensureIndex treats as a
// missing index.
func indexNotFoundErr() *meilisearch.Error {
	merr := &meilisearch.Error{}
	merr.MeilisearchApiError.Code = "index_not_found"

	return merr
}

// stubManagers wires a service-manager mock whose Index method returns the
// given index-manager mock.
func stubManagers(idx *mock.MeiliIndexManager) *mock.MeiliServiceManager {
	return &mock.MeiliServiceManager{
		IndexFunc: func(string) meilisearch.IndexManager {
			return idx
		},
	}
}

func Test_NewClient(t *testing.T) {
	t.Parallel()

	// a nil service manager is the disabled mode: nothing is set up and
	// every call refuses with the sentinel.
	t.Run("Unconfigured client", func(t *testing.T) {
		t.Parallel()

		c, err := NewClient(context.Background(), nil)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.False(t, c.Configured())

		data, serr := c.SearchDocuments(context.Background(), "org1", "query")
		assert.ErrorIs(t, serr, ErrNotConfigured)
		assert.Nil(t, data)

		blocks, serr := c.SearchDocumentBlocks(context.Background(), "org1", "query", 5)
		assert.ErrorIs(t, serr, ErrNotConfigured)
		assert.Nil(t, blocks)

		assert.ErrorIs(t, c.ReplaceDocumentBlocks(context.Background(), BlocksDifference{}), ErrNotConfigured)
	})

	type check func(*testing.T, *mock.MeiliServiceManager, *mock.MeiliIndexManager, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(expect bool) check {
		return func(t *testing.T, _ *mock.MeiliServiceManager, _ *mock.MeiliIndexManager, err error) {
			if expect {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		}
	}

	wasCreateIndexCalled := func(count int) check {
		return func(t *testing.T, svc *mock.MeiliServiceManager, _ *mock.MeiliIndexManager, _ error) {
			ff := svc.CreateIndexWithContextCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "documents", ff[0].Config.Uid)
			assert.Equal(t, "id", ff[0].Config.PrimaryKey)
		}
	}

	wasSettingsUpdated := func(count int) check {
		return func(t *testing.T, _ *mock.MeiliServiceManager, idx *mock.MeiliIndexManager, _ error) {
			ff := idx.UpdateSettingsWithContextCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, []string{"text"}, ff[0].Request.SearchableAttributes)
			assert.Equal(t, _displayedAttributes, ff[0].Request.DisplayedAttributes)
			assert.Equal(t, _filterableAttributes, ff[0].Request.FilterableAttributes)
			assert.NotEmpty(t, ff[0].Request.Synonyms)
		}
	}

	// current is what a settled index reports: the declared settings, with
	// the attribute lists in an order of Meilisearch's choosing.
	current := func() *meilisearch.Settings {
		want, err := indexSettings()
		require.NoError(t, err)

		have := *want
		have.DisplayedAttributes = slices.Clone(want.DisplayedAttributes)
		slices.Reverse(have.DisplayedAttributes)

		return &have
	}

	// fresh is what a just-created index reports.
	fresh := func() *meilisearch.Settings {
		return &meilisearch.Settings{
			SearchableAttributes: []string{"*"},
			DisplayedAttributes:  []string{"*"},
		}
	}

	tests := map[string]struct {
		GetIndexErr    error
		CreateErr      error
		Settings       *meilisearch.Settings
		GetSettingsErr error
		SettingsErr    error
		WaitErr        error
		TaskStatus     meilisearch.TaskStatus
		Checks         []check
	}{
		"Existing index with the declared settings is left alone": {
			Settings: current(),
			Checks: checks(
				hasError(false),
				wasCreateIndexCalled(0),
				wasSettingsUpdated(0),
			),
		},
		"Existing index with stale settings gets them updated": {
			Settings: func() *meilisearch.Settings {
				st := current()
				st.FilterableAttributes = []string{"organizationId", "documentId"}

				return st
			}(),
			Checks: checks(
				hasError(false),
				wasCreateIndexCalled(0),
				wasSettingsUpdated(1),
			),
		},
		"Existing index with stale synonyms gets them updated": {
			Settings: func() *meilisearch.Settings {
				st := current()
				st.Synonyms = map[string][]string{"2fa": {"mfa"}}

				return st
			}(),
			Checks: checks(
				hasError(false),
				wasSettingsUpdated(1),
			),
		},
		"Settings fetch failure fails": {
			GetSettingsErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasSettingsUpdated(0),
			),
		},
		"Rejected settings task fails": {
			Settings:   fresh(),
			TaskStatus: meilisearch.TaskStatusFailed,
			Checks: checks(
				hasError(true),
				wasCreateIndexCalled(0),
				wasSettingsUpdated(1),
			),
		},
		"Task wait failure fails": {
			Settings: fresh(),
			WaitErr:  assert.AnError,
			Checks: checks(
				hasError(true),
			),
		},
		"Missing index is created with settings": {
			GetIndexErr: indexNotFoundErr(),
			Settings:    fresh(),
			Checks: checks(
				hasError(false),
				wasCreateIndexCalled(1),
				wasSettingsUpdated(1),
			),
		},
		"Other meilisearch errors fail": {
			GetIndexErr: &meilisearch.Error{},
			Checks: checks(
				hasError(true),
				wasCreateIndexCalled(0),
			),
		},
		"Non-meilisearch errors fail": {
			GetIndexErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasCreateIndexCalled(0),
			),
		},
		"Index creation failure fails": {
			GetIndexErr: indexNotFoundErr(),
			CreateErr:   assert.AnError,
			Checks: checks(
				hasError(true),
				wasSettingsUpdated(0),
			),
		},
		"Settings update failure fails": {
			GetIndexErr: indexNotFoundErr(),
			Settings:    fresh(),
			SettingsErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasSettingsUpdated(1),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idx := &mock.MeiliIndexManager{
				GetSettingsWithContextFunc: func(context.Context) (*meilisearch.Settings, error) {
					return tc.Settings, tc.GetSettingsErr
				},
				UpdateSettingsWithContextFunc: func(context.Context, *meilisearch.Settings) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 2}, tc.SettingsErr
				},
			}

			svc := stubManagers(idx)
			svc.GetIndexWithContextFunc = func(context.Context, string) (*meilisearch.IndexResult, error) {
				return nil, tc.GetIndexErr
			}
			svc.CreateIndexWithContextFunc = func(context.Context, *meilisearch.IndexConfig) (*meilisearch.TaskInfo, error) {
				return &meilisearch.TaskInfo{TaskUID: 1}, tc.CreateErr
			}
			svc.WaitForTaskWithContextFunc = func(context.Context, int64, time.Duration) (*meilisearch.Task, error) {
				status := meilisearch.TaskStatusSucceeded
				if tc.TaskStatus != "" {
					status = tc.TaskStatus
				}

				return &meilisearch.Task{Status: status}, tc.WaitErr
			}

			client, err := NewClient(context.Background(), svc)

			for _, ch := range tc.Checks {
				ch(t, svc, idx, err)
			}

			if err == nil {
				assert.NotNil(t, client)
			}
		})
	}
}

// newTestClient builds a Client wired straight to the given index mock,
// bypassing ensureIndex. Every task the index reports settles with the
// given status.
func newTestClient(idx *mock.MeiliIndexManager, status meilisearch.TaskStatus) *Client {
	svc := stubManagers(idx)
	svc.WaitForTaskWithContextFunc = func(context.Context, int64, time.Duration) (*meilisearch.Task, error) {
		task := &meilisearch.Task{Status: status}
		task.Error.Message = "boom"
		task.Error.Code = "invalid_document_id"

		return task, nil
	}

	return &Client{meiliMan: svc}
}

func Test_Client_SearchDocuments(t *testing.T) {
	t.Parallel()

	t.Run("Formatted hits are returned as JSON", func(t *testing.T) {
		t.Parallel()

		var captured *meilisearch.SearchRequest

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(_ context.Context, _ string, request *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				captured = request

				return &meilisearch.SearchResponse{
					Hits: meilisearch.Hits{
						{"_formatted": json.RawMessage(`{"id": "b1", "text": "<mark>hello</mark>"}`)},
					},
				}, nil
			},
		}

		data, err := newTestClient(idx, meilisearch.TaskStatusSucceeded).SearchDocuments(context.Background(), "org-1", "hello")
		require.NoError(t, err)

		assert.JSONEq(t, `[{"id": "b1", "text": "<mark>hello</mark>"}]`, string(data))

		require.NotNil(t, captured)
		assert.Equal(t, `organizationId = "org-1"`, captured.Filter)
		assert.Equal(t, []string{"text"}, captured.AttributesToSearchOn)
		assert.Equal(t, int64(5), captured.CropLength)
		assert.Equal(t, int64(20), captured.Limit)
		assert.Equal(t, "<mark>", captured.HighlightPreTag)
	})

	t.Run("Search failure is propagated", func(t *testing.T) {
		t.Parallel()

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				return nil, assert.AnError
			},
		}

		_, err := newTestClient(idx, meilisearch.TaskStatusSucceeded).SearchDocuments(context.Background(), "org-1", "hello")
		require.Error(t, err)
	})
}

func Test_Client_SearchDocumentBlocks(t *testing.T) {
	t.Parallel()

	documentID := xid.New()

	t.Run("Hits are decoded into blocks", func(t *testing.T) {
		t.Parallel()

		var captured *meilisearch.SearchRequest

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(_ context.Context, _ string, request *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				captured = request

				return &meilisearch.SearchResponse{
					Hits: meilisearch.Hits{
						{
							"id":             json.RawMessage(`"b1"`),
							"organizationId": json.RawMessage(`"org-1"`),
							"documentId":     json.RawMessage(`"` + documentID.String() + `"`),
							"type":           json.RawMessage(`"paragraph"`),
							"text":           json.RawMessage(`"hello"`),
						},
					},
				}, nil
			},
		}

		blocks, err := newTestClient(idx, meilisearch.TaskStatusSucceeded).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
		require.NoError(t, err)

		assert.Equal(t, []Block{{
			ID:             "b1",
			OrganizationID: "org-1",
			DocumentID:     documentID,
			Type:           "paragraph",
			Text:           "hello",
		}}, blocks)

		require.NotNil(t, captured)
		assert.Equal(t, `organizationId = "org-1"`, captured.Filter)
		assert.Equal(t, int64(7), captured.Limit)
	})

	t.Run("Undecodable hit fails", func(t *testing.T) {
		t.Parallel()

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				return &meilisearch.SearchResponse{
					Hits: meilisearch.Hits{
						{"id": json.RawMessage(`42`)},
					},
				}, nil
			},
		}

		_, err := newTestClient(idx, meilisearch.TaskStatusSucceeded).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
		require.Error(t, err)
	})

	t.Run("Search failure is propagated", func(t *testing.T) {
		t.Parallel()

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				return nil, assert.AnError
			},
		}

		_, err := newTestClient(idx, meilisearch.TaskStatusSucceeded).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
		require.Error(t, err)
	})
}

func Test_Client_ReplaceDocumentBlocks(t *testing.T) {
	t.Parallel()

	type check func(*testing.T, *mock.MeiliIndexManager, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(expect bool) check {
		return func(t *testing.T, _ *mock.MeiliIndexManager, err error) {
			if expect {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		}
	}

	wasAddCalled := func(count int) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			require.Len(t, idx.AddDocumentsWithContextCalls(), count)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			require.Len(t, idx.DeleteDocumentsWithContextCalls(), count)
		}
	}

	wasDeleteByFilterCalled := func(count int) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			require.Len(t, idx.DeleteDocumentsByFilterWithContextCalls(), count)
		}
	}

	hasFilter := func(filter string) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			ff := idx.DeleteDocumentsByFilterWithContextCalls()
			require.NotEmpty(t, ff)
			assert.Equal(t, filter, ff[0].Filter)
		}
	}

	docID1, docID2 := xid.New(), xid.New()
	branchID1, branchID2 := xid.New(), xid.New()

	fullDiff := BlocksDifference{
		Added:   []Block{{ID: "a"}},
		Updated: []Block{{ID: "u"}},
		Removed: []Block{{ID: "r1"}, {ID: "r2"}},
	}

	tests := map[string]struct {
		Diff            BlocksDifference
		TaskStatus      meilisearch.TaskStatus
		AddErr          error
		DeleteErr       error
		DeleteFilterErr error
		Checks          []check
	}{
		"Empty difference makes no calls": {
			Checks: checks(
				hasError(false),
				wasAddCalled(0),
				wasDeleteCalled(0),
				wasDeleteByFilterCalled(0),
			),
		},
		"Removed branches are cleared by filter": {
			Diff: BlocksDifference{RemovedBranches: []BranchRemoval{
				{DocumentID: docID1, BranchID: branchID1},
				{DocumentID: docID1, BranchID: branchID2},
			}},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(0),
				wasDeleteByFilterCalled(1),
				hasFilter(fmt.Sprintf("branchId IN [%q, %q]", branchID1, branchID2)),
			),
		},
		"Removed documents are cleared by filter": {
			Diff: BlocksDifference{RemovedDocuments: []xid.ID{docID1, docID2}},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(0),
				wasDeleteByFilterCalled(1),
				hasFilter(fmt.Sprintf("documentId IN [%q, %q]", docID1, docID2)),
			),
		},
		"Removed organizations are cleared by filter": {
			Diff: BlocksDifference{RemovedOrganizations: []string{"org-1", "org-2"}},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(0),
				wasDeleteByFilterCalled(1),
				hasFilter(`organizationId IN ["org-1", "org-2"]`),
			),
		},
		"Delete by filter failure is propagated": {
			Diff:            BlocksDifference{RemovedDocuments: []xid.ID{docID1}},
			DeleteFilterErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasDeleteByFilterCalled(1),
			),
		},
		// added and updated entries are complete either way, so one
		// add-or-replace request carries both.
		"Full difference sends one add and one delete": {
			Diff: fullDiff,
			Checks: checks(
				hasError(false),
				wasAddCalled(1),
				wasDeleteCalled(1),
				func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
					assert.Equal(t, []Block{{ID: "a"}, {ID: "u"}}, idx.AddDocumentsWithContextCalls()[0].DocumentsPtr)
					assert.Equal(t, []string{"r1", "r2"}, idx.DeleteDocumentsWithContextCalls()[0].Identifiers)
				},
			),
		},
		"Add failure short-circuits": {
			Diff:   fullDiff,
			AddErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasDeleteCalled(0),
			),
		},
		"Delete failure is propagated": {
			Diff:      fullDiff,
			DeleteErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasAddCalled(1),
			),
		},
		// the request is accepted but the task fails later; the batch is
		// reported so the job is retried rather than dropped.
		"Failed task is reported with its cause": {
			Diff:       fullDiff,
			TaskStatus: meilisearch.TaskStatusFailed,
			Checks: checks(
				hasError(true),
				wasAddCalled(1),
				wasDeleteCalled(0),
				func(t *testing.T, _ *mock.MeiliIndexManager, err error) {
					assert.ErrorContains(t, err, "boom (invalid_document_id)")
				},
			),
		},
		"Calls run under a deadline": {
			Diff: fullDiff,
			Checks: checks(
				hasError(false),
				func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
					_, ok := idx.AddDocumentsWithContextCalls()[0].Ctx.Deadline()
					assert.True(t, ok)
				},
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idx := &mock.MeiliIndexManager{
				AddDocumentsWithContextFunc: func(context.Context, any, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 1}, tc.AddErr
				},
				DeleteDocumentsWithContextFunc: func(context.Context, []string, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 2}, tc.DeleteErr
				},
				DeleteDocumentsByFilterWithContextFunc: func(context.Context, any, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 3}, tc.DeleteFilterErr
				},
			}

			status := meilisearch.TaskStatusSucceeded
			if tc.TaskStatus != "" {
				status = tc.TaskStatus
			}

			err := newTestClient(idx, status).ReplaceDocumentBlocks(context.Background(), tc.Diff)

			for _, ch := range tc.Checks {
				ch(t, idx, err)
			}
		})
	}
}
