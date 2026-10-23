package document

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedHook builds a stored hook on the document's main branch.
func storedHook() hookCore.Hook {
	return hookCore.Hook{
		ID:             xid.New(),
		Type:           hookCore.TypeScheduledReminder,
		DocumentID:     null.ValueFrom(_documentID),
		OrganizationID: null.StringFrom("org1"),
		BranchID:       null.ValueFrom(_branchID),
		Settings:       processor.Settings(`{"scale":"linear"}`),
		State:          null.ValueFrom(processor.State(`{}`)),
	}
}

func Test_Handler_FetchDocumentBranchesUnsafe(t *testing.T) {
	cc := map[string]struct {
		DB                          *DBMock
		OmitDoc                     bool
		RespCode                    int
		FetchDocumentBranchesUnsafe int
	}{
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentBranchesUnsafe": {
			DB: &DBMock{
				FetchDocumentBranchesUnsafeFunc: func(context.Context, xid.ID) ([]documentCore.BranchSummary, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:                    http.StatusInternalServerError,
			FetchDocumentBranchesUnsafe: 1,
		},
		"Successful fetch without a session": {
			DB: &DBMock{
				FetchDocumentBranchesUnsafeFunc: func(context.Context, xid.ID) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}}, nil
				},
			},
			RespCode:                    http.StatusOK,
			FetchDocumentBranchesUnsafe: 1,
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
			assert.Len(t, c.DB.FetchDocumentBranchesUnsafeCalls(), c.FetchDocumentBranchesUnsafe)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _branchID.String())
			}
		})
	}
}

func Test_Handler_FetchDocumentBranchByIDUnsafe(t *testing.T) {
	cc := map[string]struct {
		DB                            *DBMock
		OmitBranch                    bool
		RespCode                      int
		FetchDocumentUnsafeByBranchID int
	}{
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentUnsafeByBranchID": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:                      http.StatusInternalServerError,
			FetchDocumentUnsafeByBranchID: 1,
		},
		"Successful fetch without a session": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode:                      http.StatusOK,
			FetchDocumentUnsafeByBranchID: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentBranchByIDUnsafe(rec, newRequest(http.MethodGet, "", true, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentUnsafeByBranchIDCalls(), c.FetchDocumentUnsafeByBranchID)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _documentID.String())
			}
		})
	}
}

