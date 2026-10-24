package tag

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	tagCore "github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

var (
	// _tagID is the tag the path-addressed tests operate on.
	_tagID = xid.New()

	// _documentID is the document the assignment tests operate on.
	_documentID = xid.New()

	// _branchID is the branch of that document the assignment tests operate
	// on.
	_branchID = xid.New()
)

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
}

func Test_Handler_FetchTagTree(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Code      int
		Body      string
		Fetches   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Code:      http.StatusUnauthorized,
			Body:      `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Error returned by DB.FetchTagTree": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Fetches: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tagCore.Summaries{
						{
							ID:      _tagID,
							TagName: "Production",
							Color:   "green",
							Documents: documentCore.Summaries{
								{ID: _documentID, DocumentName: "Runbook", Icon: "lucide:file"},
							},
						},
					}, nil
				},
			},
			Code:    http.StatusOK,
			Fetches: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()
			hdl.FetchTagTree(rec, req)

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.FetchTagTreeCalls(), c.Fetches)

			if c.Fetches > 0 {
				assert.Equal(t, "org1", c.DB.FetchTagTreeCalls()[0].OrganizationID)
				assert.Equal(t, "u1", c.DB.FetchTagTreeCalls()[0].UserID)
			}

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			if c.Code == http.StatusOK {
				assert.Contains(t, rec.Body.String(), `"tagName":"Production"`)
				assert.Contains(t, rec.Body.String(), `"documentName":"Runbook"`)
			}
		})
	}
}

func Test_Handler_UpdateTagTree(t *testing.T) {
	t.Parallel()

	tree := tagCore.Summaries{
		{
			ID:      _tagID,
			TagName: "Production",
			Color:   "green",
			Documents: documentCore.Summaries{
				{ID: _documentID, DocumentName: "Runbook", Icon: "lucide:file"},
			},
		},
		{
			ID:      xid.New(),
			TagName: "Staging",
			Color:   "green",
			Documents: documentCore.Summaries{
				{ID: _documentID, DocumentName: "Runbook", Icon: "lucide:file"},
			},
		},
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Payload   string
		Code      int
		Body      string
		Fetches   int
		Updates   int
		Notifies  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Payload:   `{}`,
			Code:      http.StatusUnauthorized,
			Body:      `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed payload": {
			DB:      &DBMock{},
			Payload: `{`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Error returned by DB.FetchTagTree": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":0}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Fetches: 1,
		},
		"Sort index out of range": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree, nil
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":9}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag_summary.invalid_sort_index","message":"sort index is out of range"}`,
			Fetches: 1,
		},
		"Unknown tag": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree, nil
				},
			},
			Payload: `{"id":"` + xid.New().String() + `","sortIndex":0}`,
			Code:    http.StatusNotFound,
			Body:    `{"code":"general","message":"not found"}`,
			Fetches: 1,
		},
		"Error returned by DB.UpdateTagTree": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree, nil
				},
				UpdateTagTreeFunc: func(context.Context, tagCore.Summaries, string) error {
					return assert.AnError
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":1}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Fetches: 1,
			Updates: 1,
		},
		"Successful swap": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree, nil
				},
			},
			Payload:  `{"id":"` + _tagID.String() + `","sortIndex":1}`,
			Code:     http.StatusOK,
			Fetches:  1,
			Updates:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Payload))

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()
			hdl.UpdateTagTree(rec, req)

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.FetchTagTreeCalls(), c.Fetches)
			assert.Len(t, c.DB.UpdateTagTreeCalls(), c.Updates)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			if c.Code == http.StatusOK {
				persisted := c.DB.UpdateTagTreeCalls()[0].Tree
				require.Len(t, persisted, 2)
				assert.Equal(t, _tagID, persisted[1].ID)
				assert.Contains(t, rec.Body.String(), `"tagName":"Staging"`)
			}
		})
	}
}

func Test_Handler_CreateTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Payload   string
		Code      int
		Body      string
		Inserts   int
		Notifies  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Payload:   `{}`,
			Code:      http.StatusUnauthorized,
			Body:      `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed payload": {
			DB:      &DBMock{},
			Payload: `{`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Missing name": {
			DB:      &DBMock{},
			Payload: `{"tagName":"","color":"green"}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag.invalid_name","message":"tag name cannot be empty"}`,
		},
		"Off palette colour": {
			DB:      &DBMock{},
			Payload: `{"tagName":"Production","color":"navy"}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag.invalid_color","message":"tag colour must be one of the palette"}`,
		},
		"Hex instead of a name": {
			DB:      &DBMock{},
			Payload: `{"tagName":"Production","color":"#00a63e"}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag.invalid_color","message":"tag colour must be one of the palette"}`,
		},
		"Error returned by DB.InsertTag": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tagCore.Tag) error {
					return assert.AnError
				},
			},
			Payload: `{"tagName":"Production","color":"green"}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Inserts: 1,
		},
		"Successful creation": {
			DB:       &DBMock{},
			Payload:  `{"tagName":"Production","color":"green"}`,
			Code:     http.StatusCreated,
			Inserts:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Payload))

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()
			hdl.CreateTag(rec, req)

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.InsertTagCalls(), c.Inserts)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			if c.Code == http.StatusCreated {
				stored := c.DB.InsertTagCalls()[0].T
				assert.Equal(t, "Production", stored.TagName)
				assert.Equal(t, "org1", stored.OrganizationID)
				assert.Equal(t, "u1", stored.CreatedBy.String)
				assert.False(t, stored.ID.IsNil())
				assert.Contains(t, rec.Body.String(), `"tagName":"Production"`)
			}
		})
	}
}

func Test_Handler_UpdateTag(t *testing.T) {
	t.Parallel()

	type check func(*testing.T, *DBMock, *wsMock.Topic, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, _ *wsMock.Topic, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasUpdateCalled := func(count int, inp tagCore.UpdateInput) check {
		return func(t *testing.T, db *DBMock, _ *wsMock.Topic, _ *httptest.ResponseRecorder) {
			calls := db.UpdateTagCalls()
			require.Len(t, calls, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", calls[0].OrganizationID)
			assert.Equal(t, _tagID, calls[0].ID)
			assert.Equal(t, inp, calls[0].Inp)
		}
	}

	wasPublishCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, tpc *wsMock.Topic, _ *httptest.ResponseRecorder) {
			calls := tpc.PublishManyCalls()
			require.Len(t, calls, count)

			if count == 0 {
				return
			}

			assert.Equal(t, TreeChangeMessage{}, calls[0].Payload)
		}
	}

	stubDB := func(err error) *DBMock {
		return &DBMock{
			UpdateTagFunc: func(context.Context, string, xid.ID, tagCore.UpdateInput) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		TagID     string
		JSON      string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			TagID:     _tagID.String(),
			JSON:      `{`,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUpdateCalled(0, tagCore.UpdateInput{}),
				wasPublishCalled(0),
			),
		},
		"Malformed tag id": {
			DB:    &DBMock{},
			TagID: "not-an-xid",
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasUpdateCalled(0, tagCore.UpdateInput{}),
				wasPublishCalled(0),
			),
		},
		"Malformed payload": {
			DB:    &DBMock{},
			TagID: _tagID.String(),
			JSON:  `{`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasUpdateCalled(0, tagCore.UpdateInput{}),
				wasPublishCalled(0),
			),
		},
		"Empty update": {
			DB:    &DBMock{},
			TagID: _tagID.String(),
			JSON:  `{}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"tag.empty_update","message":"tag update needs a name or a colour"}`),
				wasUpdateCalled(0, tagCore.UpdateInput{}),
				wasPublishCalled(0),
			),
		},
		"Error returned by DB.UpdateTag": {
			DB:    stubDB(assert.AnError),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpdateCalled(1, tagCore.UpdateInput{TagName: null.StringFrom("Release")}),
				wasPublishCalled(0),
			),
		},
		"Duplicate name returned by DB.UpdateTag": {
			DB:    stubDB(tagCore.ErrDuplicateTagName),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusConflict, `{"code":"tag.duplicate_name","message":"tag name is already in use"}`),
				wasUpdateCalled(1, tagCore.UpdateInput{TagName: null.StringFrom("Release")}),
				wasPublishCalled(0),
			),
		},
		"Missing or foreign tag returned by DB.UpdateTag": {
			DB:    stubDB(errutil.ErrNotFound),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasUpdateCalled(1, tagCore.UpdateInput{TagName: null.StringFrom("Release")}),
				wasPublishCalled(0),
			),
		},
		"Successful update": {
			DB:    &DBMock{},
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release","color":"green"}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasUpdateCalled(1, tagCore.UpdateInput{
					TagName: null.StringFrom("Release"),
					Color:   null.StringFrom("green"),
				}),
				wasPublishCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}
			hdl.BindTreeChange(tpc)

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.JSON))
			ctx := testutil.AddChiCtx(req.Context(), "tagId", c.TagID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.UpdateTag(rec, req.WithContext(ctx))

			for _, check := range c.Checks {
				check(t, c.DB, tpc, rec)
			}
		})
	}
}

func Test_Handler_SetTagVisibility(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		TagID     string
		Payload   string
		Code      int
		Body      string
		Updates   int
		Notifies  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			TagID:     _tagID.String(),
			Payload:   `{}`,
			Code:      http.StatusUnauthorized,
			Body:      `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed tag id": {
			DB:      &DBMock{},
			TagID:   "not-an-xid",
			Payload: `{}`,
			Code:    http.StatusNotFound,
			Body:    `{"code":"general","message":"not found"}`,
		},
		"Malformed payload": {
			DB:      &DBMock{},
			TagID:   _tagID.String(),
			Payload: `{`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Error returned by DB.SetTagVisibility": {
			DB: &DBMock{
				SetTagVisibilityFunc: func(context.Context, string, string, xid.ID, tagCore.VisibilityInput) error {
					return assert.AnError
				},
			},
			TagID:   _tagID.String(),
			Payload: `{"hidden":true}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Updates: 1,
		},
		"Successful hide": {
			DB:       &DBMock{},
			TagID:    _tagID.String(),
			Payload:  `{"hidden":true}`,
			Code:     http.StatusNoContent,
			Updates:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Payload))
			ctx := testutil.AddChiCtx(req.Context(), "tagId", c.TagID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.SetTagVisibility(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.SetTagVisibilityCalls(), c.Updates)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())

			call := c.DB.SetTagVisibilityCalls()[0]
			assert.Equal(t, _tagID, call.ID)
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, "u1", call.UserID)
			assert.True(t, call.Inp.Hidden)

			// the change is one user's own, so only their sessions hear it
			owner := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})
			other := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u2",
				ActiveOrganizationID: "org1",
			})

			pubs := tpc.PublishManyCalls()
			assert.True(t, pubs[0].Filter(owner, "topic"))
			assert.False(t, pubs[0].Filter(other, "topic"))
		})
	}
}

func Test_Handler_DeleteTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		TagID     string
		Code      int
		Body      string
		Deletes   int
		Notifies  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			TagID:     _tagID.String(),
			Code:      http.StatusUnauthorized,
			Body:      `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed tag id": {
			DB:    &DBMock{},
			TagID: "not-an-xid",
			Code:  http.StatusNotFound,
			Body:  `{"code":"general","message":"not found"}`,
		},
		"Error returned by DB.DeleteTag": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error {
					return assert.AnError
				},
			},
			TagID:   _tagID.String(),
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Deletes: 1,
		},
		"Successful deletion": {
			DB:       &DBMock{},
			TagID:    _tagID.String(),
			Code:     http.StatusNoContent,
			Deletes:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)
			ctx := testutil.AddChiCtx(req.Context(), "tagId", c.TagID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.DeleteTag(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.DeleteTagCalls(), c.Deletes)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())
			assert.Equal(t, _tagID, c.DB.DeleteTagCalls()[0].ID)
			assert.Equal(t, "org1", c.DB.DeleteTagCalls()[0].OrganizationID)
		})
	}
}

