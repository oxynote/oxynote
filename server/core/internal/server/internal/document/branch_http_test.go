package document

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedHook builds a stored hook whose processor needs no external
// dependencies.
func storedHook(typ hookCore.Type) hookCore.Hook {
	return hookCore.Hook{
		ID:             xid.New(),
		Type:           typ,
		DocumentID:     null.ValueFrom(_documentID),
		OrganizationID: null.StringFrom("org1"),
		BranchID:       null.ValueFrom(_branchID),
		Settings:       processor.Settings(`{"scale":"linear"}`),
	}
}

func Test_Handler_FetchDocumentBranchesUnsafe(t *testing.T) {
	cc := map[string]struct {
		DB       *DBMock
		OmitDoc  bool
		RespCode int
	}{
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Branch fetch error": {
			DB: &DBMock{
				FetchDocumentBranchesUnsafeFunc: func(context.Context, xid.ID) ([]documentCore.BranchSummary, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch without a session": {
			DB: &DBMock{
				FetchDocumentBranchesUnsafeFunc: func(context.Context, xid.ID) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}}, nil
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			// the unsafe endpoints are session-free by design.
			hdl.FetchDocumentBranchesUnsafe(rec, newRequest(http.MethodGet, "", true, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _branchID.String())
			}
		})
	}
}

func Test_Handler_FetchDocumentBranchByIDUnsafe(t *testing.T) {
	cc := map[string]struct {
		DB         *DBMock
		OmitBranch bool
		RespCode   int
	}{
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Document fetch error": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch without a session": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentBranchByIDUnsafe(rec, newRequest(http.MethodGet, "", true, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _documentID.String())
			}
		})
	}
}

func Test_Handler_UpdateDocumentBranchByIDUnsafe(t *testing.T) {
	validBody := `{"name":"Renamed","maintainers":["u1","u9"]}`

	fetchStored := func(context.Context, xid.ID) (*documentCore.Document, error) {
		return storedDoc(), nil
	}

	cc := map[string]struct {
		DB          *DBMock
		Tx          *TxMock
		BeginErr    error
		OmitBranch  bool
		Body        string
		RespCode    int
		Committed   int
		Metadata    int
		Maintainers int
		SearchJobs  int
	}{
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			OmitBranch: true,
			Body:       validBody,
			RespCode:   http.StatusNotFound,
		},
		"Document fetch error": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Invalid JSON body": {
			DB:       &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Protected document rejects non-system update": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.Protected = true

					return doc, nil
				},
			},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusForbidden,
		},
		"Transaction start error": {
			DB:       &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Document update error": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				UpdateDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Maintainer fetch error": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Maintainer upsert error": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Search job insertion error on default branch": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
				InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
					return errors.New("boom")
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			SearchJobs: 1,
		},
		"Commit error": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			Committed:  1,
			SearchJobs: 1,
		},
		"Successful update with new maintainers": {
			DB:          &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx:          &TxMock{},
			Body:        validBody,
			RespCode:    http.StatusOK,
			Committed:   1,
			Metadata:    1,
			Maintainers: 1,
			SearchJobs:  1,
		},
		"Successful update without new maintainers": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
			},
			Body:       validBody,
			RespCode:   http.StatusOK,
			Committed:  1,
			Metadata:   1,
			SearchJobs: 1,
		},
		"Non-default branch skips the search job": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return branchDoc(_branchID2), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
			},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentBranchByIDUnsafe(rec, newRequest(http.MethodPut, c.Body, true, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.Metadata, cnt.metadata)
			assert.Equal(t, c.Maintainers, cnt.maintainers)
			assert.Len(t, c.Tx.InsertDocumentSearchJobCalls(), c.SearchJobs)
		})
	}
}