func Test_Handler_UpdateDocumentBranchByIDUnsafe(t *testing.T) {
	validBody := `{"name":"Renamed","maintainers":["u1","u9"]}`

	cc := map[string]struct {
		DB          *DBMock
		Tx          *TxMock
		Queue       *SearchQueueMock
		BeginErr    error
		OmitBranch  bool
		Body        string
		RespCode    int
		Committed   int
		Metadata    int
		Maintainers int
		SearchJobs  int
		BranchID    xid.ID
		// History is how many history entries the persist records.
		History int
		// HistoryBy is who the recorded entry is credited to.
		HistoryBy                     null.String
		FetchDocumentUnsafeByBranchID int
		BeginTx                       int
		UpdateDocument                int
		FetchDocumentMaintainers      int
		UpsertDocumentMaintainers     int
		Rollback                      int
	}{
		"Unchanged persist writes no history entry": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                            &TxMock{},
			Body:                          `{"name":"Doc","maintainers":["u1","u9"]}`,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			Maintainers:                   1,
			SearchJobs:                    1,
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			UpsertDocumentMaintainers:     1,
			Rollback:                      1,
		},
		"Error returned by Tx.RecordDocumentBranchHistoryEntry": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			Rollback:                      1,
		},
		"System write records an unattributed entry": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.LastUpdatedBy = null.StringFrom("u5")

					return doc, nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
			},
			Body:                          `{"name":"Renamed","system":true}`,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			SearchJobs:                    1,
			History:                       1,
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Edit naming no maintainers records an unattributed entry": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.LastUpdatedBy = null.StringFrom("u5")

					return doc, nil
				},
			},
			Tx:                            &TxMock{},
			Body:                          `{"name":"Renamed"}`,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			SearchJobs:                    1,
			History:                       1,
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			OmitBranch: true,
			Body:       validBody,
			RespCode:   http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentUnsafeByBranchID": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:                            &TxMock{},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			FetchDocumentUnsafeByBranchID: 1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                            &TxMock{},
			Body:                          "{",
			RespCode:                      http.StatusBadRequest,
			FetchDocumentUnsafeByBranchID: 1,
		},
		"Protected document rejects a user update": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.Protected = true

					return doc, nil
				},
			},
			Tx:                            &TxMock{},
			Body:                          validBody,
			RespCode:                      http.StatusForbidden,
			FetchDocumentUnsafeByBranchID: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                            &TxMock{},
			BeginErr:                      errors.New("boom"),
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
		},
		"Error returned by Tx.UpdateDocument": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				UpdateDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			Rollback:                      1,
		},
		"Error returned by Tx.FetchDocumentMaintainers": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Error returned by Tx.UpsertDocumentMaintainers": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			UpsertDocumentMaintainers:     1,
			Rollback:                      1,
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
			},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			SearchJobs:                    1,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			Committed:                     1,
			SearchJobs:                    1,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Successful update with new maintainers": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                            &TxMock{},
			Body:                          validBody,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			Maintainers:                   1,
			SearchJobs:                    1,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			UpsertDocumentMaintainers:     1,
			Rollback:                      1,
		},
		"Successful update without new maintainers": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			SearchJobs:                    1,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
		"Feature branch queues its own search job": {
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
			Body:                          validBody,
			RespCode:                      http.StatusOK,
			Committed:                     1,
			Metadata:                      1,
			SearchJobs:                    1,
			BranchID:                      _branchID2,
			History:                       1,
			HistoryBy:                     null.StringFrom("u9"),
			FetchDocumentUnsafeByBranchID: 1,
			BeginTx:                       1,
			UpdateDocument:                1,
			FetchDocumentMaintainers:      1,
			Rollback:                      1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})
			hdl.searchQueue = queue

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentBranchByIDUnsafe(rec, newRequest(http.MethodPut, c.Body, true, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentUnsafeByBranchIDCalls(), c.FetchDocumentUnsafeByBranchID)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.UpdateDocumentCalls(), c.UpdateDocument)
			assert.Len(t, c.Tx.FetchDocumentMaintainersCalls(), c.FetchDocumentMaintainers)
			assert.Len(t, c.Tx.UpsertDocumentMaintainersCalls(), c.UpsertDocumentMaintainers)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Equal(t, c.Metadata, cnt.metadata)
			assert.Equal(t, c.Maintainers, cnt.maintainers)

			qq := queue.EnqueueCalls()
			require.Len(t, qq, c.SearchJobs)

			if c.SearchJobs > 0 {
				assert.Same(t, c.Tx, qq[0].Tx)
			}

			// the entry is ordinary and records the persisted branch,
			// credited to the last maintainer. A system write is not
			// credited to anyone.
			hh := c.Tx.RecordDocumentBranchHistoryEntryCalls()
			require.Len(t, hh, c.History)

			if c.History > 0 {
				branchID := c.BranchID
				if branchID.IsZero() {
					branchID = _branchID
				}

				assert.Equal(t, branchID, hh[0].BranchID)
				assert.Equal(t, "org1", hh[0].OrganizationID)
				assert.Equal(t, c.HistoryBy, hh[0].By)
				assert.False(t, hh[0].Boundary)
			}

			// the job is scoped to the persisted branch, whichever branch
			// it is.
			if !c.BranchID.IsZero() {
				j := qq[0].J
				assert.Equal(t, c.BranchID, j.BranchID.V)
				assert.True(t, j.DocumentID.Valid)
			}
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

	otherDocID := xid.New()

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		OmitDoc   bool
		Body      string
		RespCode  int
		RespJSON  string
		Committed int
		Metadata  int
		Reviewers int
		// History is how many history entries the merge records.
		History                          int
		FetchDocumentByBranchID          int
		BeginTx                          int
		DetachDocumentHooksByBranchID    int
		DeleteDocumentCommentsByBranchID int
		UpdateDocument                   int
		FetchDocumentHooksByBranchID     int
		InsertDocumentHook               int
		ReplaceBranchTags                int
		Enqueue                          int
		PromoteBranchApprovals           int
		Rollback                         int
		SetUpBranch                      int
	}{
		"Error returned by Tx.RecordDocumentBranchHistoryEntry": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			History:                          1,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			Rollback:                         1,
		},
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Merging a branch into itself": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			Body:     `{"fromBranchId":"` + _branchID.String() + `","toBranchId":"` + _branchID.String() + `"}`,
			RespCode: http.StatusBadRequest,
			RespJSON: `{"code":"document.branch_self_merge","message":"cannot merge a branch into itself"}`,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:                      &TxMock{},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
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
			Tx:                      &TxMock{},
			Body:                    validBody,
			RespCode:                http.StatusNotFound,
			RespJSON:                `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
			FetchDocumentByBranchID: 2,
		},
		"Branches of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, _ string) (*documentCore.Document, error) {
					doc := storedDoc()

					if branchID == _branchID2 {
						doc = branchDoc(_branchID2)
					}

					doc.ID = otherDocID

					return doc, nil
				},
			},
			Tx:                      &TxMock{},
			Body:                    validBody,
			RespCode:                http.StatusNotFound,
			RespJSON:                `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
			FetchDocumentByBranchID: 2,
		},
		"Error returned by DB.BeginTx": {
			DB:                      &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx:                      &TxMock{},
			BeginErr:                errors.New("boom"),
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 2,
			BeginTx:                 1,
		},
		"Error returned by Tx.DetachDocumentHooksByBranchID": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				DetachDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:                          validBody,
			RespCode:                      http.StatusInternalServerError,
			FetchDocumentByBranchID:       2,
			BeginTx:                       1,
			DetachDocumentHooksByBranchID: 1,
			Rollback:                      1,
		},
		"Error returned by Tx.DeleteDocumentCommentsByBranchID": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				DeleteDocumentCommentsByBranchIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			Rollback:                         1,
		},
		"Error returned by Tx.UpdateDocument": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				UpdateDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			Rollback:                         1,
		},
		"Error returned by Tx.PromoteBranchApprovals": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				PromoteBranchApprovalsFunc: func(context.Context, xid.ID, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			History:                          1,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			ReplaceBranchTags:                1,
			Enqueue:                          1,
			PromoteBranchApprovals:           1,
			Rollback:                         1,
		},
		"Error returned by Tx.ReplaceBranchTags": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				ReplaceBranchTagsFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			History:                          1,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			ReplaceBranchTags:                1,
			Rollback:                         1,
		},
		"Error returned by Tx.FetchDocumentHooksByBranchID": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			Rollback:                         1,
		},
		"Error returned by Tx.InsertDocumentHook": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook()}, nil
				},
				InsertDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			InsertDocumentHook:               1,
			Rollback:                         1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			Committed:                        1,
			History:                          1,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			ReplaceBranchTags:                1,
			Enqueue:                          1,
			PromoteBranchApprovals:           1,
			Rollback:                         1,
		},
		"Successful merge": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					hk := storedHook()
					hk.BranchID = null.ValueFrom(_branchID2)

					return []hookCore.Hook{hk}, nil
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusOK,
			Committed:                        1,
			Metadata:                         1,
			Reviewers:                        1,
			History:                          1,
			FetchDocumentByBranchID:          2,
			BeginTx:                          1,
			DetachDocumentHooksByBranchID:    1,
			DeleteDocumentCommentsByBranchID: 1,
			UpdateDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			InsertDocumentHook:               1,
			ReplaceBranchTags:                1,
			Enqueue:                          1,
			PromoteBranchApprovals:           1,
			Rollback:                         1,
			SetUpBranch:                      1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.DB.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})

			queue := &SearchQueueMock{}
			hdl.searchQueue = queue

			man := &HookManagerMock{}
			hdl.hookMan = man

			rec := httptest.NewRecorder()

			hdl.MergeBranches(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.Metadata, cnt.metadata)
			assert.Equal(t, c.Reviewers, cnt.reviewers)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.History)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.DetachDocumentHooksByBranchIDCalls(), c.DetachDocumentHooksByBranchID)
			assert.Len(t, c.Tx.DeleteDocumentCommentsByBranchIDCalls(), c.DeleteDocumentCommentsByBranchID)
			assert.Len(t, c.Tx.UpdateDocumentCalls(), c.UpdateDocument)
			assert.Len(t, c.Tx.FetchDocumentHooksByBranchIDCalls(), c.FetchDocumentHooksByBranchID)
			assert.Len(t, c.Tx.InsertDocumentHookCalls(), c.InsertDocumentHook)
			assert.Len(t, c.Tx.ReplaceBranchTagsCalls(), c.ReplaceBranchTags)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, c.Tx.PromoteBranchApprovalsCalls(), c.PromoteBranchApprovals)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Len(t, man.SetUpBranchCalls(), c.SetUpBranch)

			if c.RespCode == http.StatusOK {
				// the merge is a boundary entry of the target, attributed to
				// the merging user.
				entry := c.Tx.RecordDocumentBranchHistoryEntryCalls()[0]
				assert.Equal(t, _branchID, entry.BranchID)
				assert.Equal(t, "org1", entry.OrganizationID)
				assert.True(t, entry.Boundary)
				assert.Equal(t, null.StringFrom("u1"), entry.By)

				// the target's hooks are detached and the source's copied
				// inside the transaction; the copies are set up once it
				// commits.
				require.Len(t, c.Tx.DetachDocumentHooksByBranchIDCalls(), 1)
				assert.Equal(t, _branchID, c.Tx.DetachDocumentHooksByBranchIDCalls()[0].BranchID)

				hh := c.Tx.FetchDocumentHooksByBranchIDCalls()
				require.Len(t, hh, 1)
				assert.Equal(t, _branchID2, hh[0].BranchID)
				assert.Equal(t, "org1", hh[0].OrganizationID)

				ii := c.Tx.InsertDocumentHookCalls()
				require.Len(t, ii, 1)
				assert.Equal(t, null.ValueFrom(_branchID), ii[0].Hk.BranchID)
				assert.Equal(t, null.ValueFrom(_documentID), ii[0].Hk.DocumentID)
				assert.Equal(t, processor.StatusInitializing, ii[0].Hk.Status)
				assert.False(t, ii[0].Hk.State.Valid)

				pp := man.SetUpBranchCalls()
				require.Len(t, pp, 1)
				assert.Equal(t, _branchID, pp[0].BranchID)
				assert.Equal(t, "org1", pp[0].OrganizationID)
				require.Len(t, c.Tx.PromoteBranchApprovalsCalls(), 1)

				// the target takes the source's tags the way it takes its
				// name and icon, before the commit.
				require.Len(t, c.Tx.ReplaceBranchTagsCalls(), 1)
				assert.Equal(t, _branchID2, c.Tx.ReplaceBranchTagsCalls()[0].FromBranchID)
				assert.Equal(t, _branchID, c.Tx.ReplaceBranchTagsCalls()[0].ToBranchID)
			}
		})
	}
}

