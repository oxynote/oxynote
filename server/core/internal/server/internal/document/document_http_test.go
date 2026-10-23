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
	"time"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.IgnoreBleveWorkers())
}

// fixed IDs used across test requests.
var (
	_documentID = xid.New()
	_branchID   = xid.New()
	_branchID2  = xid.New()

	_dataSourceID = xid.New()
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
		log:         slog.New(slog.DiscardHandler),
		db:          db,
		notifPub:    pub,
		storer:      &StorerMock{},
		hookMan:     &HookManagerMock{},
		searchQueue: &SearchQueueMock{},
	}

	hdl.tree.changeCallback = func(string, null.Value[xid.ID]) { cnt.tree++ }
	hdl.metadata.changeCallback = func(string, documentCore.Document) { cnt.metadata++ }
	hdl.reviewers.changeCallback = func(string, xid.ID) { cnt.reviewers++ }
	hdl.maintainers.changeCallback = func(string, xid.ID) { cnt.maintainers++ }

	return hdl, cnt
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

// insertAwareTx builds a Tx mock whose parent subtree holds the freshly
// inserted document, so the sort-index swap can find it.
func insertAwareTx() *TxMock {
	tx := &TxMock{}
	tx.FetchDocumentTreeByDocumentParentIDFunc = func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
		return documentCore.Summaries{{ID: tx.InsertDocumentCalls()[0].Doc.ID}}, nil
	}

	return tx
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	searcher := &SearcherMock{}
	queue := &SearchQueueMock{}
	pub := &fakePublisher{}
	st := &StorerMock{}
	man := &HookManagerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, man, searcher, queue, pub, st)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, man, hdl.hookMan)
	assert.Same(t, searcher, hdl.searcher)
	assert.Same(t, queue, hdl.searchQueue)
	assert.Same(t, pub, hdl.notifPub)
	assert.Same(t, st, hdl.storer)
}

func Test_Handler_RequireDocumentAccess(t *testing.T) {
	cc := map[string]struct {
		DB                  *DBMock
		NoSession           bool
		OmitDoc             bool
		RespCode            int
		Passed              bool
		CheckDocumentExists int
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
		"Error returned by DB.CheckDocumentExists": {
			DB: &DBMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
				},
			},
			RespCode:            http.StatusInternalServerError,
			CheckDocumentExists: 1,
		},
		"Document in another organization": {
			DB: &DBMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			RespCode:            http.StatusNotFound,
			CheckDocumentExists: 1,
		},
		"Document in the active organization": {
			DB:                  &DBMock{},
			RespCode:            http.StatusTeapot,
			Passed:              true,
			CheckDocumentExists: 1,
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
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.CheckDocumentExists)

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
		DB                       *DBMock
		NoSession                bool
		OmitDoc                  bool
		RespCode                 int
		RespJSON                 string
		FetchDocumentMaintainers int
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
		"Error returned by DB.FetchDocumentMaintainers": {
			DB: &DBMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:                 http.StatusInternalServerError,
			FetchDocumentMaintainers: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"u1", "u2"}, nil
				},
			},
			RespCode:                 http.StatusOK,
			RespJSON:                 `["u1","u2"]`,
			FetchDocumentMaintainers: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentMaintainers(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentMaintainersCalls(), c.FetchDocumentMaintainers)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}
		})
	}
}

func Test_Handler_VerifyDocumentAccess(t *testing.T) {
	cc := map[string]struct {
		DB            *DBMock
		NoSession     bool
		OmitDoc       bool
		RespCode      int
		FetchDocument int
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
		"Error returned by DB.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:      http.StatusInternalServerError,
			FetchDocument: 1,
		},
		"Document in another organization": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode:      http.StatusNotFound,
			FetchDocument: 1,
		},
		"Successful check": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			RespCode:      http.StatusNoContent,
			FetchDocument: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.VerifyDocumentAccess(rec, newRequest(http.MethodGet, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCalls(), c.FetchDocument)

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
		DB                      *DBMock
		NoSession               bool
		OmitDoc                 bool
		OmitBranch              bool
		RespCode                int
		FetchDocumentByBranchID int
		FetchBranchReviewers    int
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
					doc := storedDoc()
					doc.ID = xid.New()

					return doc, nil
				},
			},
			RespCode:                http.StatusNotFound,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.FetchBranchReviewers": {
			DB: &DBMock{
				FetchBranchReviewersFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchReviewer, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewers:    1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchBranchReviewersFunc: func(context.Context, xid.ID, string) ([]documentCore.BranchReviewer, error) {
					return []documentCore.BranchReviewer{{BranchID: _branchID, UserID: "u2"}}, nil
				},
			},
			RespCode:                http.StatusOK,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewers:    1,
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
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.FetchBranchReviewersCalls(), c.FetchBranchReviewers)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), `"u2"`)
			}
		})
	}
}

