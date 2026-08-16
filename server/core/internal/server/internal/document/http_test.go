package document

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fixed IDs used across test requests.
var (
	_documentID = xid.New()
	_branchID   = xid.New()
	_branchID2  = xid.New()
)

// fakePublisher captures published notifications.
type fakePublisher struct {
	calls []struct {
		OrganizationID string
		Core           notification.Core
		UserIDs        []string
	}
}

func (p *fakePublisher) PublishNotifications(organizationID string, nc notification.Core, userIDs ...string) {
	p.calls = append(p.calls, struct {
		OrganizationID string
		Core           notification.Core
		UserIDs        []string
	}{organizationID, nc, userIDs})
}

// callbackCounts tracks how many times each ws change callback fired.
type callbackCounts struct {
	tree, metadata, reviewers, maintainers int
}

// newTestHandler builds a Handler with all ws change callbacks counting
// into the returned callbackCounts.
func newTestHandler(db DB, pub *fakePublisher) (*Handler, *callbackCounts) {
	cnt := &callbackCounts{}

	hdl := &Handler{
		log:      slog.New(slog.DiscardHandler),
		db:       db,
		notifPub: pub,
	}

	hdl.tree.changeCallback = func(string, null.Value[xid.ID]) { cnt.tree++ }
	hdl.metadata.changeCallback = func(string, documentCore.Document) { cnt.metadata++ }
	hdl.reviewers.changeCallback = func(string, xid.ID) { cnt.reviewers++ }
	hdl.maintainers.changeCallback = func(string, xid.ID) { cnt.maintainers++ }

	return hdl, cnt
}

// withTx wires the DB mock's BeginTx to hand out the provided Tx mock.
func withTx(db *DBMock, tx *TxMock, err error) *DBMock {
	db.BeginTxFunc = func(_ context.Context, dest any) error {
		if err != nil {
			return err
		}

		*(dest.(*Tx)) = tx

		return nil
	}

	return db
}

// newRequest builds a request carrying a session and the standard chi
// parameters unless the respective omit flags are set.
func newRequest(method, body string, noSession, omitDoc, omitBranch bool) *http.Request {
	req := httptest.NewRequest(method, "http://test.com/", http.NoBody)

	if body != "" {
		req = httptest.NewRequest(method, "http://test.com/", strings.NewReader(body))
	}

	ctx := req.Context()

	if !noSession {
		ctx = auth.AddSessionToContext(ctx, auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: "org1",
		})
	}

	if !omitDoc {
		ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
	}

	if !omitBranch {
		ctx = testutil.AddChiCtx(ctx, "branchId", _branchID.String())
	}

	return req.WithContext(ctx)
}

// storedDoc builds a stored document on its default branch.
func storedDoc() *documentCore.Document {
	return &documentCore.Document{
		ID:             _documentID,
		OrganizationID: "org1",
		Branch: documentCore.Branch{
			BranchID:     _branchID,
			BranchName:   documentCore.DefaultBranch,
			DocumentName: "Doc",
			Default:      true,
		},
	}
}

// branchDoc builds a stored document on a non-default branch.
func branchDoc(branchID xid.ID) *documentCore.Document {
	return &documentCore.Document{
		ID:             _documentID,
		OrganizationID: "org1",
		Branch: documentCore.Branch{
			BranchID:     branchID,
			BranchName:   "feature",
			DocumentName: "Doc",
		},
	}
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	gw := &SearchGatewayMock{}
	pub := &fakePublisher{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, nil, nil, gw, pub)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, gw, hdl.searchGateway)
	assert.Same(t, pub, hdl.notifPub)
	assert.Nil(t, hdl.githubMan)
	assert.Nil(t, hdl.webchangeClient)
}

func Test_Handler_FetchDocumentMaintainers(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		RespCode  int
		RespJSON  string
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
		"Maintainer fetch error": {
			DB: &DBMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u2"}, nil
				},
			},
			RespCode: http.StatusOK,
			RespJSON: `["u1","u2"]`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentMaintainers(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}
		})
	}
}