func Test_Handler_FetchDocumentBranches(t *testing.T) {
	cc := map[string]struct {
		DB                    *DBMock
		NoSession             bool
		OmitDoc               bool
		RespCode              int
		FetchDocumentBranches int
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
		"Error returned by DB.FetchDocumentBranches": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:              http.StatusInternalServerError,
			FetchDocumentBranches: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchSummary, error) {
					return []documentCore.BranchSummary{{BranchID: _branchID}}, nil
				},
			},
			RespCode:              http.StatusOK,
			FetchDocumentBranches: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentBranches(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.FetchDocumentBranches)

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
		Queue     *SearchQueueMock
		BeginErr  error
		NoSession bool
		OmitDoc   bool
		Body      string
		RespCode  int
		Committed int
		// History is how many history entries the fork records.
		History                      int
		FetchDocumentByBranchID      int
		BeginTx                      int
		InsertDocumentBranch         int
		FetchDocumentHooksByBranchID int
		InsertDocumentHook           int
		CopyBranchTags               int
		Enqueue                      int
		Rollback                     int
		SetUpBranch                  int
	}{
		"Error returned by Tx.RecordDocumentBranchHistoryEntry": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			History:                      1,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			Rollback:                     1,
		},
		"No session in context": {
			DB:        &DBMock{},
			Tx:        &TxMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			Tx:       &TxMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Source branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.ID = xid.New()

					return doc, nil
				},
			},
			Tx:                      &TxMock{},
			Body:                    validBody,
			RespCode:                http.StatusNotFound,
			FetchDocumentByBranchID: 1,
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
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:                      &TxMock{},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                      &TxMock{},
			BeginErr:                errors.New("boom"),
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
		},
		"Error returned by Tx.InsertDocumentBranch": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				InsertDocumentBranchFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
			InsertDocumentBranch:    1,
			Rollback:                1,
		},
		"Error returned by Tx.FetchDocumentHooksByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			Rollback:                     1,
		},
		"Error returned by Tx.InsertDocumentHook": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook()}, nil
				},
				InsertDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			InsertDocumentHook:           1,
			Rollback:                     1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			Committed:                    1,
			History:                      1,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			CopyBranchTags:               1,
			Enqueue:                      1,
			Rollback:                     1,
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			History:                      1,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			CopyBranchTags:               1,
			Enqueue:                      1,
			Rollback:                     1,
		},
		"Successful branch creation": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook()}, nil
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusCreated,
			Committed:                    1,
			History:                      1,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			InsertDocumentHook:           1,
			CopyBranchTags:               1,
			Enqueue:                      1,
			Rollback:                     1,
			SetUpBranch:                  1,
		},
		"Error returned by Tx.CopyBranchTags": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
				return storedDoc(), nil
			}},
			Tx: &TxMock{
				CopyBranchTagsFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			Body:                         validBody,
			RespCode:                     http.StatusInternalServerError,
			History:                      1,
			FetchDocumentByBranchID:      1,
			BeginTx:                      1,
			InsertDocumentBranch:         1,
			FetchDocumentHooksByBranchID: 1,
			CopyBranchTags:               1,
			Rollback:                     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.searchQueue = queue

			man := &HookManagerMock{}
			hdl.hookMan = man

			rec := httptest.NewRecorder()

			hdl.CreateDocumentBranch(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.History)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.InsertDocumentBranchCalls(), c.InsertDocumentBranch)
			assert.Len(t, c.Tx.FetchDocumentHooksByBranchIDCalls(), c.FetchDocumentHooksByBranchID)
			assert.Len(t, c.Tx.InsertDocumentHookCalls(), c.InsertDocumentHook)
			assert.Len(t, c.Tx.CopyBranchTagsCalls(), c.CopyBranchTags)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Len(t, man.SetUpBranchCalls(), c.SetUpBranch)

			if c.RespCode == http.StatusCreated {
				require.Len(t, c.Tx.InsertDocumentBranchCalls(), 1)
				forked := c.Tx.InsertDocumentBranchCalls()[0].Doc
				assert.Equal(t, "feature", forked.BranchName)
				assert.NotEqual(t, _branchID, forked.BranchID)
				assert.False(t, forked.Protected)
				assert.Nil(t, forked.RawContent)

				// the fork starts with a boundary entry of its own content.
				entry := c.Tx.RecordDocumentBranchHistoryEntryCalls()[0]
				assert.Equal(t, forked.BranchID, entry.BranchID)
				assert.Equal(t, "org1", entry.OrganizationID)
				assert.True(t, entry.Boundary)
				assert.Equal(t, null.StringFrom("u1"), entry.By)

				// the source's hooks are copied inside the transaction and
				// set up once it commits.
				hh := c.Tx.FetchDocumentHooksByBranchIDCalls()
				require.Len(t, hh, 1)
				assert.Equal(t, _branchID, hh[0].BranchID)
				assert.Equal(t, "org1", hh[0].OrganizationID)

				ii := c.Tx.InsertDocumentHookCalls()
				require.Len(t, ii, 1)
				assert.Equal(t, null.ValueFrom(forked.BranchID), ii[0].Hk.BranchID)
				assert.Equal(t, null.ValueFrom(_documentID), ii[0].Hk.DocumentID)
				assert.Equal(t, processor.StatusInitializing, ii[0].Hk.Status)
				assert.False(t, ii[0].Hk.State.Valid)

				pp := man.SetUpBranchCalls()
				require.Len(t, pp, 1)
				assert.Equal(t, forked.BranchID, pp[0].BranchID)
				assert.Equal(t, "org1", pp[0].OrganizationID)

				// the fork takes the source's tags before the commit.
				require.Len(t, c.Tx.CopyBranchTagsCalls(), 1)
				assert.Equal(t, _branchID, c.Tx.CopyBranchTagsCalls()[0].FromBranchID)
				assert.Equal(t, forked.BranchID, c.Tx.CopyBranchTagsCalls()[0].ToBranchID)

				// the fork is queued under its own branch id before the
				// commit, so the branch is searchable without an edit.
				qq := queue.EnqueueCalls()
				require.Len(t, qq, 1)
				assert.Same(t, c.Tx, qq[0].Tx)
				assert.Equal(t, search.BranchScope("org1", forked.ID, forked.BranchID), qq[0].J)
			}
		})
	}
}

