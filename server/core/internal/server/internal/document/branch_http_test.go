package document

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
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
		State:          null.ValueFrom(processor.State(`{}`)),
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
		BranchID    xid.ID
		// History is how many history entries the persist records.
		History int
		// HistoryBy is who the recorded entry is credited to.
		HistoryBy null.String
	}{
		"Unchanged persist writes no history entry": {
			DB:          &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx:          &TxMock{},
			Body:        `{"name":"Doc","maintainers":["u1","u9"]}`,
			RespCode:    http.StatusOK,
			Committed:   1,
			Metadata:    1,
			Maintainers: 1,
			SearchJobs:  1,
		},
		"Error returned by Tx.RecordDocumentBranchHistoryEntry": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) (xid.ID, error) {
					return xid.ID{}, errors.New("boom")
				},
			},
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			History:   1,
			HistoryBy: null.StringFrom("u9"),
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
			Body:       `{"name":"Renamed","system":true}`,
			RespCode:   http.StatusOK,
			Committed:  1,
			Metadata:   1,
			SearchJobs: 1,
			History:    1,
		},
		"Edit naming no maintainers records an unattributed entry": {
			DB: &DBMock{
				FetchDocumentUnsafeByBranchIDFunc: func(context.Context, xid.ID) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.LastUpdatedBy = null.StringFrom("u5")

					return doc, nil
				},
			},
			Tx:         &TxMock{},
			Body:       `{"name":"Renamed"}`,
			RespCode:   http.StatusOK,
			Committed:  1,
			Metadata:   1,
			SearchJobs: 1,
			History:    1,
		},
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
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			History:   1,
			HistoryBy: null.StringFrom("u9"),
		},
		"Maintainer upsert error": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			History:   1,
			HistoryBy: null.StringFrom("u9"),
		},
		"Search job insertion error on default branch": {
			DB: &DBMock{FetchDocumentUnsafeByBranchIDFunc: fetchStored},
			Tx: &TxMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u9"}, nil
				},
				InsertSearchJobFunc: func(context.Context, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:       validBody,
			RespCode:   http.StatusInternalServerError,
			SearchJobs: 1,
			History:    1,
			HistoryBy:  null.StringFrom("u9"),
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
			History:    1,
			HistoryBy:  null.StringFrom("u9"),
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
			History:     1,
			HistoryBy:   null.StringFrom("u9"),
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
			History:    1,
			HistoryBy:  null.StringFrom("u9"),
		},
		"Non-default branch queues its own search job": {
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
			Body:       validBody,
			RespCode:   http.StatusOK,
			Committed:  1,
			Metadata:   1,
			SearchJobs: 1,
			BranchID:   _branchID2,
			History:    1,
			HistoryBy:  null.StringFrom("u9"),
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
			assert.Len(t, c.Tx.InsertSearchJobCalls(), c.SearchJobs)

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
				job := c.Tx.InsertSearchJobCalls()[0].Job
				assert.Equal(t, c.BranchID, job.BranchID.V)
				assert.True(t, job.DocumentID.Valid)
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

	fetchOtherDocByBranch := func(_ context.Context, branchID xid.ID, _ string) (*documentCore.Document, error) {
		doc := storedDoc()

		if branchID == _branchID2 {
			doc = branchDoc(_branchID2)
		}

		doc.ID = otherDocID

		return doc, nil
	}

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
		History int
	}{
		"History entry insert error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) (xid.ID, error) {
					return xid.ID{}, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			History:  1,
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
			RespCode: http.StatusNotFound,
			RespJSON: `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
		},
		"Branches of another document": {
			DB:       &DBMock{FetchDocumentByBranchIDFunc: fetchOtherDocByBranch},
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusNotFound,
			RespJSON: `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
		},
		"Transaction start error": {
			DB:       &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook detach error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				DetachDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) error {
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
		// the hooks are copied after the commit, so a failure there leaves
		// the merge itself standing.
		"Hook copy fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: fetchByBranch,
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:        &TxMock{},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
			Reviewers: 1,
			History:   1,
		},
		"Hook re-creation error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: fetchByBranch,
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook("bogus")}, nil
				},
			},
			Tx:        &TxMock{},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
			Reviewers: 1,
			History:   1,
		},
		"Hook insertion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: fetchByBranch,
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{storedHook(hookCore.TypeScheduledReminder)}, nil
				},
				InsertDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Tx:        &TxMock{},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
			Reviewers: 1,
			History:   1,
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
			History:  1,
		},
		"Tag replacement error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				ReplaceBranchTagsFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			History:  1,
		},
		"Error returned by tx.FetchDocumentHooksByBranchID": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx: &TxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
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
			History:   1,
		},
		"Successful merge": {
			DB:        &DBMock{FetchDocumentByBranchIDFunc: fetchByBranch},
			Tx:        &TxMock{},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			Metadata:  1,
			Reviewers: 1,
			History:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

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

				pp := man.QueueBranchCalls()
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
		OmitDoc   bool
		Body      string
		RespCode  int
		Committed int
		// History is how many history entries the fork records.
		History int
	}{
		"History entry insert error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) (xid.ID, error) {
					return xid.ID{}, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			History:  1,
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
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusNotFound,
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
		"Branch insert error": {
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
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Error returned by tx.FetchDocumentHooksByBranchID": {
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
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
			History:   1,
		},
		"Search job insert error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				InsertSearchJobFunc: func(context.Context, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			History:  1,
		},
		"Successful branch creation": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:        &TxMock{},
			Body:      validBody,
			RespCode:  http.StatusCreated,
			Committed: 1,
			History:   1,
		},
		"Tag copy error": {
			DB: &DBMock{FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
				return storedDoc(), nil
			}},
			Tx: &TxMock{
				CopyBranchTagsFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			History:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(withTx(c.DB, c.Tx, c.BeginErr), &fakePublisher{})

			man := &HookManagerMock{}
			hdl.hookMan = man

			rec := httptest.NewRecorder()

			hdl.CreateDocumentBranch(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.History)

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

				pp := man.QueueBranchCalls()
				require.Len(t, pp, 1)
				assert.Equal(t, forked.BranchID, pp[0].BranchID)
				assert.Equal(t, "org1", pp[0].OrganizationID)

				// the fork takes the source's tags before the commit.
				require.Len(t, c.Tx.CopyBranchTagsCalls(), 1)
				assert.Equal(t, _branchID, c.Tx.CopyBranchTagsCalls()[0].FromBranchID)
				assert.Equal(t, forked.BranchID, c.Tx.CopyBranchTagsCalls()[0].ToBranchID)

				// the fork is queued under its own branch id before the
				// commit, so the branch is searchable without an edit.
				require.Len(t, c.Tx.InsertSearchJobCalls(), 1)
				assert.Equal(t, search.BranchScope("org1", forked.ID, forked.BranchID), c.Tx.InsertSearchJobCalls()[0].Job)
			}
		})
	}
}

