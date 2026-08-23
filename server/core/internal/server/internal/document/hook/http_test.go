package hook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	hookCore "github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
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
	_hookID     = xid.New()
)

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// urlWatcherHook builds a stored url-watcher hook that already holds a
// changedetection.io watcher in its state.
func urlWatcherHook() *hookCore.Hook {
	return &hookCore.Hook{
		ID:             _hookID,
		Type:           hookCore.TypeURLWatcher,
		DocumentID:     null.ValueFrom(_documentID),
		OrganizationID: null.StringFrom("org1"),
		Settings:       processor.Settings(`{"url":"https://example.com"}`),
		State:          processor.State(`{"watcherId":"w1"}`),
	}
}

// scheduledHook builds a stored hook whose processor needs no external
// dependencies.
func scheduledHook(typ hookCore.Type) *hookCore.Hook {
	return &hookCore.Hook{
		ID:             _hookID,
		Type:           typ,
		DocumentID:     null.ValueFrom(_documentID),
		OrganizationID: null.StringFrom("org1"),
		Settings:       processor.Settings(`{"scale":"linear"}`),
	}
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, nil, nil)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Nil(t, hdl.githubMan)
	assert.Nil(t, hdl.webchangeClient)
}

func Test_Handler_FetchDocumentHooks(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasFetchCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchDocumentHooksByBranchIDCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _branchID, ff[0].BranchID)
			assert.Equal(t, "org1", ff[0].OrganizationID)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Query     string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Query:     "?branchId=" + _branchID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasFetchCalled(0),
			),
		},
		"Invalid branch ID query parameter": {
			DB:    &DBMock{},
			Query: "?branchId=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasFetchCalled(0),
			),
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Query: "?branchId=" + _branchID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchCalled(1),
			),
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hookCore.Hook, error) {
					return []hookCore.Hook{*scheduledHook(hookCore.TypeScheduledReminder)}, nil
				},
			},
			Query: "?branchId=" + _branchID.String(),
			Checks: checks(
				func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Contains(t, rec.Body.String(), _hookID.String())
				},
				wasFetchCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:             slog.New(slog.DiscardHandler),
				db:              c.DB,
				webchangeClient: webchange.NewClient("", ""),
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchDocumentHooks(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_CreateDocumentHook(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasInsertCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.InsertDocumentHookCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, hookCore.TypeScheduledReminder, ff[0].Hk.Type)
			assert.Equal(t, null.ValueFrom(_documentID), ff[0].Hk.DocumentID)
			assert.Equal(t, null.StringFrom("org1"), ff[0].Hk.OrganizationID)
			assert.NotEmpty(t, ff[0].Hk.State)
		}
	}

	validBody := `{"type":"scheduled-reminder","branchId":"` + _branchID.String() + `","settings":{"scale":"linear"}}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitID    bool
		Body      string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasInsertCalled(0),
			),
		},
		"Missing document ID parameter": {
			DB:     &DBMock{},
			OmitID: true,
			Body:   validBody,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasInsertCalled(0),
			),
		},
		"Invalid JSON body": {
			DB:   &DBMock{},
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasInsertCalled(0),
			),
		},
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertCalled(0),
			),
		},
		"Invalid hook type": {
			DB:   &DBMock{},
			Body: `{"type":"bogus","branchId":"` + _branchID.String() + `","settings":{}}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"document_hook.invalid_type","message":"invalid hook type"}`),
				wasInsertCalled(0),
			),
		},
		"Hook insertion error": {
			DB: &DBMock{
				InsertDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasInsertCalled(1),
			),
		},
		"Branch belongs to another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: xid.New()}, nil
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`),
				wasInsertCalled(0),
			),
		},
		"URL watcher without changedetection": {
			DB:   &DBMock{},
			Body: `{"type":"url-watcher","branchId":"` + _branchID.String() + `","settings":{"url":"https://example.com"}}`,
			Checks: checks(
				hasResp(http.StatusConflict, `{"code":"changedetection.not_configured","message":"changedetection is not configured"}`),
				wasInsertCalled(0),
			),
		},
		"Successful creation": {
			DB:   &DBMock{},
			Body: validBody,
			Checks: checks(
				func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusCreated, rec.Code)
					assert.Contains(t, rec.Body.String(), `"scheduled-reminder"`)
				},
				wasInsertCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.DB.FetchDocumentByBranchIDFunc == nil {
				c.DB.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: _documentID}, nil
				}
			}

			hdl := Handler{
				log:             slog.New(slog.DiscardHandler),
				db:              c.DB,
				webchangeClient: webchange.NewClient("", ""),
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Body))

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			rec := httptest.NewRecorder()

			hdl.CreateDocumentHook(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_UpdateDocumentHook(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasUpdateCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.UpdateDocumentHookCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _hookID, ff[0].Hk.ID)
			assert.JSONEq(t, `{"scale":"linear","duration":"48h"}`, string(ff[0].Hk.Settings))
			assert.True(t, ff[0].Hk.UpdatedAt.Valid)
		}
	}

	validBody := `{"settings":{"scale":"linear","duration":"48h"}}`

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitID    bool
		Body      string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      validBody,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasUpdateCalled(0),
			),
		},
		"Missing hook ID parameter": {
			DB:     &DBMock{},
			OmitID: true,
			Body:   validBody,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasUpdateCalled(0),
			),
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpdateCalled(0),
			),
		},
		"Invalid JSON body": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
			},
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasUpdateCalled(0),
			),
		},
		"Update application error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook("bogus"), nil
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpdateCalled(0),
			),
		},
		"Hook update error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
				UpdateDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			Body: validBody,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasUpdateCalled(1),
			),
		},
		"URL watcher without changedetection": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return urlWatcherHook(), nil
				},
			},
			Body: `{"settings":{"url":"https://example.com"}}`,
			Checks: checks(
				hasResp(http.StatusConflict, `{"code":"changedetection.not_configured","message":"changedetection is not configured"}`),
				wasUpdateCalled(0),
			),
		},
		"Successful update": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
			},
			Body: validBody,
			Checks: checks(
				func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Contains(t, rec.Body.String(), _hookID.String())
				},
				wasUpdateCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:             slog.New(slog.DiscardHandler),
				db:              c.DB,
				webchangeClient: webchange.NewClient("", ""),
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Body))

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "hookId", _hookID.String())
			}

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentHook(rec, req.WithContext(ctx))

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_ResetDocumentHook(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	wasUpdateCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.UpdateDocumentHookCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _hookID, ff[0].Hk.ID)
			assert.NotEmpty(t, ff[0].Hk.State)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitID    bool
		RespCode  int
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
			Checks:    checks(wasUpdateCalled(0)),
		},
		"Missing hook ID parameter": {
			DB:       &DBMock{},
			OmitID:   true,
			RespCode: http.StatusNotFound,
			Checks:   checks(wasUpdateCalled(0)),
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasUpdateCalled(0)),
		},
		"Reset error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook("bogus"), nil
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasUpdateCalled(0)),
		},
		"Hook update error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
				UpdateDocumentHookFunc: func(context.Context, hookCore.Hook) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasUpdateCalled(1)),
		},
		"URL watcher without changedetection": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return urlWatcherHook(), nil
				},
			},
			RespCode: http.StatusConflict,
			Checks:   checks(wasUpdateCalled(0)),
		},
		"Successful reset": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
			},
			RespCode: http.StatusOK,
			Checks:   checks(wasUpdateCalled(1)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:             slog.New(slog.DiscardHandler),
				db:              c.DB,
				webchangeClient: webchange.NewClient("", ""),
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "hookId", _hookID.String())
			}

			rec := httptest.NewRecorder()

			hdl.ResetDocumentHook(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_DeleteDocumentHook(t *testing.T) {
	type check func(*testing.T, *DBMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.DeleteDocumentHookCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _hookID, ff[0].ID)
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		OmitID    bool
		RespCode  int
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
			Checks:    checks(wasDeleteCalled(0)),
		},
		"Missing hook ID parameter": {
			DB:       &DBMock{},
			OmitID:   true,
			RespCode: http.StatusNotFound,
			Checks:   checks(wasDeleteCalled(0)),
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasDeleteCalled(0)),
		},
		"External cleanup error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook("bogus"), nil
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasDeleteCalled(0)),
		},
		"Hook deletion error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
				DeleteDocumentHookFunc: func(context.Context, xid.ID) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Checks:   checks(wasDeleteCalled(1)),
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return scheduledHook(hookCore.TypeScheduledReminder), nil
				},
			},
			RespCode: http.StatusNoContent,
			Checks:   checks(wasDeleteCalled(1)),
		},

		// an unconfigured changedetection must not trap the row: the
		// teardown is skipped and the deletion proceeds.
		"URL watcher deletion without changedetection": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return urlWatcherHook(), nil
				},
			},
			RespCode: http.StatusNoContent,
			Checks:   checks(wasDeleteCalled(1)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:             slog.New(slog.DiscardHandler),
				db:              c.DB,
				webchangeClient: webchange.NewClient("", ""),
			}

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "hookId", _hookID.String())
			}

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentHook(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}