func Test_Handler_DeleteDocumentBranch(t *testing.T) {
	cc := map[string]struct {
		DB                      *DBMock
		Tx                      *TxMock
		Queue                   *SearchQueueMock
		NoSession               bool
		OmitDoc                 bool
		OmitBranch              bool
		RespCode                int
		RespJSON                string
		Deleted                 int
		Committed               int
		FetchDocumentByBranchID int
		CountDocumentBranches   int
		BeginTx                 int
		Enqueue                 int
		Rollback                int
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
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			RespCode:                http.StatusNotFound,
			RespJSON:                `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
			FetchDocumentByBranchID: 1,
		},
		"Default branch cannot be deleted": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode:                http.StatusConflict,
			RespJSON:                `{"code":"document.default_branch","message":"cannot delete the default branch"}`,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.CountDocumentBranches": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			CountDocumentBranches:   1,
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
			RespCode:                http.StatusConflict,
			RespJSON:                `{"code":"document.last_branch","message":"cannot delete the last branch"}`,
			FetchDocumentByBranchID: 1,
			CountDocumentBranches:   1,
		},
		"Error returned by Tx.DeleteDocumentBranchByID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 2, nil
				},
			},
			Tx: &TxMock{
				DeleteDocumentBranchByIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode:                http.StatusInternalServerError,
			Deleted:                 1,
			FetchDocumentByBranchID: 1,
			CountDocumentBranches:   1,
			BeginTx:                 1,
			Rollback:                1,
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 2, nil
				},
			},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			RespCode:                http.StatusInternalServerError,
			Deleted:                 1,
			FetchDocumentByBranchID: 1,
			CountDocumentBranches:   1,
			BeginTx:                 1,
			Enqueue:                 1,
			Rollback:                1,
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
			RespCode:                http.StatusNoContent,
			Deleted:                 1,
			Committed:               1,
			FetchDocumentByBranchID: 1,
			CountDocumentBranches:   1,
			BeginTx:                 1,
			Enqueue:                 1,
			Rollback:                1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := c.Tx
			if tx == nil {
				tx = &TxMock{}
			}

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(tx, nil)

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.searchQueue = queue

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentBranch(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.CountDocumentBranchesCalls(), c.CountDocumentBranches)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, tx.DeleteDocumentBranchByIDCalls(), c.Deleted)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.Rollback)

			// the branch row is gone with its content, so the worker finds
			// nothing under the scope and clears it.
			if c.RespCode == http.StatusNoContent {
				qq := queue.EnqueueCalls()
				require.Len(t, qq, 1)
				assert.Same(t, tx, qq[0].Tx)
				assert.Equal(t, search.BranchScope("org1", _documentID, _branchID), qq[0].J)
			}
		})
	}
}

func Test_Handler_UpdateDocumentBranch(t *testing.T) {
	validBody := `{"name":"Renamed","protected":true}`

	cc := map[string]struct {
		DB                      *DBMock
		Tx                      *TxMock
		Queue                   *SearchQueueMock
		NoSession               bool
		OmitDoc                 bool
		Body                    string
		WantName                string
		RespCode                int
		Updated                 int
		Metadata                int
		Jobs                    int
		FetchDocumentByBranchID int
		BeginTx                 int
		Committed               int
		Rollback                int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusNotFound,
			FetchDocumentByBranchID: 1,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:                    "{",
			RespCode:                http.StatusBadRequest,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by Tx.UpdateDocumentBranchMetadata": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Tx: &TxMock{
				UpdateDocumentBranchMetadataFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Updated:                 1,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
			Rollback:                1,
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Updated:                 1,
			Jobs:                    1,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
			Rollback:                1,
		},
		// the main branch is found by its flag in the tree and content
		// queries, but its name is what the user sees; renaming it would
		// leave the document looking nameless.
		"Renaming the default branch is rejected": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusBadRequest,
			FetchDocumentByBranchID: 1,
		},
		"Protecting the default branch is allowed": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:                    `{"protected":true}`,
			WantName:                documentCore.DefaultBranch,
			RespCode:                http.StatusOK,
			Updated:                 1,
			Metadata:                1,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
			Committed:               1,
			Rollback:                1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Body:                    validBody,
			WantName:                "Renamed",
			RespCode:                http.StatusOK,
			Updated:                 1,
			Metadata:                1,
			Jobs:                    1,
			FetchDocumentByBranchID: 1,
			BeginTx:                 1,
			Committed:               1,
			Rollback:                1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := c.Tx
			if tx == nil {
				tx = &TxMock{}
			}

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(tx, nil)

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})
			hdl.searchQueue = queue

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentBranch(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, tx.UpdateDocumentBranchMetadataCalls(), c.Updated)
			assert.Len(t, tx.CommitCalls(), c.Committed)
			assert.Len(t, tx.RollbackCalls(), c.Rollback)

			qq := queue.EnqueueCalls()
			require.Len(t, qq, c.Jobs)
			assert.Equal(t, c.Metadata, cnt.metadata)

			if c.RespCode == http.StatusOK {
				ff := tx.UpdateDocumentBranchMetadataCalls()
				require.NotEmpty(t, ff)
				assert.Equal(t, c.WantName, ff[0].Doc.BranchName)
				assert.True(t, ff[0].Doc.Protected)
			}

			// a rename resyncs the branch, since every entry carries the
			// branch name.
			if c.Jobs == 1 {
				assert.Same(t, tx, qq[0].Tx)
			}

			if c.Jobs == 1 && c.RespCode == http.StatusOK {
				assert.Equal(t, search.BranchScope("org1", _documentID, _branchID), qq[0].J)
			}
		})
	}
}

func Test_Handler_RecordDocumentBranchView(t *testing.T) {
	cc := map[string]struct {
		DB                      *DBMock
		NoSession               bool
		OmitDoc                 bool
		OmitBranch              bool
		Query                   string
		RespCode                int
		Upserted                int
		From                    search.ViewFrom
		FetchDocumentByBranchID int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid origin": {
			DB:       &DBMock{},
			Query:    "from=tree",
			RespCode: http.StatusBadRequest,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, assert.AnError
				},
			},
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			RespCode:                http.StatusNotFound,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.UpsertDocumentBranchView": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				UpsertDocumentBranchViewFunc: func(context.Context, string, string, xid.ID, search.ViewFrom, time.Time) error {
					return assert.AnError
				},
			},
			RespCode:                http.StatusInternalServerError,
			Upserted:                1,
			From:                    search.ViewFromAll,
			FetchDocumentByBranchID: 1,
		},
		"Successful record": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			RespCode:                http.StatusNoContent,
			Upserted:                1,
			From:                    search.ViewFromAll,
			FetchDocumentByBranchID: 1,
		},
		"Successful record from search": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Query:                   "from=search",
			RespCode:                http.StatusNoContent,
			Upserted:                1,
			From:                    search.ViewFromSearch,
			FetchDocumentByBranchID: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			req := newRequest(http.MethodPost, "", c.NoSession, c.OmitDoc, c.OmitBranch)
			req.URL.RawQuery = c.Query

			rec := httptest.NewRecorder()

			hdl.RecordDocumentBranchView(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)

			ff := c.DB.UpsertDocumentBranchViewCalls()
			require.Len(t, ff, c.Upserted)

			if c.Upserted == 0 {
				return
			}

			assert.Equal(t, "u1", ff[0].UserID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, _branchID, ff[0].BranchID)
			assert.Equal(t, c.From, ff[0].From)
			assert.False(t, ff[0].ViewedAt.IsZero())

			if c.RespCode == http.StatusNoContent {
				assert.Empty(t, rec.Body.String())
			}
		})
	}
}

func Test_Handler_SearchDocumentBranch(t *testing.T) {
	dataSourceID := xid.New()

	chartPage := search.BranchPage{
		Hits: []search.Block{
			{
				ID:       _branchID.String() + "-p1",
				BranchID: _branchID,
				Type:     "paragraph",
				Text:     "<mark>lag</mark> grows",
			},
			{
				ID:       _branchID.String() + "-m1",
				BranchID: _branchID,
				Type:     "metricBlock",
				Text:     "<mark>Lag</mark>",
				Attrs: map[string]string{
					"dataSourceId":      dataSourceID.String(),
					"visualizationType": "bar",
				},
			},
		},
		TotalHits:     7,
		NextPageToken: null.StringFrom("next"),
	}

	// stubSearcher answers every branch search with the page or the error.
	stubSearcher := func(page search.BranchPage, err error) *SearcherMock {
		return &SearcherMock{
			SearchBranchFunc: func(context.Context, search.BranchQuery) (search.BranchPage, error) {
				return page, err
			},
		}
	}

	cc := map[string]struct {
		DB                      *DBMock
		Searcher                *SearcherMock
		NoSession               bool
		OmitDoc                 bool
		OmitBranch              bool
		Query                   string
		RespCode                int
		RespJSON                string
		Check                   func(*testing.T, *DBMock, *SearcherMock)
		FetchDocumentByBranchID int
		SearchBranch            int
	}{
		"No session in context": {
			DB:        &DBMock{},
			Searcher:  &SearcherMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			Searcher: &SearcherMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing branch ID parameter": {
			DB:         &DBMock{},
			Searcher:   &SearcherMock{},
			OmitBranch: true,
			RespCode:   http.StatusNotFound,
		},
		"Invalid limit": {
			DB:       &DBMock{},
			Searcher: &SearcherMock{},
			Query:    "q=lag&limit=abc",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, assert.AnError
				},
			},
			Searcher:                &SearcherMock{},
			Query:                   "q=lag",
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			Searcher: &SearcherMock{},
			Query:    "q=lag",
			RespCode: http.StatusNotFound,
			Check: func(t *testing.T, _ *DBMock, searcher *SearcherMock) {
				t.Helper()

				assert.Empty(t, searcher.SearchBranchCalls())
			},
			FetchDocumentByBranchID: 1,
		},
		"Error returned by Searcher.SearchBranch": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Searcher:                stubSearcher(search.BranchPage{}, assert.AnError),
			Query:                   "q=lag",
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			SearchBranch:            1,
		},
		"Invalid query": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Searcher:                stubSearcher(search.BranchPage{}, search.ErrInvalidQuery),
			RespCode:                http.StatusBadRequest,
			FetchDocumentByBranchID: 1,
			SearchBranch:            1,
		},
		"Successful search": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Searcher: stubSearcher(search.BranchPage{
				Hits:      []search.Block{chartPage.Hits[0]},
				TotalHits: 1,
			}, nil),
			Query:    "q=lag&limit=5&nextToken=token",
			RespCode: http.StatusOK,
			RespJSON: `{
				"totalHits": 1,
				"nextToken": null,
				"hits": [{"id": "p1", "type": "paragraph", "text": "<mark>lag</mark> grows"}]
			}`,
			Check: func(t *testing.T, _ *DBMock, searcher *SearcherMock) {
				t.Helper()

				sf := searcher.SearchBranchCalls()
				require.Len(t, sf, 1)
				assert.Equal(t, search.BranchQuery{
					OrganizationID: "org1",
					BranchID:       _branchID,
					Query:          "lag",
					Limit:          5,
					PageToken:      "token",
				}, sf[0].Bq)
			},
			FetchDocumentByBranchID: 1,
			SearchBranch:            1,
		},
		"Successful search with charts": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Searcher: stubSearcher(chartPage, nil),
			Query:    "q=lag",
			RespCode: http.StatusOK,
			RespJSON: `{
				"totalHits": 7,
				"nextToken": "next",
				"hits": [
					{"id": "p1", "type": "paragraph", "text": "<mark>lag</mark> grows"},
					{
						"id": "m1",
						"type": "metricBlock",
						"text": "<mark>Lag</mark>",
						"attrs": {"dataSourceId": "` + dataSourceID.String() + `", "visualizationType": "bar"}
					}
				]
			}`,
			FetchDocumentByBranchID: 1,
			SearchBranch:            1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.searcher = c.Searcher

			req := newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, c.OmitBranch)
			req.URL.RawQuery = c.Query

			rec := httptest.NewRecorder()

			hdl.SearchDocumentBranch(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.Searcher.SearchBranchCalls(), c.SearchBranch)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			if c.Check != nil {
				c.Check(t, c.DB, c.Searcher)
			}
		})
	}
}
