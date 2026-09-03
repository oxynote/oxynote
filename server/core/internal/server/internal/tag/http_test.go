package tag

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	tagCore "github.com/oxynote/oxynote/server/core/internal/tag"
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
)

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// otherUserCtx returns a subscriber context for a different member of the
// same organization.
func otherUserCtx() context.Context {
	return auth.AddSessionToContext(context.Background(), auth.Session{
		UserID:               "u2",
		ActiveOrganizationID: "org1",
	})
}

// withParams puts chi path parameters on the request, which is where
// httpserver.ExtractNamedID reads them from.
func withParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()

	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// summary builds a tag summary carrying one document.
func summary(id xid.ID, name string) tagCore.Summary {
	return tagCore.Summary{
		ID:      id,
		TagName: name,
		Color:   "#22c55e",
		Documents: documentCore.Summaries{
			{ID: _documentID, DocumentName: "Runbook", Icon: "lucide:file"},
		},
	}
}

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
		"Tree fetch error": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Fetches: 1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tagCore.Summaries{summary(_tagID, "Production")}, nil
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
				req = req.WithContext(addSession(req.Context()))
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

	tree := func() tagCore.Summaries {
		return tagCore.Summaries{summary(_tagID, "Production"), summary(xid.New(), "Staging")}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Payload   string
		Code      int
		Body      string
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
		"Tree fetch error": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return nil, errors.New("boom")
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":0}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
		},
		"Sort index out of range": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree(), nil
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":9}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag_summary.invalid_sort_index","message":"sort index is out of range"}`,
		},
		"Unknown tag": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree(), nil
				},
			},
			Payload: `{"id":"` + xid.New().String() + `","sortIndex":0}`,
			Code:    http.StatusNotFound,
			Body:    `{"code":"general","message":"not found"}`,
		},
		"Tree update error": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree(), nil
				},
				UpdateTagTreeFunc: func(context.Context, tagCore.Summaries, string) error {
					return errors.New("boom")
				},
			},
			Payload: `{"id":"` + _tagID.String() + `","sortIndex":1}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Updates: 1,
		},
		"Successful swap": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tagCore.Summaries, error) {
					return tree(), nil
				},
			},
			Payload:  `{"id":"` + _tagID.String() + `","sortIndex":1}`,
			Code:     http.StatusOK,
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
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()
			hdl.UpdateTagTree(rec, req)

			assert.Equal(t, c.Code, rec.Code)
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
			Payload: `{"tagName":"","color":"#22c55e"}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag.invalid_name","message":"tag name cannot be empty"}`,
		},
		"Malformed colour": {
			DB:      &DBMock{},
			Payload: `{"tagName":"Production","color":"green"}`,
			Code:    http.StatusBadRequest,
			Body:    `{"code":"tag.invalid_color","message":"tag colour must be a hex triplet"}`,
		},
		"Tag insert error": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tagCore.Tag) error {
					return errors.New("boom")
				},
			},
			Payload: `{"tagName":"Production","color":"#22c55e"}`,
			Code:    http.StatusInternalServerError,
			Body:    `{"code":"general","message":"internal server error"}`,
			Inserts: 1,
		},
		"Successful creation": {
			DB:       &DBMock{},
			Payload:  `{"tagName":"Production","color":"#22c55e"}`,
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
				req = req.WithContext(addSession(req.Context()))
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
		"Visibility write error": {
			DB: &DBMock{
				SetTagVisibilityFunc: func(context.Context, string, string, xid.ID, tagCore.VisibilityInput) error {
					return errors.New("boom")
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

			req := withParams(
				httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Payload)),
				map[string]string{"tagId": c.TagID},
			)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()
			hdl.SetTagVisibility(rec, req)

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
			pubs := tpc.PublishManyCalls()
			assert.True(t, pubs[0].Filter(sessionCtx(), "topic"))
			assert.False(t, pubs[0].Filter(otherUserCtx(), "topic"))
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
		"Tag delete error": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error {
					return errors.New("boom")
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

			req := withParams(
				httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody),
				map[string]string{"tagId": c.TagID},
			)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()
			hdl.DeleteTag(rec, req)

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

func Test_Handler_AssignDocumentTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		DocumentID string
		Payload    string
		Code       int
		Body       string
		Assigns    int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			Payload:    `{}`,
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			DocumentID: "not-an-xid",
			Payload:    `{}`,
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed payload": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			Payload:    `{`,
			Code:       http.StatusBadRequest,
			Body:       `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Assignment error": {
			DB: &DBMock{
				AssignDocumentTagFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			DocumentID: _documentID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Assigns:    1,
		},
		"Successful assignment": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			Payload:    `{"tagId":"` + _tagID.String() + `"}`,
			Code:       http.StatusNoContent,
			Assigns:    1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := withParams(
				httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Payload)),
				map[string]string{"documentId": c.DocumentID},
			)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()
			hdl.AssignDocumentTag(rec, req)

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.AssignDocumentTagCalls(), c.Assigns)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())

			call := c.DB.AssignDocumentTagCalls()[0]
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, _documentID, call.DocumentID)
			assert.Equal(t, _tagID, call.TagID)
		})
	}
}

func Test_Handler_UnassignDocumentTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		DocumentID string
		TagID      string
		Code       int
		Body       string
		Unassigns  int
		Notifies   int
	}{
		"No session in context": {
			DB:         &DBMock{},
			NoSession:  true,
			DocumentID: _documentID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusUnauthorized,
			Body:       `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed document id": {
			DB:         &DBMock{},
			DocumentID: "not-an-xid",
			TagID:      _tagID.String(),
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Malformed tag id": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			TagID:      "not-an-xid",
			Code:       http.StatusNotFound,
			Body:       `{"code":"general","message":"not found"}`,
		},
		"Unassignment error": {
			DB: &DBMock{
				UnassignDocumentTagFunc: func(context.Context, string, xid.ID, xid.ID) error {
					return errors.New("boom")
				},
			},
			DocumentID: _documentID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusInternalServerError,
			Body:       `{"code":"general","message":"internal server error"}`,
			Unassigns:  1,
		},
		"Successful unassignment": {
			DB:         &DBMock{},
			DocumentID: _documentID.String(),
			TagID:      _tagID.String(),
			Code:       http.StatusNoContent,
			Unassigns:  1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tpc := &wsMock.Topic{}
			hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}
			hdl.BindTreeChange(tpc)

			req := withParams(
				httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody),
				map[string]string{"documentId": c.DocumentID, "tagId": c.TagID},
			)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()
			hdl.UnassignDocumentTag(rec, req)

			assert.Equal(t, c.Code, rec.Code)
			assert.Len(t, c.DB.UnassignDocumentTagCalls(), c.Unassigns)
			assert.Len(t, tpc.PublishManyCalls(), c.Notifies)

			if c.Body != "" {
				assert.JSONEq(t, c.Body, rec.Body.String())
				return
			}

			assert.Zero(t, rec.Body.Len())

			call := c.DB.UnassignDocumentTagCalls()[0]
			assert.Equal(t, "org1", call.OrganizationID)
			assert.Equal(t, _documentID, call.DocumentID)
			assert.Equal(t, _tagID, call.TagID)
		})
	}
}