func Test_Handler_RequestBranchReviewer(t *testing.T) {
	validBody := `{"userId":"u2"}`

	cc := map[string]struct {
		DB                      *DBMock
		NoSession               bool
		OmitDoc                 bool
		Body                    string
		RespCode                int
		Inserted                int
		Updated                 int
		Notified                int
		Reviewers               int
		FetchDocumentByBranchID int
		CheckOrganizationMember int
		FetchBranchReviewer     int
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
					doc := storedDoc()
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
		"Error returned by DB.CheckOrganizationMember": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return false, errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
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
			Body:                    validBody,
			RespCode:                http.StatusForbidden,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
		},
		"Error returned by DB.FetchBranchReviewer": {
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
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
			FetchBranchReviewer:     1,
		},
		"Error returned by DB.UpdateBranchReviewer": {
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
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Updated:                 1,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
			FetchBranchReviewer:     1,
		},
		"Existing reviewer requested again": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckOrganizationMemberFunc: func(context.Context, string, string) (bool, error) {
					return true, nil
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusCreated,
			Updated:                 1,
			Notified:                1,
			Reviewers:               1,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
			FetchBranchReviewer:     1,
		},
		"Error returned by DB.InsertBranchReviewer": {
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
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Inserted:                1,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
			FetchBranchReviewer:     1,
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
			Body:                    validBody,
			RespCode:                http.StatusCreated,
			Inserted:                1,
			Notified:                1,
			Reviewers:               1,
			FetchDocumentByBranchID: 1,
			CheckOrganizationMember: 1,
			FetchBranchReviewer:     1,
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
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.CheckOrganizationMemberCalls(), c.CheckOrganizationMember)
			assert.Len(t, c.DB.FetchBranchReviewerCalls(), c.FetchBranchReviewer)
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
		DB                      *DBMock
		NoSession               bool
		OmitDoc                 bool
		Query                   string
		RespCode                int
		Deleted                 int
		Reviewers               int
		FetchDocumentByBranchID int
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
		"Error returned by DB.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Query:                   "?userId=u2",
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
		},
		"Branch of another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					doc := storedDoc()
					doc.ID = xid.New()

					return doc, nil
				},
			},
			Query:                   "?userId=u2",
			RespCode:                http.StatusNotFound,
			FetchDocumentByBranchID: 1,
		},
		"Error returned by DB.DeleteBranchReviewer": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				DeleteBranchReviewerFunc: func(context.Context, xid.ID, string, string) error {
					return errors.New("boom")
				},
			},
			Query:                   "?userId=u2",
			RespCode:                http.StatusInternalServerError,
			Deleted:                 1,
			FetchDocumentByBranchID: 1,
		},
		"Successful removal": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Query:                   "?userId=u2",
			RespCode:                http.StatusNoContent,
			Deleted:                 1,
			Reviewers:               1,
			FetchDocumentByBranchID: 1,
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
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.DeleteBranchReviewerCalls(), c.Deleted)
			assert.Equal(t, c.Reviewers, cnt.reviewers)
		})
	}
}

func Test_Handler_UpdateBranchReviewApproval(t *testing.T) {
	validBody := `{"approved":true}`

	cc := map[string]struct {
		DB                      *DBMock
		NoSession               bool
		Body                    string
		RespCode                int
		Inserted                int
		Updated                 int
		Reviewers               int
		FetchDocumentByBranchID int
		FetchBranchReviewer     int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
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
					doc := storedDoc()
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
		"Error returned by DB.FetchBranchReviewer": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchBranchReviewerFunc: func(context.Context, xid.ID, string, string) (*documentCore.BranchReviewer, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewer:     1,
		},
		"Error returned by DB.UpdateBranchReviewer": {
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
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Updated:                 1,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewer:     1,
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
			Body:                    validBody,
			RespCode:                http.StatusOK,
			Updated:                 1,
			Reviewers:               1,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewer:     1,
		},
		"Error returned by DB.InsertBranchReviewer": {
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
			Body:                    validBody,
			RespCode:                http.StatusInternalServerError,
			Inserted:                1,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewer:     1,
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
			Body:                    validBody,
			RespCode:                http.StatusOK,
			Inserted:                1,
			Reviewers:               1,
			FetchDocumentByBranchID: 1,
			FetchBranchReviewer:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateBranchReviewApproval(rec, newRequest(http.MethodPut, c.Body, c.NoSession, false, false))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchID)
			assert.Len(t, c.DB.FetchBranchReviewerCalls(), c.FetchBranchReviewer)
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
		DB                *DBMock
		NoSession         bool
		RespCode          int
		FetchDocumentTree int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Error returned by DB.FetchDocumentTree": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (documentCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:          http.StatusInternalServerError,
			FetchDocumentTree: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: _documentID, DocumentName: "Doc"}}, nil
				},
			},
			RespCode:          http.StatusOK,
			FetchDocumentTree: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.FetchDocumentTree(rec, newRequest(http.MethodGet, "", c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentTreeCalls(), c.FetchDocumentTree)

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

	cc := map[string]struct {
		Tx                                  *TxMock
		BeginErr                            error
		NoSession                           bool
		Body                                string
		RespCode                            int
		Committed                           int
		TreeCbs                             int
		BeginTx                             int
		FetchDocument                       int
		CheckDocumentExists                 int
		CheckDocumentCycle                  int
		FetchDocumentTreeByDocumentParentID int
		UpdateDocumentTree                  int
		UpdateDocumentParentID              int
		Rollback                            int
		Check                               func(*testing.T, *httptest.ResponseRecorder)
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
		"Error returned by DB.BeginTx": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     sameParentBody,
			RespCode: http.StatusInternalServerError,
			BeginTx:  1,
		},
		"Error returned by Tx.FetchDocument": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body:          sameParentBody,
			RespCode:      http.StatusInternalServerError,
			BeginTx:       1,
			FetchDocument: 1,
			Rollback:      1,
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
			Body:                                sameParentBody,
			RespCode:                            http.StatusNotFound,
			BeginTx:                             1,
			FetchDocument:                       1,
			FetchDocumentTreeByDocumentParentID: 1,
			Rollback:                            1,
		},
		"Error returned by Tx.UpdateDocumentTree": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
				},
				UpdateDocumentTreeFunc: func(context.Context, documentCore.Summaries, string) error {
					return errors.New("boom")
				},
			},
			Body:                                sameParentBody,
			RespCode:                            http.StatusInternalServerError,
			BeginTx:                             1,
			FetchDocument:                       1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
		"Error returned by Tx.Commit": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
				},
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			Body:                                sameParentBody,
			RespCode:                            http.StatusInternalServerError,
			Committed:                           1,
			BeginTx:                             1,
			FetchDocument:                       1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
		"Successful reorder within the same parent": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: _documentID}, {ID: xid.New()}}, nil
				},
			},
			Body:                                sameParentBody,
			RespCode:                            http.StatusOK,
			Committed:                           1,
			TreeCbs:                             1,
			BeginTx:                             1,
			FetchDocument:                       1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
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
			Body:                                newParentBody,
			RespCode:                            http.StatusOK,
			Committed:                           1,
			TreeCbs:                             2,
			BeginTx:                             1,
			FetchDocument:                       1,
			CheckDocumentExists:                 1,
			CheckDocumentCycle:                  1,
			FetchDocumentTreeByDocumentParentID: 2,
			UpdateDocumentTree:                  1,
			UpdateDocumentParentID:              1,
			Rollback:                            1,
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
			Body:                newParentBody,
			RespCode:            http.StatusNotFound,
			BeginTx:             1,
			FetchDocument:       1,
			CheckDocumentExists: 1,
			Rollback:            1,
		},
		"Error returned by Tx.CheckDocumentCycle": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				CheckDocumentCycleFunc: func(context.Context, xid.ID, xid.ID, string) (bool, error) {
					return false, errors.New("boom")
				},
			},
			Body:                newParentBody,
			RespCode:            http.StatusInternalServerError,
			BeginTx:             1,
			FetchDocument:       1,
			CheckDocumentExists: 1,
			CheckDocumentCycle:  1,
			Rollback:            1,
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
			Body:                newParentBody,
			RespCode:            http.StatusBadRequest,
			BeginTx:             1,
			FetchDocument:       1,
			CheckDocumentExists: 1,
			CheckDocumentCycle:  1,
			Rollback:            1,
		},
		"Error returned by Tx.UpdateDocumentParentID": {
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
			Body:                                newParentBody,
			RespCode:                            http.StatusInternalServerError,
			BeginTx:                             1,
			FetchDocument:                       1,
			CheckDocumentExists:                 1,
			CheckDocumentCycle:                  1,
			FetchDocumentTreeByDocumentParentID: 2,
			UpdateDocumentParentID:              1,
			Rollback:                            1,
		},
		"Response carries the swapped ordering": {
			Tx: &TxMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return documentCore.Summaries{{ID: xid.New()}, {ID: _documentID}}, nil
				},
			},
			Body:                                sameParentBody,
			RespCode:                            http.StatusOK,
			Committed:                           1,
			TreeCbs:                             1,
			BeginTx:                             1,
			FetchDocument:                       1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
			Check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				// the persisted ordering is the swapped one, so the client
				// must not be handed back the ordering it asked to change.
				var got documentCore.Summaries

				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				require.Len(t, got, 2)
				assert.Equal(t, _documentID, got[0].ID)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{BeginTxFunc: testutil.StubBeginTx(c.Tx, c.BeginErr)}
			hdl, cnt := newTestHandler(db, &fakePublisher{})

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentTree(rec, newRequest(http.MethodPut, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, db.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.FetchDocumentCalls(), c.FetchDocument)
			assert.Len(t, c.Tx.CheckDocumentExistsCalls(), c.CheckDocumentExists)
			assert.Len(t, c.Tx.CheckDocumentCycleCalls(), c.CheckDocumentCycle)
			assert.Len(t, c.Tx.FetchDocumentTreeByDocumentParentIDCalls(), c.FetchDocumentTreeByDocumentParentID)
			assert.Len(t, c.Tx.UpdateDocumentTreeCalls(), c.UpdateDocumentTree)
			assert.Len(t, c.Tx.UpdateDocumentParentIDCalls(), c.UpdateDocumentParentID)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.Check != nil {
				c.Check(t, rec)
			}
		})
	}
}