func Test_Handler_FetchBranchReviewers(t *testing.T) {
	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		OmitBranch bool
		RespCode   int
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
		"Reviewer fetch error": {
			DB: &DBMock{
				FetchBranchReviewersFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchReviewer, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchBranchReviewersFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchReviewer, error) {
					return []documentCore.BranchReviewer{{BranchID: _branchID, UserID: "u2"}}, nil
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

			hdl.FetchBranchReviewers(rec, newRequest(http.MethodGet, "", c.NoSession, true, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), `"u2"`)
			}
		})
	}
}

func Test_Handler_RequestBranchReviewer(t *testing.T) {
	validBody := `{"userId":"u2"}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		Body      string
		RespCode  int
		Inserted  int
		Updated   int
		Notified  int
		Reviewers int
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
					doc := storedDoc()
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
		"Member check error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return false, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Not an organization member": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return false, nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusForbidden,
		},
		"Reviewer lookup error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Existing reviewer update error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
				UpdateBranchReviewerFunc: func(context.Context, documentCore.BranchReviewer) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Existing reviewer re-request": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
			},
			Body:      validBody,
			RespCode:  http.StatusCreated,
			Updated:   1,
			Notified:  1,
			Reviewers: 1,
		},
		"New reviewer insertion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errutil.ErrNotFound
				},
				InsertBranchReviewerFunc: func(context.Context, documentCore.BranchReviewer) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		"New reviewer request": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Body:      validBody,
			RespCode:  http.StatusCreated,
			Inserted:  1,
			Notified:  1,
			Reviewers: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pub := &fakePublisher{}
			hdl, cnt := newTestHandler(c.DB, pub)

			rec := httptest.NewRecorder()

			hdl.RequestBranchReviewer(rec, newRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertBranchReviewerCalls(), c.Inserted)
			assert.Len(t, c.DB.UpdateBranchReviewerCalls(), c.Updated)
			require.Len(t, pub.calls, c.Notified)
			assert.Equal(t, c.Reviewers, cnt.reviewers)

			if c.Notified > 0 {
				assert.Equal(t, []string{"u2"}, pub.calls[0].UserIDs)
			}
		})
	}
}

func Test_Handler_RemoveBranchReviewer(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		Query     string
		RespCode  int
		Deleted   int
		Reviewers int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Query:     "?userId=u2",
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       &DBMock{},
			OmitDoc:  true,
			Query:    "?userId=u2",
			RespCode: http.StatusNotFound,
		},
		"Missing user ID query parameter": {
			DB:       &DBMock{},
			RespCode: http.StatusBadRequest,
		},
		"Reviewer deletion error": {
			DB: &DBMock{
				DeleteBranchReviewerFunc: func(context.Context, xid.ID, string, string) error {
					return errors.New("boom")
				},
			},
			Query:    "?userId=u2",
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful removal": {
			DB:        &DBMock{},
			Query:     "?userId=u2",
			RespCode:  http.StatusOK,
			Deleted:   1,
			Reviewers: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/"+c.Query, http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			if !c.OmitDoc {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			ctx = testutil.AddChiCtx(ctx, "branchId", _branchID.String())

			rec := httptest.NewRecorder()

			hdl.RemoveBranchReviewer(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.DeleteBranchReviewerCalls(), c.Deleted)
			assert.Equal(t, c.Reviewers, cnt.reviewers)
		})
	}
}

func Test_Handler_UpdateBranchReviewApproval(t *testing.T) {
	validBody := `{"approved":true}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Body      string
		RespCode  int
		Inserted  int
		Updated   int
		Reviewers int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
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
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Reviewer lookup error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Existing reviewer update error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return &documentCore.BranchReviewer{PreviouslyApproved: true}, nil
				},
				UpdateBranchReviewerFunc: func(context.Context, documentCore.BranchReviewer) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Existing reviewer approval": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return &documentCore.BranchReviewer{PreviouslyApproved: true}, nil
				},
			},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Updated:   1,
			Reviewers: 1,
		},
		"New reviewer insertion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errutil.ErrNotFound
				},
				InsertBranchReviewerFunc: func(context.Context, documentCore.BranchReviewer) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		"New reviewer approval": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Body:      validBody,
			RespCode:  http.StatusOK,
			Inserted:  1,
			Reviewers: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateBranchReviewApproval(rec, newRequest(http.MethodPut, c.Body, c.NoSession, false, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertBranchReviewerCalls(), c.Inserted)
			assert.Len(t, c.DB.UpdateBranchReviewerCalls(), c.Updated)
			assert.Equal(t, c.Reviewers, cnt.reviewers)

			if c.Updated == 1 && c.RespCode == http.StatusOK {
				assert.True(t, c.DB.UpdateBranchReviewerCalls()[0].Reviewer.CurrentlyApproved)
				assert.True(t, c.DB.UpdateBranchReviewerCalls()[0].Reviewer.PreviouslyApproved)
			}
		})
	}
}

