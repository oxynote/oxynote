package tag

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	tagCore "github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
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
			Payload:  `{"tagName":" Production ","color":"green"}`,
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

	stored := tagCore.Tag{
		ID:             _tagID,
		OrganizationID: "org1",
		TagName:        "Production",
		Color:          "blue",
		SortIndex:      2,
	}

	updated := func(name, color string) tagCore.Tag {
		t := stored
		t.TagName = name
		t.Color = color

		return t
	}

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

	wasFetchCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *wsMock.Topic, _ *httptest.ResponseRecorder) {
			calls := db.FetchTagCalls()
			require.Len(t, calls, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _tagID, calls[0].ID)
			assert.Equal(t, "org1", calls[0].OrganizationID)
		}
	}

	wasUpdateCalled := func(count int, tg tagCore.Tag) check {
		return func(t *testing.T, db *DBMock, _ *wsMock.Topic, _ *httptest.ResponseRecorder) {
			calls := db.UpdateTagCalls()
			require.Len(t, calls, count)

			if count == 0 {
				return
			}

			assert.Equal(t, tg, calls[0].T)
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

	stubDB := func(fetchErr, updateErr error) *DBMock {
		return &DBMock{
			FetchTagFunc: func(context.Context, xid.ID, string) (*tagCore.Tag, error) {
				if fetchErr != nil {
					return nil, fetchErr
				}

				t := stored

				return &t, nil
			},
			UpdateTagFunc: func(context.Context, tagCore.Tag) error {
				return updateErr
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
				wasFetchCalled(0),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Malformed tag id": {
			DB:    &DBMock{},
			TagID: "not-an-xid",
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasFetchCalled(0),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Malformed payload": {
			DB:    &DBMock{},
			TagID: _tagID.String(),
			JSON:  `{`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasFetchCalled(0),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Error returned by DB.FetchTag": {
			DB:    stubDB(assert.AnError, nil),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchCalled(1),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Missing or foreign tag returned by DB.FetchTag": {
			DB:    stubDB(errutil.ErrNotFound, nil),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasFetchCalled(1),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Empty update": {
			DB:    stubDB(nil, nil),
			TagID: _tagID.String(),
			JSON:  `{}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"tag.empty_update","message":"tag update needs a name or a colour"}`),
				wasFetchCalled(1),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Name over the length limit": {
			DB:    stubDB(nil, nil),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"` + strings.Repeat("a", 65) + `"}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"tag.name_too_long","message":"tag name cannot be longer than 64 characters"}`),
				wasFetchCalled(1),
				wasUpdateCalled(0, tagCore.Tag{}),
				wasPublishCalled(0),
			),
		},
		"Error returned by DB.UpdateTag": {
			DB:    stubDB(nil, assert.AnError),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchCalled(1),
				wasUpdateCalled(1, updated("Release", "blue")),
				wasPublishCalled(0),
			),
		},
		"Duplicate name returned by DB.UpdateTag": {
			DB:    stubDB(nil, tagCore.ErrDuplicateTagName),
			TagID: _tagID.String(),
			JSON:  `{"tagName":"Release"}`,
			Checks: checks(
				hasResp(http.StatusConflict, `{"code":"tag.duplicate_name","message":"tag name is already in use"}`),
				wasFetchCalled(1),
				wasUpdateCalled(1, updated("Release", "blue")),
				wasPublishCalled(0),
			),
		},
		"Successful update": {
			DB:    stubDB(nil, nil),
			TagID: _tagID.String(),
			JSON:  `{"tagName":" Release ","color":"green"}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasFetchCalled(1),
				wasUpdateCalled(1, updated("Release", "green")),
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
		Tx         *TxMock
		BeginErr   error
		NoSession  bool
		DocumentID string
		BranchID   string
		Payload    string
		Code       int
		Body       string
		Fetches    int
		TagFetches int
		Begun      int
		Writes     int
		Entries    int
		Committed  int
		RolledBack int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{}`,
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			Payload:    `{}`,
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed branch id": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			Payload:    `{}`,
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed payload": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{`,
			Code:       http.StatusBadRequest,
			Body:       `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, assert.AnError)},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
		},
		"Protected branch is refused": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(true, nil)},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusForbidden,
			Body:       `{"code":"document.branch_protected","message":"a protected branch cannot be changed"}`,
			Fetches:    1,
		},
		"Error returned by DB.FetchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(assert.AnError),
			},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{},
			BeginErr:   assert.AnError,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
		},
		"Error returned by Tx.AssignBranchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				AssignBranchTagFunc: stubTagWrite(false, assert.AnError),
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				AssignBranchTagFunc: stubTagWrite(true, nil),
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				AssignBranchTagFunc: stubTagWrite(true, nil),
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Tag the branch already carries records no entry": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusNoContent,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Committed:  1,
			RolledBack: 1,
			Notifies:   1,
		},
		"Successful change": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{AssignBranchTagFunc: stubTagWrite(true, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusNoContent,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.DB.BeginTxFunc = testutil.StubBeginTx[sqlutil.Tx](c.Tx, c.BeginErr)

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
			assert.Len(t, c.DB.FetchTagCalls(), c.TagFetches)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			assert.Len(t, c.Tx.AssignBranchTagCalls(), c.Writes)
			assert.Len(t, c.Tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.RolledBack)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)
			assert.Len(t, branchTpc.PublishManyCalls(), c.Notifies)

			if c.Writes > 0 {
				call := c.Tx.AssignBranchTagCalls()[0]
				assert.Equal(t, "org1", call.OrganizationID)
				assert.Equal(t, _documentID, call.DocumentID)
				assert.Equal(t, _branchID, call.BranchID)
				assert.Equal(t, _tagID, call.TagID)
			}

			if c.Entries > 0 {
				entries := c.Tx.InsertDocumentActivityEntriesCalls()[0].Entries
				require.Len(t, entries, 1)
				assert.Equal(t, activity.TypeTagAssigned, entries[0].Type)
				assert.Equal(t, activity.UserActor("u1"), entries[0].Actor)
				assert.Equal(t, activity.NewTarget(activity.TargetKindTag, _tagID.String()), entries[0].Target)
				assert.Equal(t, activity.Metadata{activity.MetaKeyTagName: "Production"}, entries[0].Metadata)
				assert.Equal(t, _documentID, entries[0].DocumentID)
				assert.Equal(t, _branchID, entries[0].BranchID)
			}

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())
		})
	}
}

func Test_Handler_UnassignBranchTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		Tx         *TxMock
		BeginErr   error
		NoSession  bool
		DocumentID string
		BranchID   string
		TagID      string
		Code       int
		Body       string
		Fetches    int
		TagFetches int
		Begun      int
		Writes     int
		Entries    int
		Committed  int
		RolledBack int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: "not-an-xid",
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed branch id": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   "not-an-xid",
			TagID:      _tagID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed tag id": {
			DB:         &DBMock{},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      "not-an-xid",
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Error returned by DB.FetchDocumentByBranchID": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(false, assert.AnError)},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
		},
		"Protected branch is refused": {
			DB:         &DBMock{FetchDocumentByBranchIDFunc: stubBranch(true, nil)},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusForbidden,
			Body:       `{"code":"document.branch_protected","message":"a protected branch cannot be changed"}`,
			Fetches:    1,
		},
		"Error returned by DB.FetchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(assert.AnError),
			},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
		},
		"Error returned by DB.BeginTx": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{},
			BeginErr:   assert.AnError,
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
		},
		"Error returned by Tx.UnassignBranchTag": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				UnassignBranchTagFunc: stubTagWrite(false, assert.AnError),
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			RolledBack: 1,
		},
		"Error returned by Tx.InsertDocumentActivityEntries": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				UnassignBranchTagFunc: stubTagWrite(true, nil),
				InsertDocumentActivityEntriesFunc: func(context.Context, ...activity.Entry) error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			RolledBack: 1,
		},
		"Error returned by Tx.Commit": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx: &TxMock{
				UnassignBranchTagFunc: stubTagWrite(true, nil),
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
		},
		"Tag the branch does not carry records no entry": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNoContent,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Committed:  1,
			RolledBack: 1,
			Notifies:   1,
		},
		"Successful change": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: stubBranch(false, nil),
				FetchTagFunc:                stubTag(nil),
			},
			Tx:         &TxMock{UnassignBranchTagFunc: stubTagWrite(true, nil)},
			DocumentID: _documentID.String(),
			BranchID:   _branchID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNoContent,
			Fetches:    1,
			TagFetches: 1,
			Begun:      1,
			Writes:     1,
			Entries:    1,
			Committed:  1,
			RolledBack: 1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.DB.BeginTxFunc = testutil.StubBeginTx[sqlutil.Tx](c.Tx, c.BeginErr)

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
			assert.Len(t, c.DB.FetchTagCalls(), c.TagFetches)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begun)
			assert.Len(t, c.Tx.UnassignBranchTagCalls(), c.Writes)
			assert.Len(t, c.Tx.InsertDocumentActivityEntriesCalls(), c.Entries)
			assert.Len(t, c.Tx.CommitCalls(), c.Committed)
			assert.Len(t, c.Tx.RollbackCalls(), c.RolledBack)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)
			assert.Len(t, branchTpc.PublishManyCalls(), c.Notifies)

			if c.Writes > 0 {
				call := c.Tx.UnassignBranchTagCalls()[0]
				assert.Equal(t, "org1", call.OrganizationID)
				assert.Equal(t, _documentID, call.DocumentID)
				assert.Equal(t, _branchID, call.BranchID)
				assert.Equal(t, _tagID, call.TagID)
			}

			if c.Entries > 0 {
				entries := c.Tx.InsertDocumentActivityEntriesCalls()[0].Entries
				require.Len(t, entries, 1)
				assert.Equal(t, activity.TypeTagUnassigned, entries[0].Type)
				assert.Equal(t, activity.UserActor("u1"), entries[0].Actor)
				assert.Equal(t, activity.NewTarget(activity.TargetKindTag, _tagID.String()), entries[0].Target)
				assert.Equal(t, activity.Metadata{activity.MetaKeyTagName: "Production"}, entries[0].Metadata)
				assert.Equal(t, _documentID, entries[0].DocumentID)
				assert.Equal(t, _branchID, entries[0].BranchID)
			}

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())
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
// stubTag returns a tag fetch that finds the tag or fails with err.
func stubTagWrite(changed bool, err error) func(context.Context, string, xid.ID, xid.ID, xid.ID) (bool, error) {
	return func(context.Context, string, xid.ID, xid.ID, xid.ID) (bool, error) {
		return changed, err
	}
}

func stubTag(err error) func(context.Context, xid.ID, string) (*tagCore.Tag, error) {
	return func(context.Context, xid.ID, string) (*tagCore.Tag, error) {
		if err != nil {
			return nil, err
		}

		return &tagCore.Tag{ID: _tagID, TagName: "Production"}, nil
	}
}

func stubBranch(protected bool, err error) func(context.Context, xid.ID, string) (*documentCore.Document, error) {
	return func(context.Context, xid.ID, string) (*documentCore.Document, error) {
		if err != nil {
			return nil, err
		}

		return &documentCore.Document{ID: _documentID, BranchID: _branchID, Protected: protected}, nil
	}
}
