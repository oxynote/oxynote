package search

import (
	"context"
	"encoding/json"
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

	wasFilterableUpdated := func(count int) check {
		return func(t *testing.T, _ *mock.MeiliServiceManager, idx *mock.MeiliIndexManager, _ error) {
			ff := idx.UpdateFilterableAttributesWithContextCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, &[]any{"organizationId"}, ff[0].Request)
		}
	}

	wasSynonymsUpdated := func(count int) check {
		return func(t *testing.T, _ *mock.MeiliServiceManager, idx *mock.MeiliIndexManager, _ error) {
			ff := idx.UpdateSynonymsWithContextCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			require.NotNil(t, ff[0].Request)
			assert.NotEmpty(t, *ff[0].Request)
		}
	}

	tests := map[string]struct {
		GetIndexErr   error
		CreateErr     error
		FilterableErr error
		SynonymsErr   error
		WaitErr       error
		TaskStatus    meilisearch.TaskStatus
		Checks        []check
	}{
		"Existing index still has its settings applied": {
			Checks: checks(
				hasError(false),
				wasCreateIndexCalled(0),
				wasFilterableUpdated(1),
				wasSynonymsUpdated(1),
			),
		},
		"Rejected settings task fails": {
			TaskStatus: meilisearch.TaskStatusFailed,
			Checks: checks(
				hasError(true),
				wasCreateIndexCalled(0),
			),
		},
		"Task wait failure fails": {
			WaitErr: assert.AnError,
			Checks: checks(
				hasError(true),
			),
		},
		"Missing index is created with settings": {
			GetIndexErr: indexNotFoundErr(),
			Checks: checks(
				hasError(false),
				wasCreateIndexCalled(1),
				wasFilterableUpdated(1),
				wasSynonymsUpdated(1),
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
				wasFilterableUpdated(0),
			),
		},
		"Filterable attribute failure fails": {
			GetIndexErr:   indexNotFoundErr(),
			FilterableErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasSynonymsUpdated(0),
			),
		},
		"Synonyms failure fails": {
			GetIndexErr: indexNotFoundErr(),
			SynonymsErr: assert.AnError,
			Checks: checks(
				hasError(true),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idx := &mock.MeiliIndexManager{
				UpdateFilterableAttributesWithContextFunc: func(context.Context, *[]any) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 2}, tc.FilterableErr
				},
				UpdateSynonymsWithContextFunc: func(context.Context, *map[string][]string) (*meilisearch.TaskInfo, error) {
					return &meilisearch.TaskInfo{TaskUID: 3}, tc.SynonymsErr
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
// bypassing ensureIndex.
func newTestClient(idx *mock.MeiliIndexManager) *Client {
	return &Client{meiliMan: stubManagers(idx)}
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

		data, err := newTestClient(idx).SearchDocuments(context.Background(), "org-1", "hello")
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

		_, err := newTestClient(idx).SearchDocuments(context.Background(), "org-1", "hello")
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

		blocks, err := newTestClient(idx).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
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

		_, err := newTestClient(idx).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
		require.Error(t, err)
	})

	t.Run("Search failure is propagated", func(t *testing.T) {
		t.Parallel()

		idx := &mock.MeiliIndexManager{
			SearchWithContextFunc: func(context.Context, string, *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
				return nil, assert.AnError
			},
		}

		_, err := newTestClient(idx).SearchDocumentBlocks(context.Background(), "org-1", "hello", 7)
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

	wasUpdateCalled := func(count int) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			require.Len(t, idx.UpdateDocumentsWithContextCalls(), count)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
			require.Len(t, idx.DeleteDocumentsWithContextCalls(), count)
		}
	}

	fullDiff := BlocksDifference{
		Added:   []Block{{ID: "a"}},
		Updated: []Block{{ID: "u"}},
		Removed: []Block{{ID: "r1"}, {ID: "r2"}},
	}

	tests := map[string]struct {
		Diff      BlocksDifference
		AddErr    error
		UpdateErr error
		DeleteErr error
		Checks    []check
	}{
		"Empty difference makes no calls": {
			Checks: checks(
				hasError(false),
				wasAddCalled(0),
				wasUpdateCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Full difference dispatches all three": {
			Diff: fullDiff,
			Checks: checks(
				hasError(false),
				wasAddCalled(1),
				wasUpdateCalled(1),
				wasDeleteCalled(1),
				func(t *testing.T, idx *mock.MeiliIndexManager, _ error) {
					ff := idx.DeleteDocumentsWithContextCalls()
					require.NotEmpty(t, ff)
					assert.Equal(t, []string{"r1", "r2"}, ff[0].Identifiers)
				},
			),
		},
		"Add failure short-circuits": {
			Diff:   fullDiff,
			AddErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasUpdateCalled(0),
				wasDeleteCalled(0),
			),
		},
		"Update failure short-circuits": {
			Diff:      fullDiff,
			UpdateErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasAddCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Delete failure is propagated": {
			Diff:      fullDiff,
			DeleteErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasAddCalled(1),
				wasUpdateCalled(1),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idx := &mock.MeiliIndexManager{
				AddDocumentsWithContextFunc: func(context.Context, any, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return nil, tc.AddErr
				},
				UpdateDocumentsWithContextFunc: func(context.Context, any, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return nil, tc.UpdateErr
				},
				DeleteDocumentsWithContextFunc: func(context.Context, []string, *meilisearch.DocumentOptions) (*meilisearch.TaskInfo, error) {
					return nil, tc.DeleteErr
				},
			}

			err := newTestClient(idx).ReplaceDocumentBlocks(context.Background(), tc.Diff)

			for _, ch := range tc.Checks {
				ch(t, idx, err)
			}
		})
	}
}