func Test_Handler_FetchDocumentTree(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Tree fetch error": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (documentCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: _documentID, DocumentName: "Doc"}}, nil
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

			hdl.FetchDocumentTree(rec, newRequest(http.MethodGet, "", c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), _documentID.String())
			}
		})
	}
}

func Test_Handler_UpdateDocumentTree(t *testing.T) {
	sameParentBody := `{"id":"` + _documentID.String() + `","sortIndex":0}`
	newParentID := xid.New()
	newParentBody := `{"id":"` + _documentID.String() + `","parentId":"` + newParentID.String() + `","sortIndex":0}`

	// sameParentTx serves a document already under the requested parent.
	sameParentTx := func() *TxMock {
		return &TxMock{
			FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
				return storedDoc(), nil
			},
			FetchDocumentTreeByDocumentParentIDFunc: func(_ context.Context, _ null.Value[xid.ID], _ string) (documentCore.Summaries, error) {
				return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
			},
		}
	}

	cc := map[string]struct {
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		Body      string
		RespCode  int
		Committed int
		TreeCbs   int
	}{
		"No session in context": {
			Tx:        &TxMock{},
			NoSession: true,
			Body:      sameParentBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Transaction start error": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     sameParentBody,
			RespCode: http.StatusInternalServerError,
		},
		"Document fetch error": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     sameParentBody,
			RespCode: http.StatusInternalServerError,
		},
		"Document missing from tree": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: xid.New()}}, nil
				},
			},
			Body:     sameParentBody,
			RespCode: http.StatusNotFound,
		},
		"Tree update error": {
			Tx: func() *TxMock {
				tx := sameParentTx()
				tx.UpdateDocumentTreeFunc = func(context.Context, documentCore.Summaries, string) error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:     sameParentBody,
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			Tx: func() *TxMock {
				tx := sameParentTx()
				tx.CommitFunc = func() error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:      sameParentBody,
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful reorder within the same parent": {
			Tx:        sameParentTx(),
			Body:      sameParentBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			TreeCbs:   1,
		},
		"Successful move to a new parent": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(_ context.Context, parentID null.Value[xid.ID], _ string) (documentCore.Summaries, error) {
					if parentID.Valid {
						// the requested new parent subtree.
						return documentCore.Summaries{{ID: xid.New()}}, nil
					}

					// the old (root) subtree still holding the document.
					return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
				},
			},
			Body:      newParentBody,
			RespCode:  http.StatusOK,
			Committed: 1,
			TreeCbs:   2,
		},
		"New parent does not exist": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			Body:     newParentBody,
			RespCode: http.StatusNotFound,
		},
		"New parent cycle check error": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckDocumentCycleFunc: func(context.Context, xid.ID, xid.ID, string) (bool, error) {
					return false, errors.New("boom")
				},
			},
			Body:     newParentBody,
			RespCode: http.StatusInternalServerError,
		},
		"New parent is the document or its descendant": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckDocumentCycleFunc: func(context.Context, xid.ID, xid.ID, string) (bool, error) {
					return true, nil
				},
			},
			Body:     newParentBody,
			RespCode: http.StatusBadRequest,
		},
		"Move with parent update error": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(_ context.Context, parentID null.Value[xid.ID], _ string) (documentCore.Summaries, error) {
					if parentID.Valid {
						return documentCore.Summaries{{ID: xid.New()}}, nil
					}

					return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
				},
				UpdateDocumentParentIDFunc: func(context.Context, xid.ID, null.Value[xid.ID], string) error {
					return errors.New("boom")
				},
			},
			Body:     newParentBody,
			RespCode: http.StatusInternalServerError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(&DBMock{}, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentTree(rec, newRequest(http.MethodPut, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.TreeCbs, cnt.tree)
		})
	}
}