func Test_Handler_SearchDocuments(t *testing.T) {
	updatedAt := time.Date(2026, 10, 4, 9, 12, 41, 0, time.UTC)

	doc := documentCore.Document{
		ID:             _documentID,
		OrganizationID: "org1",
		BranchID:       _branchID,
		BranchName:     documentCore.DefaultBranch,
		DocumentName:   "Shipments",
		Icon:           "truck",
		Default:        true,
		UpdatedAt:      updatedAt,
		LastUpdatedBy:  null.StringFrom("u2"),
	}

	docJSON := `{
		"id": "` + _documentID.String() + `",
		"title": "Shipments",
		"titleHtml": null,
		"icon": "truck",
		"branch": {"id": "` + _branchID.String() + `", "name": "main", "default": true},
		"updatedAt": "2026-10-04T09:12:41Z",
		"updatedBy": "u2"
	}`

	chart := search.Block{
		ID:       _branchID.String() + "-m1",
		BranchID: _branchID,
		Type:     "metricBlock",
		Text:     "<mark>Shipments</mark> per minute",
		Attrs: map[string]string{
			"dataSourceId":      _dataSourceID.String(),
			"visualizationType": "timeseries",
		},
	}

	// stubSearcher answers every search with the page or the error.
	stubSearcher := func(page search.GroupPage, err error) *SearcherMock {
		return &SearcherMock{
			SearchGroupsFunc: func(context.Context, search.GroupQuery) (search.GroupPage, error) {
				return page, err
			},
		}
	}

	paragraphPage := search.GroupPage{
		Groups: []search.Group{
			{
				DocumentID: _documentID,
				BranchID:   _branchID,
				Hits: []search.Block{
					{
						ID:       _branchID.String() + "-p1",
						BranchID: _branchID,
						Type:     "paragraph",
						Text:     "<mark>shipment</mark> events",
					},
				},
				TotalHits:     4,
				NextHitsToken: null.StringFrom("more"),
			},
		},
		TotalHits:      1,
		TotalDocuments: 1,
		NextPageToken:  null.StringFrom("next"),
	}

	chartPage := search.GroupPage{
		Groups: []search.Group{
			{
				DocumentID: _documentID,
				BranchID:   _branchID,
				Hits:       []search.Block{chart},
				TotalHits:  1,
			},
		},
		TotalHits:      1,
		TotalDocuments: 1,
	}

	cc := map[string]struct {
		DB                        *DBMock
		Searcher                  *SearcherMock
		NoSession                 bool
		Query                     string
		RespCode                  int
		RespJSON                  string
		SearchGroups              int
		FetchDocumentsByBranchIDs int
		Check                     func(*testing.T, *DBMock, *SearcherMock)
	}{
		"No session in context": {
			DB:        &DBMock{},
			Searcher:  &SearcherMock{},
			NoSession: true,
			Query:     "?q=shipment",
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid limit": {
			DB:       &DBMock{},
			Searcher: &SearcherMock{},
			Query:    "?q=shipment&limit=abc",
			RespCode: http.StatusBadRequest,
		},
		"Invalid current document id": {
			DB:       &DBMock{},
			Searcher: &SearcherMock{},
			Query:    "?q=shipment&currentDocId=nope",
			RespCode: http.StatusBadRequest,
		},
		"Empty current document id": {
			DB:       &DBMock{},
			Searcher: &SearcherMock{},
			Query:    "?q=shipment&currentDocId=",
			RespCode: http.StatusBadRequest,
		},
		"Short query": {
			DB:           &DBMock{},
			Searcher:     stubSearcher(search.GroupPage{}, search.ErrInvalidQuery),
			Query:        "?q=a",
			RespCode:     http.StatusBadRequest,
			SearchGroups: 1,
		},
		"Error returned by Searcher.SearchGroups": {
			DB:           &DBMock{},
			Searcher:     stubSearcher(search.GroupPage{}, assert.AnError),
			Query:        "?q=shipment",
			RespCode:     http.StatusInternalServerError,
			SearchGroups: 1,
		},
		"Invalid page token": {
			DB:           &DBMock{},
			Searcher:     stubSearcher(search.GroupPage{}, search.ErrInvalidPageToken),
			Query:        "?q=shipment&nextToken=nope",
			RespCode:     http.StatusBadRequest,
			SearchGroups: 1,
		},
		"Error returned by DB.FetchDocumentsByBranchIDs": {
			DB: &DBMock{
				FetchDocumentsByBranchIDsFunc: func(context.Context, []xid.ID, string) ([]documentCore.Document, error) {
					return nil, assert.AnError
				},
			},
			Searcher:                  stubSearcher(paragraphPage, nil),
			Query:                     "?q=shipment",
			RespCode:                  http.StatusInternalServerError,
			SearchGroups:              1,
			FetchDocumentsByBranchIDs: 1,
		},
		"Successful search": {
			DB: &DBMock{
				FetchDocumentsByBranchIDsFunc: func(context.Context, []xid.ID, string) ([]documentCore.Document, error) {
					return []documentCore.Document{doc}, nil
				},
			},
			Searcher: stubSearcher(paragraphPage, nil),
			Query:    "?q=shipment&limit=500&hitsLimit=5&currentDocId=" + _documentID.String() + "&nextToken=token",
			RespCode: http.StatusOK,
			RespJSON: `{
				"total": {"hits": 1, "documents": 1, "capped": false},
				"nextToken": "next",
				"results": [{
					"document": ` + docJSON + `,
					"hits": [{"id": "p1", "type": "paragraph", "text": "<mark>shipment</mark> events"}],
					"totalHits": 4,
					"nextHitsToken": "more"
				}]
			}`,
			SearchGroups:              1,
			FetchDocumentsByBranchIDs: 1,
			Check: func(t *testing.T, db *DBMock, searcher *SearcherMock) {
				t.Helper()

				sf := searcher.SearchGroupsCalls()
				require.Len(t, sf, 1)
				assert.Equal(t, search.GroupQuery{
					OrganizationID:    "org1",
					Query:             "shipment",
					CurrentDocumentID: null.ValueFrom(_documentID),
					Limit:             search.GroupLimitMax,
					HitsLimit:         5,
					PageToken:         "token",
				}, sf[0].Gq)

				df := db.FetchDocumentsByBranchIDsCalls()
				require.Len(t, df, 1)
				assert.Equal(t, []xid.ID{_branchID}, df[0].BranchIDs)
				assert.Equal(t, "org1", df[0].OrganizationID)
			},
		},
		"Successful search with charts": {
			DB: &DBMock{
				FetchDocumentsByBranchIDsFunc: func(context.Context, []xid.ID, string) ([]documentCore.Document, error) {
					return []documentCore.Document{doc}, nil
				},
			},
			Searcher: stubSearcher(chartPage, nil),
			Query:    "?q=shipment",
			RespCode: http.StatusOK,
			RespJSON: `{
				"total": {"hits": 1, "documents": 1, "capped": false},
				"nextToken": null,
				"results": [{
					"document": ` + docJSON + `,
					"hits": [{
						"id": "m1",
						"type": "metricBlock",
						"text": "<mark>Shipments</mark> per minute",
						"attrs": {"dataSourceId": "` + _dataSourceID.String() + `", "visualizationType": "timeseries"}
					}],
					"totalHits": 1,
					"nextHitsToken": null
				}]
			}`,
			SearchGroups:              1,
			FetchDocumentsByBranchIDs: 1,
			Check: func(t *testing.T, _ *DBMock, searcher *SearcherMock) {
				t.Helper()

				sf := searcher.SearchGroupsCalls()
				require.Len(t, sf, 1)
				assert.False(t, sf[0].Gq.CurrentDocumentID.Valid)
				assert.Equal(t, search.GroupLimitDefault, sf[0].Gq.Limit)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.searcher = c.Searcher

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
			assert.Len(t, c.Searcher.SearchGroupsCalls(), c.SearchGroups)
			assert.Len(t, c.DB.FetchDocumentsByBranchIDsCalls(), c.FetchDocumentsByBranchIDs)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			if c.Check != nil {
				c.Check(t, c.DB, c.Searcher)
			}
		})
	}
}