func Test_Handler_DeleteDocumentBranch(t *testing.T) {
	cc := map[string]struct {
		DB         *DBMock
		Tx         *TxMock
		NoSession  bool
		OmitDoc    bool
		OmitBranch bool
		RespCode   int
		RespJSON   string
		Deleted    int
		Committed  int
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
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			RespCode: http.StatusNotFound,
			RespJSON: `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
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
			},
			Tx: &TxMock{
				DeleteDocumentBranchByIDFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Search job insert error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
				CountDocumentBranchesFunc: func(context.Context, xid.ID, string) (int, error) {
					return 2, nil
				},
			},
			Tx: &TxMock{
				InsertSearchJobFunc: func(context.Context, search.Job) error {
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
			RespCode:  http.StatusNoContent,
			Deleted:   1,
			Committed: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := c.Tx
			if tx == nil {
				tx = &TxMock{}
			}

			hdl, _ := newTestHandler(withTx(c.DB, tx, nil), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentBranch(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, tx.DeleteDocumentBranchByIDCalls(), c.Deleted)
			assert.Len(t, tx.CommitCalls(), c.Committed)

			// the branch row is gone with its content, so the worker finds
			// nothing under the scope and clears it.
			if c.RespCode == http.StatusNoContent {
				require.Len(t, tx.InsertSearchJobCalls(), 1)
				assert.Equal(t, search.BranchScope("org1", _documentID, _branchID), tx.InsertSearchJobCalls()[0].Job)
			}
		})
	}
}

func Test_Handler_UpdateDocumentBranch(t *testing.T) {
	validBody := `{"name":"Renamed","protected":true}`

	cc := map[string]struct {
		DB        *DBMock
		Tx        *TxMock
		NoSession bool
		OmitDoc   bool
		Body      string
		WantName  string
		RespCode  int
		Updated   int
		Metadata  int
		Jobs      int
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
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := branchDoc(_branchID)
					doc.ID = xid.New()

					return doc, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Metadata update error": {
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
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Search job insert error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Tx: &TxMock{
				InsertSearchJobFunc: func(context.Context, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
			Jobs:     1,
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
			Body:     validBody,
			RespCode: http.StatusBadRequest,
		},
		"Protecting the default branch is allowed": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:     `{"protected":true}`,
			WantName: documentCore.DefaultBranch,
			RespCode: http.StatusOK,
			Updated:  1,
			Metadata: 1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return branchDoc(_branchID), nil
				},
			},
			Body:     validBody,
			WantName: "Renamed",
			RespCode: http.StatusOK,
			Updated:  1,
			Metadata: 1,
			Jobs:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := c.Tx
			if tx == nil {
				tx = &TxMock{}
			}

			hdl, cnt := newTestHandler(withTx(c.DB, tx, nil), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentBranch(rec, newRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, tx.UpdateDocumentBranchMetadataCalls(), c.Updated)
			assert.Len(t, tx.InsertSearchJobCalls(), c.Jobs)
			assert.Equal(t, c.Metadata, cnt.metadata)

			if c.RespCode == http.StatusOK {
				ff := tx.UpdateDocumentBranchMetadataCalls()
				require.NotEmpty(t, ff)
				assert.Equal(t, c.WantName, ff[0].Doc.BranchName)
				assert.True(t, ff[0].Doc.Protected)
				assert.Len(t, tx.CommitCalls(), 1)
			}

			// a rename resyncs the branch, since every entry carries the
			// branch name.
			if c.Jobs == 1 && c.RespCode == http.StatusOK {
				assert.Equal(t, search.BranchScope("org1", _documentID, _branchID), tx.InsertSearchJobCalls()[0].Job)
			}
		})
	}
}