func Test_Handler_SearchDocuments(t *testing.T) {
	cc := map[string]struct {
		Gateway   *SearchGatewayMock
		NoSession bool
		Query     string
		RespCode  int
		RespBody  string
	}{
		"No session in context": {
			Gateway:   &SearchGatewayMock{},
			NoSession: true,
			Query:     "?q=test",
			RespCode:  http.StatusUnauthorized,
		},
		"Empty search query": {
			Gateway:  &SearchGatewayMock{},
			RespCode: http.StatusBadRequest,
		},
		"Search gateway error": {
			Gateway: &SearchGatewayMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return nil, errors.New("boom")
				},
			},
			Query:    "?q=test",
			RespCode: http.StatusInternalServerError,
		},
		"Successful search": {
			Gateway: &SearchGatewayMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return []byte(`{"hits":[]}`), nil
				},
			},
			Query:    "?q=test",
			RespCode: http.StatusOK,
			RespBody: `{"hits":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(&DBMock{}, &fakePublisher{})
			hdl.searchGateway = c.Gateway

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.SearchDocuments(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespBody != "" {
				assert.JSONEq(t, c.RespBody, rec.Body.String())
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func Test_Handler_CreateDocument(t *testing.T) {
	validBody := `{"name":"New doc","icon":"icon"}`
	parentBody := `{"name":"New doc","icon":"icon","parentId":"` + xid.New().String() + `"}`

	// insertAwareTx returns the freshly inserted document in the parent
	// subtree so the sort-index swap can find it.
	insertAwareTx := func() *TxMock {
		tx := &TxMock{}
		tx.FetchDocumentTreeByDocumentParentIDFunc = func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
			return documentCore.Summaries{{ID: tx.InsertDocumentCalls()[0].Doc.ID}}, nil
		}

		return tx
	}

	cc := map[string]struct {
		Tx        *TxMock
		BeginErr  error
		NoSession bool
		Body      string
		RespCode  int
		Committed int
		TreeCbs   int
	}{
		"No session in context": {
			Tx:        &TxMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			Tx:       &TxMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Transaction start error": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Parent document does not exist": {
			Tx: &TxMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			Body:     parentBody,
			RespCode: http.StatusNotFound,
		},
		"Document insertion error": {
			Tx: &TxMock{
				InsertDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Maintainer upsert error": {
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Search job insertion error": {
			Tx: &TxMock{
				InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Tree fetch error": {
			Tx: &TxMock{
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Sort index swap error": {
			Tx:       &TxMock{},
			Body:     validBody,
			RespCode: http.StatusBadRequest,
		},
		"Tree update error": {
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.UpdateDocumentTreeFunc = func(context.Context, documentCore.Summaries, string) error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Commit error": {
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.CommitFunc = func() error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:      validBody,
			RespCode:  http.StatusInternalServerError,
			Committed: 1,
		},
		"Successful creation": {
			Tx:        insertAwareTx(),
			Body:      validBody,
			RespCode:  http.StatusCreated,
			Committed: 1,
			TreeCbs:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(withTx(&DBMock{}, c.Tx, c.BeginErr), &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.CreateDocument(rec, newRequest(http.MethodPost, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.RespCode == http.StatusCreated {
				assert.Contains(t, rec.Body.String(), `"New doc"`)
				assert.Equal(t, []string{"u1"}, c.Tx.UpsertDocumentMaintainersCalls()[0].MaintainerIDs)
			}
		})
	}
}

func Test_Handler_UpdateDocumentBranch(t *testing.T) {
	validBody := `{"name":"Renamed","protected":true}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Body      string
		RespCode  int
		Updated   int
		Metadata  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
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
					return storedDoc(), nil
				},
				UpdateDocumentBranchMetadataFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Body:     validBody,
			RespCode: http.StatusOK,
			Updated:  1,
			Metadata: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentBranch(rec, newRequest(http.MethodPut, c.Body, c.NoSession, true, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UpdateDocumentBranchMetadataCalls(), c.Updated)
			assert.Equal(t, c.Metadata, cnt.metadata)

			if c.RespCode == http.StatusOK {
				assert.Equal(t, "Renamed", c.DB.UpdateDocumentBranchMetadataCalls()[0].Doc.BranchName)
				assert.True(t, c.DB.UpdateDocumentBranchMetadataCalls()[0].Doc.Protected)
			}
		})
	}
}