func Test_Handler_MergeBranches(t *testing.T) {
	validBody := `{"fromBranchId":"` + _branchID2.String() + `","toBranchId":"` + _branchID.String() + `"}`

	fetchByBranch := func(_ context.Context, branchID xid.ID, _ string) (*documentCore.Document, error) {
		if branchID == _branchID2 {
			return branchDoc(_branchID2), nil
		}

		return storedDoc(), nil
	}

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		Body      string
		RespCode  int
		RespJSON  string
		Committed int
		Metadata  int
		Reviewers int
	}{
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Source branch fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Branches from different documents": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, _ string) (*documentCore.Document, error) {
					if branchID == _branchID2 {
						doc := branchDoc(_branchID2)
						doc.ID = xid.New()

						return doc, nil
					}

					return storedDoc(), nil
				},
			},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusBadRequest,
			RespJSON: `{"code":"document.branch_mismatch","message":"branches must belong to the same document"}`,
		},
		"Transaction start error": {
			DB:       &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook soft-deletion error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				SoftDeleteDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Comment deletion error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				DeleteDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook copy fetch error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook re-creation error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook("bogus")}, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook insertion error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook(hookCore.TypeScheduledReminder)}, nil
				},
				InsertDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Document update error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				UpdateDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Approval promotion error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				PromoteBranchApprovalsFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful merge": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook(hookCore.TypeScheduledReminder)}, nil
				},
			},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
			Reviewers: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.MergeBranches(rec, newRequest(http.MethodPut, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.Metadata, cnt.metadata)
			assert.Equal(t, c.Reviewers, cnt.reviewers)

			if c.RespCode == http.StatusOK {
				// hooks are soft-deleted on the target and re-created
				// from the source branch.
				require.Len(t, c.Tx.SoftDeleteDocumentHooksByBranchIDCalls(), 1)
				assert.Equal(t, _branchID, c.Tx.SoftDeleteDocumentHooksByBranchIDCalls()[0].BranchID)
				require.Len(t, c.Tx.FetchDocumentHooksByBranchIDCalls(), 1)
				assert.Equal(t, _branchID2, c.Tx.FetchDocumentHooksByBranchIDCalls()[0].BranchID)
				require.Len(t, c.Tx.InsertDocumentHookCalls(), 1)
				assert.Equal(t, null.ValueFrom(_branchID), c.Tx.InsertDocumentHookCalls()[0].Hk.BranchID)
				require.Len(t, c.Tx.PromoteBranchApprovalsCalls(), 1)
			}
		})
	}
}

func Test_Handler_DeleteDocument(t *testing.T) {
	fetchStored := func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
		return storedDoc(), nil
	}

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		OmitDoc   bool
		RespCode  int
		Committed int
		TreeCbs   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Document fetch error": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentFunc: fetchStored,
				FetchDocumentHooksByDocumentIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Hook cleanup error": {
			DB: &DBMock{
				FetchDocumentFunc: fetchStored,
				FetchDocumentHooksByDocumentIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook("bogus")}, nil
				},
			},
			Tx:       &TxMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Transaction start error": {
			DB:       &DBMock{FetchDocumentFunc: fetchStored},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			RespCode: http.StatusInternalServerError,
		},
		"Document deletion error": {
			DB: &DBMock{FetchDocumentFunc: fetchStored},
			Tx: &TxMock{
				DeleteDocumentFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			DB: &DBMock{FetchDocumentFunc: fetchStored},
			Tx: &TxMock{
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful deletion with hook cleanup": {
			DB: &DBMock{
				FetchDocumentFunc: fetchStored,
				FetchDocumentHooksByDocumentIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook(hookCore.TypeScheduledReminder)}, nil
				},
			},
			Tx:        &TxMock{},
			RespCode:  http.StatusOK,
			Committed: 1,
			TreeCbs:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.DeleteDocument(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.RespCode == http.StatusOK {
				require.Len(t, c.Tx.DeleteDocumentCalls(), 1)
				assert.Equal(t, _documentID, c.Tx.DeleteDocumentCalls()[0].ID)

				// the search removal is queued by DeleteDocument itself,
				// which is what makes it cover the cascaded descendants.
				assert.Empty(t, c.Tx.InsertDocumentSearchJobCalls())
			}
		})
	}
}