func Test_Handler_FetchBranchTags(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		DocumentID string
		BranchID   string
		Code       int
		Body       string
		Fetches    int
	}{
		"No session in context": {
			DB:         &DBMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed branch id": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Error returned by DB.FetchBranchTagIDs": {
			DB: &DBMock{
				FetchBranchTagIDsFunc: func(context.Context, string, xid.ID, xid.ID) ([]xid.ID, error) {
					return nil, assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchBranchTagIDsFunc: func(context.Context, string, xid.ID, xid.ID) ([]xid.ID, error) {
					return []xid.ID{_tagID}, nil
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Code:       http.StatusOK,
			Body:       `["` + _tagID.String() + `"]`,
			Fetches:    1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			ctx := testutil.AddChiCtx(req.Context(), "documentId", c.DocumentID)
			ctx = testutil.AddChiCtx(ctx, "branchId", c.BranchID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.FetchBranchTags(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.JSONEq(t, c.Body, rec.Body.String())
			require.Len(t, c.DB.FetchBranchTagIDsCalls(), c.Fetches)

			if c.Fetches == 0 {
				return
			}

			call := c.DB.FetchBranchTagIDsCalls()[0]
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, _documentID, call.DocumentID)
			assert.Equal(t, _branchID, call.BranchID)
		})
	}
}

func Test_Handler_AssignBranchTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		DocumentID string
		BranchID   string
		Payload    string
		Code       int
		Body       string
		Fetches    int
		Assigns    int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{}`,
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			Payload:    `{}`,
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed branch id": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			Payload:    `{}`,
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed payload": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{`,
			Code:       http.StatusBadRequest,
			Body:       `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, assert.AnError)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
		},
		"Protected branch is refused": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(true, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusForbidden,
			Body:       `{"code":"document.branch_protected","message":"a protected branch cannot be changed"}`,
			Fetches:    1,
		},
		"Error returned by DB.AssignBranchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				AssignBranchTagFunc: func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			Assigns:    1,
		},
		"Successful assignment": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusNoContent,
			Fetches:    1,
			Assigns:    1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			branchTpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)
			hdl.BindBranchTagsChange(branchTpc)

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Payload))
			ctx := testutil.AddChiCtx(req.Context(), "documentId", c.DocumentID)
			ctx = testutil.AddChiCtx(ctx, "branchId", c.BranchID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.AssignBranchTag(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.Fetches)
			assert.Len(t, c.DB.AssignBranchTagCalls(), c.Assigns)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)
			assert.Len(t, branchTpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())

			call := c.DB.AssignBranchTagCalls()[0]
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, _documentID, call.DocumentID)
			assert.Equal(t, _branchID, call.BranchID)
			assert.Equal(t, _tagID, call.TagID)
		})
	}
}

func Test_Handler_UnassignBranchTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		DocumentID string
		BranchID   string
		TagID      string
		Code       int
		Body       string
		Fetches    int
		Unassigns  int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed branch id": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			TagID:      _tagID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed tag id": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      "not-an-xid",
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, assert.AnError)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
		},
		"Protected branch is refused": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(true, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusForbidden,
			Body:       `{"code":"document.branch_protected","message":"a protected branch cannot be changed"}`,
			Fetches:    1,
		},
		"Error returned by DB.UnassignBranchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				UnassignBranchTagFunc: func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			Unassigns:  1,
		},
		"Successful unassignment": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNoContent,
			Fetches:    1,
			Unassigns:  1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			branchTpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)
			hdl.BindBranchTagsChange(branchTpc)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)
			ctx := testutil.AddChiCtx(req.Context(), "documentId", c.DocumentID)
			ctx = testutil.AddChiCtx(ctx, "branchId", c.BranchID)
			ctx = testutil.AddChiCtx(ctx, "tagId", c.TagID)

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			rec := httptest.NewRecorder()
			hdl.UnassignBranchTag(rec, req.WithContext(ctx))

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.Fetches)
			assert.Len(t, c.DB.UnassignBranchTagCalls(), c.Unassigns)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)
			assert.Len(t, branchTpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())

			call := c.DB.UnassignBranchTagCalls()[0]
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, _documentID, call.DocumentID)
			assert.Equal(t, _branchID, call.BranchID)
			assert.Equal(t, _tagID, call.TagID)
		})
	}
}

func Test_extractBranchPath(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DocumentID       string
		BranchID         string
		ResultDocumentID xid.ID
		ResultBranchID   xid.ID
		Err              error
	}{
		"Malformed document id": {
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			Err:        errutil.ErrNotFound,
		},
		"Malformed branch id": {
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			Err:        errutil.ErrNotFound,
		},
		"Successful extraction": {
			DocumentID:       _documentID.String(),
			BranchID:         _branchID.String(),
			ResultDocumentID: _documentID,
			ResultBranchID:   _branchID,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			ctx := testutil.AddChiCtx(req.Context(), "documentId", c.DocumentID)
			ctx = testutil.AddChiCtx(ctx, "branchId", c.BranchID)

			documentID, branchID, err := extractBranchPath(req.WithContext(ctx))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.ResultDocumentID, documentID)
			assert.Equal(t, c.ResultBranchID, branchID)
		})
	}
}

// stubBranch returns a DB.FetchDocumentByBranchID that finds a branch,
// protected or not, or fails with err.
func stubBranch(protected bool, err error) func(context.Context, xid.ID, string) (*documentCore.Document, error) {
	return func(context.Context, xid.ID, string) (*documentCore.Document, error) {
		if err != nil {
			return nil, err
		}

		return &documentCore.Document{ID: _documentID, BranchID: _branchID, Protected: protected}, nil
	}
}