func Test_Handler_FetchRecentlyViewedDocuments(t *testing.T) {
	doc := search.RecentDocument{
		ID:             _documentID,
		OrganizationID: "org1",
		BranchID:       _branchID,
		BranchName:     documentCore.DefaultBranch,
		DocumentName:   "Shipments",
		Icon:           "truck",
		Default:        true,
		UpdatedAt:      time.Date(2026, 10, 4, 9, 12, 41, 0, time.UTC),
		LastUpdatedBy:  null.StringFrom("u2"),
		ViewedAt:       time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC),
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Query     string
		RespCode  int
		RespJSON  string
		From      search.ViewFrom
		Limit     int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid limit": {
			DB:       &DBMock{},
			Query:    "?limit=abc",
			RespCode: http.StatusBadRequest,
		},
		"Invalid origin": {
			DB:       &DBMock{},
			Query:    "?from=tree",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchRecentlyViewedDocuments": {
			DB: &DBMock{
				FetchRecentlyViewedDocumentsFunc: func(context.Context, string, string, search.ViewFrom, int) ([]search.RecentDocument, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			From:     search.ViewFromAll,
			Limit:    search.GroupLimitDefault,
		},
		"Successful listing": {
			DB: &DBMock{
				FetchRecentlyViewedDocumentsFunc: func(context.Context, string, string, search.ViewFrom, int) ([]search.RecentDocument, error) {
					return []search.RecentDocument{doc}, nil
				},
			},
			Query:    "?from=search&limit=5",
			RespCode: http.StatusOK,
			RespJSON: `{
				"results": [{
					"id": "` + _documentID.String() + `",
					"title": "Shipments",
					"titleHtml": null,
					"icon": "truck",
					"branch": {"id": "` + _branchID.String() + `", "name": "main", "default": true},
					"updatedAt": "2026-10-04T09:12:41Z",
					"updatedBy": "u2",
					"viewedAt": "2026-10-07T08:00:00Z"
				}]
			}`,
			From:  search.ViewFromSearch,
			Limit: 5,
		},
		"Successful listing capped at the maximum": {
			DB: &DBMock{
				FetchRecentlyViewedDocumentsFunc: func(context.Context, string, string, search.ViewFrom, int) ([]search.RecentDocument, error) {
					return nil, nil
				},
			},
			Query:    "?limit=500",
			RespCode: http.StatusOK,
			RespJSON: `{"results": []}`,
			From:     search.ViewFromAll,
			Limit:    search.GroupLimitMax,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchRecentlyViewedDocuments(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			ff := c.DB.FetchRecentlyViewedDocumentsCalls()

			if c.Limit == 0 {
				assert.Empty(t, ff)
				return
			}

			require.Len(t, ff, 1)
			assert.Equal(t, "u1", ff[0].UserID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, c.From, ff[0].From)
			assert.Equal(t, c.Limit, ff[0].Limit)
		})
	}
}

