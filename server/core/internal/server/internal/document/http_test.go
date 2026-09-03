package document

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
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
		log:             slog.New(slog.DiscardHandler),
		db:              db,
		notifPub:        pub,
		storer:          &StorerMock{},
		webchangeClient: webchange.NewClient("", ""),
		searchJobs:      search.NewJobs(true),
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

		*dest.(*Tx) = tx

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
		BranchID:       _branchID,
		BranchName:     documentCore.DefaultBranch,
		DocumentName:   "Doc",
		Default:        true,
	}
}

// branchDoc builds a stored document on a non-default branch.
func branchDoc(branchID xid.ID) *documentCore.Document {
	return &documentCore.Document{
		ID:             _documentID,
		OrganizationID: "org1",
		BranchID:       branchID,
		BranchName:     "feature",
		DocumentName:   "Doc",
	}
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	gw := &SearchGatewayMock{}
	jobs := search.NewJobs(true)
	pub := &fakePublisher{}
	st := &StorerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, nil, nil, gw, jobs, pub, st)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, gw, hdl.searchGateway)
	assert.Same(t, jobs, hdl.searchJobs)
	assert.Same(t, pub, hdl.notifPub)
	assert.Same(t, st, hdl.storer)
	assert.Nil(t, hdl.githubMan)
	assert.Nil(t, hdl.webchangeClient)
}

func Test_Handler_RequireDocumentAccess(t *testing.T) {
	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitDoc   bool
		RespCode  int
		Passed    bool
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
		"Document existence check error": {
			DB: &DBMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Document outside the caller's organization": {
			DB: &DBMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
		},
		"Document within the caller's organization": {
			DB:       &DBMock{},
			RespCode: http.StatusTeapot,
			Passed:   true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			var passed bool

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				passed = true

				w.WriteHeader(http.StatusTeapot)
			})

			rec := httptest.NewRecorder()

			hdl.RequireDocumentAccess(next).
				ServeHTTP(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, c.Passed, passed)

			if !c.Passed {
				return
			}

			ff := c.DB.CheckDocumentExistsCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _documentID, ff[0].ID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
		})
	}
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

func Test_Handler_VerifyDocumentAccess(t *testing.T) {
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
		"Document fetch error": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Document outside the caller's organization": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
		},
		"Successful check": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode: http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.VerifyDocumentAccess(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusNoContent {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())

				ff := c.DB.FetchDocumentCalls()
				require.Len(t, ff, 1)
				assert.Equal(t, _documentID, ff[0].ID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
				assert.Equal(t, documentCore.DefaultBranch, ff[0].BranchName)
			}
		})
	}
}

func Test_Handler_FetchBranchReviewers(t *testing.T) {
	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		OmitDoc    bool
		OmitBranch bool
		RespCode   int
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
					doc := storedDoc()
					doc.ID = xid.New()

					return doc, nil
				},
			},
			RespCode: http.StatusNotFound,
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

			if c.DB.FetchDocumentByBranchIDFunc == nil {
				c.DB.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				}
			}

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchBranchReviewers(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, c.OmitBranch))

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
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Query:    "?userId=u2",
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
			Query:    "?userId=u2",
			RespCode: http.StatusNotFound,
		},
		"Reviewer deletion error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				DeleteBranchReviewerFunc: func(context.Context, xid.ID, string, string) error {
					return errors.New("boom")
				},
			},
			Query:    "?userId=u2",
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful removal": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Query:     "?userId=u2",
			RespCode:  http.StatusNoContent,
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

			// PreviouslyApproved is deliberately not asserted: the update
			// query persists currently_approved alone, so whatever the
			// handler puts in that field never reaches the row.
			if c.Updated == 1 && c.RespCode == http.StatusOK {
				assert.True(t, c.DB.UpdateBranchReviewerCalls()[0].Reviewer.CurrentlyApproved)
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

	t.Run("Response carries the swapped ordering", func(t *testing.T) {
		t.Parallel()

		moved, other := _documentID, xid.New()

		tx := &TxMock{
			FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
				return storedDoc(), nil
			},
			FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
				return documentCore.Summaries{{ID: other}, {ID: moved}}, nil
			},
		}

		hdl, _ := newTestHandler(withTx(&DBMock{}, tx, nil), &fakePublisher{})

		rec := httptest.NewRecorder()

		body := `{"id":"` + moved.String() + `","sortIndex":0}`
		hdl.UpdateDocumentTree(rec, newRequest(http.MethodPut, body, false, true, true))

		require.Equal(t, http.StatusOK, rec.Code)

		// the persisted ordering is the swapped one, so the client must not
		// be handed back the ordering it asked to change.
		var got documentCore.Summaries

		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got, 2)
		assert.Equal(t, moved, got[0].ID)
	})
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
		"Search not configured": {
			Gateway: &SearchGatewayMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return nil, search.ErrNotConfigured
				},
			},
			Query:    "?q=test",
			RespCode: http.StatusConflict,
			RespBody: `{"code":"search.not_configured","message":"search is not configured"}`,
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
				InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
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
			assert.Len(t, tx.InsertDocumentSearchJobCalls(), c.Jobs)
			assert.Equal(t, c.Metadata, cnt.metadata)

			if c.RespCode == http.StatusOK {
				ff := tx.UpdateDocumentBranchMetadataCalls()
				require.NotEmpty(t, ff)
				assert.Equal(t, c.WantName, ff[0].Doc.BranchName)
				assert.True(t, ff[0].Doc.Protected)
				assert.Len(t, tx.CommitCalls(), 1)
			}

			// a rename rewrites every entry of the branch, since each one
			// carries the branch name.
			if c.Jobs == 1 && c.RespCode == http.StatusOK {
				diff := tx.InsertDocumentSearchJobCalls()[0].Diff
				require.Len(t, diff.Updated, 1)
				assert.Equal(t, _branchID.String()+"-docname", diff.Updated[0].ID)
				assert.Equal(t, "Renamed", diff.Updated[0].BranchName)
			}
		})
	}
}