func Test_Handler_DuplicateDocument(t *testing.T) {
	fetchStored := func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
		return storedDoc(), nil
	}

	// insertAwareTx returns the freshly inserted duplicate in the parent
	// subtree so the sort-index swap can find it.
	insertAwareTx := func() *TxMock {
		tx := &TxMock{}
		tx.FetchDocumentTreeByDocumentParentIDFunc = func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
			return documentCore.Summaries{{ID: tx.InsertDocumentCalls()[0].Doc.ID}}, nil
		}

		return tx
	}

	// fetchStoredWithImage serves a document whose content holds an image
	// served by that same document, which is what duplication has to copy.
	fetchStoredWithImage := func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
		doc := storedDoc()
		doc.Content = documentCore.RootBlock{
			Type: documentCore.BlockNodeDoc,
			Content: []documentCore.Block{
				{
					Type: documentCore.BlockNodeImageBlock,
					Attrs: documentCore.Attributes{
						"uid": "img1",
						"src": "https://app.test/core" + fmt.Sprintf(documentCore.FilePathFormat, _documentID, "img1"),
					},
				},
			},
		}

		return doc, nil
	}

	// imageDB serves the document with the image plus the file row that
	// image refers to, which is what the copy is made from.
	imageDB := func() *DBMock {
		return &DBMock{
			FetchDocumentFunc: fetchStoredWithImage,
			FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
				f := file.NewFile(id, file.LocationDocument, "key", _documentID, organizationID)

				return &f, nil
			},
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		Storer    *StorerMock
		BeginErr  error
		NoSession bool
		OmitDoc   bool
		RespCode  int
		Committed int
		TreeCbs   int
		Copies    int
	}{
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Document fetch error": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Transaction start error": {
			DB:       &DBMock{FetchDocumentFunc: fetchStored},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			RespCode: http.StatusInternalServerError,
		},
		"Duplicate insertion error": {
			DB: &DBMock{FetchDocumentFunc: fetchStored},
			Tx: &TxMock{
				InsertDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Sort index swap error": {
			DB:       &DBMock{FetchDocumentFunc: fetchStored},
			Tx:       &TxMock{},
			RespCode: http.StatusBadRequest,
		},
		"Commit error": {
			DB: &DBMock{FetchDocumentFunc: fetchStored},
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.CommitFunc = func() error {
					return errors.New("boom")
				}

				return tx
			}(),
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful duplication": {
			DB:        &DBMock{FetchDocumentFunc: fetchStored},
			Tx:        insertAwareTx(),
			RespCode:  http.StatusCreated,
			Committed: 1,
			TreeCbs:   1,
		},
		"Files are copied along with the document": {
			DB:        imageDB(),
			Tx:        insertAwareTx(),
			Storer:    &StorerMock{},
			RespCode:  http.StatusCreated,
			Committed: 1,
			TreeCbs:   1,
			Copies:    1,
		},
		"Failing file copy still yields the duplicate": {
			DB: imageDB(),
			Tx: insertAwareTx(),
			Storer: &StorerMock{
				CopyFunc: func(context.Context, string, string, string, string) error {
					return errors.New("boom")
				},
			},
			RespCode:  http.StatusCreated,
			Committed: 1,
			TreeCbs:   1,
			Copies:    1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			if c.Storer != nil {
				hdl.storer = c.Storer
			}

			rec := httptest.NewRecorder()

			hdl.DuplicateDocument(rec, newRequest(http.MethodPost, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.Storer != nil {
				assert.Len(t, c.Storer.CopyCalls(), c.Copies)
				assert.Len(t, c.DB.InsertDocumentFileCalls(), c.Copies)
			}

			if c.RespCode == http.StatusCreated {
				// the duplicate is a new document owned by the caller.
				require.Len(t, c.Tx.InsertDocumentCalls(), 1)
				assert.NotEqual(t, _documentID, c.Tx.InsertDocumentCalls()[0].Doc.ID)
				assert.Equal(t, []string{"u1"}, c.Tx.UpsertDocumentMaintainersCalls()[0].MaintainerIDs)
			}
		})
	}
}