func Test_Handler_SearchDocumentsLegacy(t *testing.T) {
	cc := map[string]struct {
		Searcher        *SearcherMock
		NoSession       bool
		Query           string
		RespCode        int
		RespBody        string
		SearchDocuments int
	}{
		"No session in context": {
			Searcher:  &SearcherMock{},
			NoSession: true,
			Query:     "?q=test",
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid search query": {
			Searcher: &SearcherMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return nil, search.ErrInvalidQuery
				},
			},
			RespCode:        http.StatusBadRequest,
			SearchDocuments: 1,
		},
		"Error returned by Searcher.SearchDocuments": {
			Searcher: &SearcherMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return nil, errors.New("boom")
				},
			},
			Query:           "?q=test",
			RespCode:        http.StatusInternalServerError,
			SearchDocuments: 1,
		},
		"Successful search": {
			Searcher: &SearcherMock{
				SearchDocumentsFunc: func(context.Context, string, string) ([]byte, error) {
					return []byte(`{"hits":[]}`), nil
				},
			},
			Query:           "?q=test",
			RespCode:        http.StatusOK,
			RespBody:        `{"hits":[]}`,
			SearchDocuments: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(&DBMock{}, &fakePublisher{})
			hdl.searcher = c.Searcher

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.SearchDocumentsLegacy(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.Searcher.SearchDocumentsCalls(), c.SearchDocuments)

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

	cc := map[string]struct {
		Tx                                  *TxMock
		Queue                               *SearchQueueMock
		BeginErr                            error
		NoSession                           bool
		Body                                string
		RespCode                            int
		Committed                           int
		TreeCbs                             int
		BeginTx                             int
		CheckDocumentExists                 int
		InsertDocument                      int
		RecordDocumentBranchHistoryEntry    int
		UpsertDocumentMaintainers           int
		Enqueue                             int
		FetchDocumentTreeByDocumentParentID int
		UpdateDocumentTree                  int
		Rollback                            int
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
		"Error returned by DB.BeginTx": {
			Tx:       &TxMock{},
			BeginErr: errors.New("boom"),
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			BeginTx:  1,
		},
		"Parent document does not exist": {
			Tx: &TxMock{
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			Body:                parentBody,
			RespCode:            http.StatusNotFound,
			BeginTx:             1,
			CheckDocumentExists: 1,
			Rollback:            1,
		},
		"Error returned by Tx.InsertDocument": {
			Tx: &TxMock{
				InsertDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			Body:           validBody,
			RespCode:       http.StatusInternalServerError,
			BeginTx:        1,
			InsertDocument: 1,
			Rollback:       1,
		},
		"Error returned by Tx.RecordDocumentBranchHistoryEntry": {
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			BeginTx:                          1,
			InsertDocument:                   1,
			RecordDocumentBranchHistoryEntry: 1,
			Rollback:                         1,
		},
		"Error returned by Tx.UpsertDocumentMaintainers": {
			Tx: &TxMock{
				UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			BeginTx:                          1,
			InsertDocument:                   1,
			RecordDocumentBranchHistoryEntry: 1,
			UpsertDocumentMaintainers:        1,
			Rollback:                         1,
		},
		"Error returned by SearchQueue.Enqueue": {
			Tx: &TxMock{},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			Body:                             validBody,
			RespCode:                         http.StatusInternalServerError,
			BeginTx:                          1,
			InsertDocument:                   1,
			RecordDocumentBranchHistoryEntry: 1,
			UpsertDocumentMaintainers:        1,
			Enqueue:                          1,
			Rollback:                         1,
		},
		"Error returned by Tx.FetchDocumentTreeByDocumentParentID": {
			Tx: &TxMock{
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (documentCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			Body:                                validBody,
			RespCode:                            http.StatusInternalServerError,
			BeginTx:                             1,
			InsertDocument:                      1,
			RecordDocumentBranchHistoryEntry:    1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			Rollback:                            1,
		},
		"Sort index swap error": {
			Tx:                                  &TxMock{},
			Body:                                validBody,
			RespCode:                            http.StatusBadRequest,
			BeginTx:                             1,
			InsertDocument:                      1,
			RecordDocumentBranchHistoryEntry:    1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			Rollback:                            1,
		},
		"Error returned by Tx.UpdateDocumentTree": {
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.UpdateDocumentTreeFunc = func(context.Context, documentCore.Summaries, string) error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:                                validBody,
			RespCode:                            http.StatusInternalServerError,
			BeginTx:                             1,
			InsertDocument:                      1,
			RecordDocumentBranchHistoryEntry:    1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
		"Error returned by Tx.Commit": {
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.CommitFunc = func() error {
					return errors.New("boom")
				}

				return tx
			}(),
			Body:                                validBody,
			RespCode:                            http.StatusInternalServerError,
			Committed:                           1,
			BeginTx:                             1,
			InsertDocument:                      1,
			RecordDocumentBranchHistoryEntry:    1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
		"Successful creation": {
			Tx:                                  insertAwareTx(),
			Body:                                validBody,
			RespCode:                            http.StatusCreated,
			Committed:                           1,
			TreeCbs:                             1,
			BeginTx:                             1,
			InsertDocument:                      1,
			RecordDocumentBranchHistoryEntry:    1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			queue := c.Queue
			if queue == nil {
				queue = &SearchQueueMock{}
			}

			db := &DBMock{BeginTxFunc: testutil.StubBeginTx(c.Tx, c.BeginErr)}
			hdl, cnt := newTestHandler(db, &fakePublisher{})
			hdl.searchQueue = queue

			rec := httptest.NewRecorder()

			hdl.CreateDocument(rec, newRequest(http.MethodPost, c.Body, c.NoSession, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, db.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.CheckDocumentExistsCalls(), c.CheckDocumentExists)
			assert.Len(t, c.Tx.InsertDocumentCalls(), c.InsertDocument)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.RecordDocumentBranchHistoryEntry)
			assert.Len(t, c.Tx.UpsertDocumentMaintainersCalls(), c.UpsertDocumentMaintainers)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, c.Tx.FetchDocumentTreeByDocumentParentIDCalls(), c.FetchDocumentTreeByDocumentParentID)
			assert.Len(t, c.Tx.UpdateDocumentTreeCalls(), c.UpdateDocumentTree)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.RespCode == http.StatusCreated {
				assert.Contains(t, rec.Body.String(), `"New doc"`)
				assert.Equal(t, []string{"u1"}, c.Tx.UpsertDocumentMaintainersCalls()[0].MaintainerIDs)

				// the first content is a boundary entry, so a later edit
				// can be rolled back to it.
				require.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), 1)
				entry := c.Tx.RecordDocumentBranchHistoryEntryCalls()[0]
				assert.Equal(t, c.Tx.InsertDocumentCalls()[0].Doc.BranchID, entry.BranchID)
				assert.Equal(t, "org1", entry.OrganizationID)
				assert.True(t, entry.Boundary)
				assert.Equal(t, null.StringFrom("u1"), entry.By)

				// a new document has no source branch to copy hooks from.
				assert.Empty(t, c.Tx.FetchDocumentHooksByBranchIDCalls())
				assert.Empty(t, hdl.hookMan.(*HookManagerMock).SetUpBranchCalls())
			}
		})
	}
}

func Test_Handler_DeleteDocument(t *testing.T) {
	cc := map[string]struct {
		DB             *DBMock
		Tx             *TxMock
		Queue          *SearchQueueMock
		BeginErr       error
		NoSession      bool
		OmitDoc        bool
		RespCode       int
		Committed      int
		TreeCbs        int
		FetchDocument  int
		BeginTx        int
		DeleteDocument int
		Enqueue        int
		Rollback       int
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
		"Error returned by DB.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:            &TxMock{},
			RespCode:      http.StatusInternalServerError,
			FetchDocument: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:            &TxMock{},
			BeginErr:      errors.New("boom"),
			RespCode:      http.StatusInternalServerError,
			FetchDocument: 1,
			BeginTx:       1,
		},
		"Error returned by Tx.DeleteDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				DeleteDocumentFunc: func(context.Context, xid.ID, string) ([]xid.ID, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode:       http.StatusInternalServerError,
			FetchDocument:  1,
			BeginTx:        1,
			DeleteDocument: 1,
			Rollback:       1,
		},
		"Error returned by SearchQueue.Enqueue": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				DeleteDocumentFunc: func(context.Context, xid.ID, string) ([]xid.ID, error) {
					return []xid.ID{_documentID}, nil
				},
			},
			Queue: &SearchQueueMock{
				EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
					return errors.New("boom")
				},
			},
			RespCode:       http.StatusInternalServerError,
			FetchDocument:  1,
			BeginTx:        1,
			DeleteDocument: 1,
			Enqueue:        1,
			Rollback:       1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				CommitFunc: func() error {
					return errors.New("boom")
				},
			},
			RespCode:       http.StatusInternalServerError,
			Committed:      1,
			FetchDocument:  1,
			BeginTx:        1,
			DeleteDocument: 1,
			Rollback:       1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				DeleteDocumentFunc: func(_ context.Context, id xid.ID, _ string) ([]xid.ID, error) {
					return []xid.ID{id}, nil
				},
			},
			RespCode:       http.StatusOK,
			Committed:      1,
			TreeCbs:        1,
			FetchDocument:  1,
			BeginTx:        1,
			DeleteDocument: 1,
			Enqueue:        1,
			Rollback:       1,
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

			hdl.DeleteDocument(rec, newRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCalls(), c.FetchDocument)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.DeleteDocumentCalls(), c.DeleteDocument)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			if c.RespCode == http.StatusOK {
				require.Len(t, c.Tx.DeleteDocumentCalls(), 1)
				assert.Equal(t, _documentID, c.Tx.DeleteDocumentCalls()[0].ID)

				// the resync of the returned subtree ids rides the same
				// transaction as the delete.
				qq := queue.EnqueueCalls()
				require.Len(t, qq, 1)
				assert.Same(t, c.Tx, qq[0].Tx)
				assert.Equal(t, search.DocumentScope("org1", _documentID), qq[0].J)
			}
		})
	}
}

func Test_Handler_DuplicateDocument(t *testing.T) {
	// imageDB serves a document whose content holds an image served by that
	// same document, plus the file row that image refers to. Duplication
	// copies the file from that row.
	imageDB := func() *DBMock {
		return &DBMock{
			FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
				doc := storedDoc()
				doc.Content = documentCore.RootBlock{
					Type: documentCore.BlockNodeDoc,
					Content: []documentCore.Block{
						{
							Type: documentCore.BlockNodeImageBlock,
							Attrs: documentCore.Attributes{
								"uid": "img-1-aaaaaaaaaaaaaaa",
								"src": "https://app.test/core" + documentCore.FilePath(_documentID, "img-1-aaaaaaaaaaaaaaa", "shot.png"),
							},
						},
					},
				}

				return doc, nil
			},
			FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
				f := file.NewFile(id, file.LocationDocument, "key", _documentID, organizationID, "shot.png", 1024, "image/png")

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
		// CopiedUID is the source block uid the case's hook is anchored
		// to.
		CopiedUID                           string
		FetchDocument                       int
		BeginTx                             int
		InsertDocument                      int
		FetchDocumentHooksByBranchID        int
		InsertDocumentHook                  int
		RecordDocumentBranchHistoryEntry    int
		CopyBranchTags                      int
		UpsertDocumentMaintainers           int
		Enqueue                             int
		FetchDocumentTreeByDocumentParentID int
		UpdateDocumentTree                  int
		Rollback                            int
		SetUpBranch                         int
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
		"Error returned by DB.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Tx:            &TxMock{},
			RespCode:      http.StatusInternalServerError,
			FetchDocument: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:            &TxMock{},
			BeginErr:      errors.New("boom"),
			RespCode:      http.StatusInternalServerError,
			FetchDocument: 1,
			BeginTx:       1,
		},
		"Error returned by Tx.InsertDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				InsertDocumentFunc: func(context.Context, documentCore.Document) error {
					return errors.New("boom")
				},
			},
			RespCode:       http.StatusInternalServerError,
			FetchDocument:  1,
			BeginTx:        1,
			InsertDocument: 1,
			Rollback:       1,
		},
		"Error returned by Tx.CopyBranchTags": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: &TxMock{
				CopyBranchTagsFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			RespCode:                         http.StatusInternalServerError,
			FetchDocument:                    1,
			BeginTx:                          1,
			InsertDocument:                   1,
			FetchDocumentHooksByBranchID:     1,
			RecordDocumentBranchHistoryEntry: 1,
			CopyBranchTags:                   1,
			Rollback:                         1,
		},
		"Sort index swap error": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                                  &TxMock{},
			RespCode:                            http.StatusBadRequest,
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			Rollback:                            1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.CommitFunc = func() error {
					return errors.New("boom")
				}

				return tx
			}(),
			RespCode:                            http.StatusInternalServerError,
			Committed:                           1,
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
		},
		"Successful duplication": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx:                                  insertAwareTx(),
			RespCode:                            http.StatusCreated,
			Committed:                           1,
			TreeCbs:                             1,
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
			SetUpBranch:                         1,
		},
		"Files are copied along with the document": {
			DB:                                  imageDB(),
			Tx:                                  insertAwareTx(),
			Storer:                              &StorerMock{},
			RespCode:                            http.StatusCreated,
			Committed:                           1,
			TreeCbs:                             1,
			Copies:                              1,
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
			SetUpBranch:                         1,
		},
		"Failing file copy still yields the duplicate": {
			DB: imageDB(),
			Tx: insertAwareTx(),
			Storer: &StorerMock{
				CopyFunc: func(context.Context, string, string, string, string) error {
					return errors.New("boom")
				},
			},
			RespCode:                            http.StatusCreated,
			Committed:                           1,
			TreeCbs:                             1,
			Copies:                              1,
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
			SetUpBranch:                         1,
		},
		"Hooks follow the regenerated block uids": {
			DB: imageDB(),
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{{BlockID: null.StringFrom("img-1-aaaaaaaaaaaaaaa")}}, nil
				}

				return tx
			}(),
			Storer:                              &StorerMock{},
			RespCode:                            http.StatusCreated,
			Committed:                           1,
			TreeCbs:                             1,
			Copies:                              1,
			CopiedUID:                           "img-1-aaaaaaaaaaaaaaa",
			FetchDocument:                       1,
			BeginTx:                             1,
			InsertDocument:                      1,
			FetchDocumentHooksByBranchID:        1,
			InsertDocumentHook:                  1,
			RecordDocumentBranchHistoryEntry:    1,
			CopyBranchTags:                      1,
			UpsertDocumentMaintainers:           1,
			Enqueue:                             1,
			FetchDocumentTreeByDocumentParentID: 1,
			UpdateDocumentTree:                  1,
			Rollback:                            1,
			SetUpBranch:                         1,
		},
		"Error returned by Tx.FetchDocumentHooksByBranchID": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*documentCore.Document, error) {
					return storedDoc(), nil
				},
			},
			Tx: func() *TxMock {
				tx := insertAwareTx()
				tx.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				}

				return tx
			}(),
			RespCode:                     http.StatusInternalServerError,
			FetchDocument:                1,
			BeginTx:                      1,
			InsertDocument:               1,
			FetchDocumentHooksByBranchID: 1,
			Rollback:                     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			storer := c.Storer
			if storer == nil {
				storer = &StorerMock{}
			}

			c.DB.BeginTxFunc = testutil.StubBeginTx(c.Tx, c.BeginErr)

			hdl, cnt := newTestHandler(c.DB, &fakePublisher{})
			hdl.storer = storer

			queue := &SearchQueueMock{}
			hdl.searchQueue = queue

			man := &HookManagerMock{}
			hdl.hookMan = man

			rec := httptest.NewRecorder()

			hdl.DuplicateDocument(rec, newRequest(http.MethodPost, "", c.NoSession, c.OmitDoc, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchDocumentCalls(), c.FetchDocument)
			assert.Len(t, c.DB.BeginTxCalls(), c.BeginTx)
			assert.Len(t, c.Tx.InsertDocumentCalls(), c.InsertDocument)
			assert.Len(t, c.Tx.FetchDocumentHooksByBranchIDCalls(), c.FetchDocumentHooksByBranchID)
			assert.Len(t, c.Tx.InsertDocumentHookCalls(), c.InsertDocumentHook)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.RecordDocumentBranchHistoryEntry)
			assert.Len(t, c.Tx.CopyBranchTagsCalls(), c.CopyBranchTags)
			assert.Len(t, c.Tx.UpsertDocumentMaintainersCalls(), c.UpsertDocumentMaintainers)
			assert.Len(t, queue.EnqueueCalls(), c.Enqueue)
			assert.Len(t, c.Tx.FetchDocumentTreeByDocumentParentIDCalls(), c.FetchDocumentTreeByDocumentParentID)
			assert.Len(t, c.Tx.UpdateDocumentTreeCalls(), c.UpdateDocumentTree)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollback)
			assert.Len(t, man.SetUpBranchCalls(), c.SetUpBranch)
			assert.Equal(t, c.TreeCbs, cnt.tree)

			// the source document has no parent to check.
			assert.Empty(t, c.Tx.CheckDocumentExistsCalls())

			assert.Len(t, c.DB.FetchDocumentFileCalls(), c.Copies)
			assert.Len(t, c.DB.InsertDocumentFileCalls(), c.Copies)
			assert.Len(t, storer.CopyCalls(), c.Copies)

			if c.RespCode == http.StatusCreated {
				// the duplicate is a new document owned by the caller.
				require.Len(t, c.Tx.InsertDocumentCalls(), 1)
				dupl := c.Tx.InsertDocumentCalls()[0].Doc
				assert.NotEqual(t, _documentID, dupl.ID)
				assert.Equal(t, []string{"u1"}, c.Tx.UpsertDocumentMaintainersCalls()[0].MaintainerIDs)

				// the copy's default branch takes the source's tags before
				// the commit.
				require.Len(t, c.Tx.CopyBranchTagsCalls(), 1)
				assert.Equal(t, _branchID, c.Tx.CopyBranchTagsCalls()[0].FromBranchID)
				assert.Equal(t, dupl.BranchID, c.Tx.CopyBranchTagsCalls()[0].ToBranchID)

				// the hooks are copied inside the transaction, re-anchored
				// to the duplicate's regenerated block uids, and set up once
				// it commits.
				hh := c.Tx.FetchDocumentHooksByBranchIDCalls()
				require.Len(t, hh, 1)
				assert.Equal(t, _branchID, hh[0].BranchID)
				assert.Equal(t, "org1", hh[0].OrganizationID)

				pp := man.SetUpBranchCalls()
				require.Len(t, pp, 1)
				assert.Equal(t, dupl.BranchID, pp[0].BranchID)

				if c.CopiedUID != "" {
					ii := c.Tx.InsertDocumentHookCalls()
					require.Len(t, ii, 1)
					assert.Equal(t, null.ValueFrom(dupl.ID), ii[0].Hk.DocumentID)
					assert.True(t, ii[0].Hk.BlockID.Valid)
					assert.NotEqual(t, c.CopiedUID, ii[0].Hk.BlockID.String)
				}

				// the duplicate starts with a boundary entry.
				require.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), 1)
				entry := c.Tx.RecordDocumentBranchHistoryEntryCalls()[0]
				assert.Equal(t, dupl.BranchID, entry.BranchID)
				assert.True(t, entry.Boundary)
			}
		})
	}
}