func Test_Handler_copyDocumentFiles(t *testing.T) {
	toDocumentID := xid.New()

	fileDB := func() *DBMock {
		return &DBMock{
			FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
				f := file.NewFile(id, file.LocationComment, "key", _documentID, organizationID)

				return &f, nil
			},
		}
	}

	cc := map[string]struct {
		DB      *DBMock
		Storer  *StorerMock
		Files   map[string]string
		Inserts int
		Copies  int
	}{
		"Nothing to copy": {
			DB:     fileDB(),
			Storer: &StorerMock{},
			Files:  map[string]string{},
		},
		"File row is missing": {
			DB: &DBMock{
				FetchDocumentFileFunc: func(context.Context, string, string) (*file.File, error) {
					return nil, errors.New("boom")
				},
			},
			Storer: &StorerMock{},
			Files:  map[string]string{"old-1": "new-1"},
		},
		"Error returned by db.InsertDocumentFile": {
			DB: func() *DBMock {
				db := fileDB()
				db.InsertDocumentFileFunc = func(context.Context, file.File) error {
					return errors.New("boom")
				}

				return db
			}(),
			Storer:  &StorerMock{},
			Files:   map[string]string{"old-1": "new-1"},
			Inserts: 1,
		},
		"Error returned by storer.Copy": {
			DB: fileDB(),
			Storer: &StorerMock{
				CopyFunc: func(context.Context, string, string, string, string) error {
					return errors.New("boom")
				},
			},
			Files:   map[string]string{"old-1": "new-1"},
			Inserts: 1,
			Copies:  1,
		},
		"Successful copy": {
			DB:      fileDB(),
			Storer:  &StorerMock{},
			Files:   map[string]string{"old-1": "new-1"},
			Inserts: 1,
			Copies:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.storer = c.Storer

			hdl.copyDocumentFiles(context.Background(), c.Files, _documentID, toDocumentID, "org1")

			ff := c.DB.InsertDocumentFileCalls()
			require.Len(t, ff, c.Inserts)

			cf := c.Storer.CopyCalls()
			require.Len(t, cf, c.Copies)

			if c.Inserts == 0 {
				return
			}

			// the copy owns its row, keyed under the new document.
			assert.Equal(t, "new-1", ff[0].F.ID)
			assert.Equal(t, file.LocationComment, ff[0].F.Location)
			assert.Equal(t, file.Key("org1", toDocumentID, "new-1"), ff[0].F.StorageKey)
			assert.Equal(t, toDocumentID, ff[0].F.DocumentID.V)

			if c.Copies == 0 {
				return
			}

			assert.Equal(t, file.Folder("org1", _documentID), cf[0].SrcFolder)
			assert.Equal(t, "old-1", cf[0].SrcID)
			assert.Equal(t, file.Folder("org1", toDocumentID), cf[0].DstFolder)
			assert.Equal(t, "new-1", cf[0].DstID)
		})
	}
}

func Test_Handler_FetchDocumentBranches(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		RespCode  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Branch fetch error": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}}, nil
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentBranches(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _branchID.String())
			}
		})
	}
}

func Test_Handler_CreateDocumentBranch(t *testing.T) {
	validBody := `{"branch":"feature","sourceBranchId":"` + _branchID.String() + `"}`

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		Body      string
		RespCode  int
		Committed int
	}{
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Empty branch name": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			Body:     `{"branch":"","sourceBranchId":"` + _branchID.String() + `"}`,
			RespCode: http.StatusBadRequest,
		},
		"Source branch fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Transaction start error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Branch fork error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				ForkDocumentBranchFunc: func(context.Context, xid.ID, string, string, string, string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"New branch fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook copy error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return branchDoc(_branchID2), nil
				},
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return branchDoc(_branchID2), nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful branch creation": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return branchDoc(_branchID2), nil
				},
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook(hookCore.TypeScheduledReminder)}, nil
				},
			},
			Body:      validBody,
			RespCode:  http.StatusCreated,
			Committed: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.CreateDocumentBranch(rec, newRequest(http.MethodPost, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)

			if c.RespCode == http.StatusCreated {
				require.Len(t, c.Tx.ForkDocumentBranchCalls(), 1)
				assert.Equal(t, "feature", c.Tx.ForkDocumentBranchCalls()[0].TargetBranch)
				// hooks are copied from the source to the new branch.
				require.Len(t, c.Tx.InsertDocumentHookCalls(), 1)
				assert.Equal(t, null.ValueFrom(_branchID2), c.Tx.InsertDocumentHookCalls()[0].Hk.BranchID)
			}
		})
	}
}

func Test_Handler_DeleteDocumentBranch(t *testing.T) {
	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		OmitBranch bool
		RespCode   int
		RespJSON   string
		Deleted    int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Default branch cannot be deleted": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode: http.StatusConflict,
			RespJSON: `{"code":"document.default_branch","message":"cannot delete the default branch"}`,
		},
		"Branch count error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Last branch cannot be deleted": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 1, nil
				},
			},
			RespCode: http.StatusConflict,
			RespJSON: `{"code":"document.last_branch","message":"cannot delete the last branch"}`,
		},
		"Branch deletion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 2, nil
				},
				DeleteDocumentBranchByIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 2, nil
				},
			},
			RespCode: http.StatusOK,
			Deleted:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentBranch(rec, newRequest(http.MethodDelete, "", c.NoSession, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.DB.DeleteDocumentBranchByIDCalls(), c.Deleted)
		})
	}
}