func Test_Handler_copyDocumentFiles(t *testing.T) {
	toDocumentID := xid.New()

	fileDB := func() *DBMock {
		return &DBMock{
			FetchDocumentFileFunc: func(_ context.Context, id, organizationID string) (*file.File, error) {
				f := file.NewFile(id, file.LocationComment, file.Key(organizationID, _documentID, id), _documentID, organizationID, "shot.png", 1024, "image/png")

				return &f, nil
			},
		}
	}

	cc := map[string]struct {
		DB                *DBMock
		Storer            *StorerMock
		Files             map[string]string
		Inserts           int
		Copies            int
		FetchDocumentFile int
	}{
		"Nothing to copy": {
			DB:     fileDB(),
			Storer: &StorerMock{},
			Files:  map[string]string{},
		},
		"Error returned by DB.FetchDocumentFile": {
			DB: &DBMock{
				FetchDocumentFileFunc: func(context.Context, string, string) (*file.File, error) {
					return nil, errors.New("boom")
				},
			},
			Storer:            &StorerMock{},
			Files:             map[string]string{"old-1": "new-1"},
			FetchDocumentFile: 1,
		},
		"Error returned by DB.InsertDocumentFile": {
			DB: func() *DBMock {
				db := fileDB()
				db.InsertDocumentFileFunc = func(context.Context, file.File) error {
					return errors.New("boom")
				}

				return db
			}(),
			Storer:            &StorerMock{},
			Files:             map[string]string{"old-1": "new-1"},
			Inserts:           1,
			FetchDocumentFile: 1,
		},
		"Error returned by Storer.Copy": {
			DB: fileDB(),
			Storer: &StorerMock{
				CopyFunc: func(context.Context, string, string, string, string) error {
					return errors.New("boom")
				},
			},
			Files:             map[string]string{"old-1": "new-1"},
			Inserts:           1,
			Copies:            1,
			FetchDocumentFile: 1,
		},
		"Successful copy": {
			DB:                fileDB(),
			Storer:            &StorerMock{},
			Files:             map[string]string{"old-1": "new-1"},
			Inserts:           1,
			Copies:            1,
			FetchDocumentFile: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, _ := newTestHandler(c.DB, &fakePublisher{})
			hdl.storer = c.Storer

			hdl.copyDocumentFiles(context.Background(), c.Files, toDocumentID, "org1")

			assert.Len(t, c.DB.FetchDocumentFileCalls(), c.FetchDocumentFile)

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

			// the copy goes from the source row's key to the new row's, so
			// both carry the extension.
			assert.Equal(t, file.Folder("org1", _documentID)+"/", cf[0].SrcFolder)
			assert.Equal(t, "old-1", cf[0].SrcID)
			assert.Equal(t, file.Folder("org1", toDocumentID)+"/", cf[0].DstFolder)
			assert.Equal(t, "new-1", cf[0].DstID)
		})
	}
}
