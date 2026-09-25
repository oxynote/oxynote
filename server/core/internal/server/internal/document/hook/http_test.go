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

// scheduledHook builds a stored hook whose processor needs no external
// dependencies.
func scheduledHook() *hookCore.Hook {
	return &hookCore.Hook{
		ID:             _hookID,
		Type:           hookCore.TypeScheduledReminder,
		DocumentID:     null.ValueFrom(_documentID),
		OrganizationID: null.StringFrom("org1"),
		BranchID:       null.ValueFrom(_branchID),
		Settings:       processor.Settings(`{"scale":"linear"}`),
	}
}

// storedHookManager answers an update or reset with the stored hook.
func storedHookManager() *ManagerMock {
	return &ManagerMock{
		UpdateHookFunc: func(context.Context, xid.ID, string, hookCore.UpdateInput, string) (*hookCore.Hook, error) {
			return scheduledHook(), nil
		},
		ResetHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
			return scheduledHook(), nil
		},
	}
}

// notified is one hooks-change announcement a handler made.
type notified struct {
	organizationID string
	documentID     xid.ID
	branchID       xid.ID
}

// recordNotifications binds the handler's hooks-change callback to a
// list the test reads back.
func recordNotifications(hdl *Handler) *[]notified {
	var out []notified

	hdl.hooks.changeCallback = func(organizationID string, documentID, branchID xid.ID) {
		out = append(out, notified{organizationID, documentID, branchID})
	}

	return &out
}

// assertNotified checks that a write announced nothing: the hook manager
// announces the hooks it changes.
func assertNotified(t *testing.T, got []notified) {
	t.Helper()

	assert.Empty(t, got)
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	man := &ManagerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, man)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, man, hdl.man)
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
		OmitID    bool
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
		"Missing document ID parameter": {
			DB:     &DBMock{},
			OmitID: true,
			Query:  "?branchId=" + _branchID.String(),
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
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
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Query: "?branchId=" + _branchID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchCalled(0),
			),
		},
		"Branch belongs to another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: xid.New()}, nil
				},
			},
			Query: "?branchId=" + _branchID.String(),
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`),
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
					return []hookCore.Hook{*scheduledHook()}, nil
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

			if c.DB.FetchDocumentByBranchIDFunc == nil {
				c.DB.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: _documentID}, nil
				}
			}

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}
			got := recordNotifications(&hdl)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			ctx := req.Context()

			if !c.NoSession {
				ctx = addSession(ctx)
			}

			if !c.OmitID {
				ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
			}

			rec := httptest.NewRecorder()

			hdl.FetchDocumentHooks(rec, req.WithContext(ctx))

			// a read announces nothing.
			assert.Empty(t, *got)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

// hookRequest builds a request carrying a session and the path and query
// parameters a hook route reads, unless the respective omit flags are set.
func hookRequest(method, body string, noSession, omitDoc, omitHook, omitBranch bool) *http.Request {
	target := "http://test.com/"
	if !omitBranch {
		target += "?branchId=" + _branchID.String()
	}

	req := httptest.NewRequest(method, target, strings.NewReader(body))

	ctx := req.Context()

	if !noSession {
		ctx = addSession(ctx)
	}

	if !omitDoc {
		ctx = testutil.AddChiCtx(ctx, "documentId", _documentID.String())
	}

	if !omitHook {
		ctx = testutil.AddChiCtx(ctx, "hookId", _hookID.String())
	}

	return req.WithContext(ctx)
}

// storedHookDB answers the hook lookup with the given hook.
func storedHookDB(hk *hookCore.Hook) *DBMock {
	return &DBMock{
		FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
			return hk, nil
		},
	}
}

func Test_Handler_CreateDocumentHook(t *testing.T) {
	validBody := `{"type":"scheduled-reminder","branchId":"` + _branchID.String() + `","settings":{"scale":"linear"}}`

	// the branch holds one block, so a hook can anchor to it and to nothing
	// else.
	branchDB := func() *DBMock {
		return &DBMock{
			FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
				return &document.Document{
					ID: _documentID,
					Content: document.RootBlock{
						Content: []document.Block{{
							Type:  document.BlockNodeParagraph,
							Attrs: document.Attributes{document.AttrUID: "b1"},
						}},
					},
				}, nil
			},
		}
	}

	created := func(context.Context, hookCore.CreateInput, xid.ID, string, string) (*hookCore.Hook, error) {
		return scheduledHook(), nil
	}

	cc := map[string]struct {
		DB        *DBMock
		Man       *ManagerMock
		NoSession bool
		OmitDoc   bool
		Body      string
		RespCode  int
		RespBody  string
		Creates   int
	}{
		"No session in context": {
			DB:        branchDB(),
			Man:       &ManagerMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
			RespBody:  `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Missing document ID parameter": {
			DB:       branchDB(),
			Man:      &ManagerMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Invalid JSON body": {
			DB:       branchDB(),
			Man:      &ManagerMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
			RespBody: `{"code":"request.invalid_json","message":"invalid JSON body"}`,
		},
		"Invalid hook type": {
			DB:       branchDB(),
			Man:      &ManagerMock{},
			Body:     `{"type":"bogus","branchId":"` + _branchID.String() + `","settings":{}}`,
			RespCode: http.StatusBadRequest,
			RespBody: `{"code":"document_hook.invalid_type","message":"invalid hook type"}`,
		},
		"Branch document fetch error": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, errors.New("boom")
				},
			},
			Man:      &ManagerMock{},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Branch belongs to another document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{ID: xid.New()}, nil
				},
			},
			Man:      &ManagerMock{},
			Body:     validBody,
			RespCode: http.StatusNotFound,
			RespBody: `{"code":"document.branch_mismatch","message":"branch does not belong to the document"}`,
		},
		"Block not in the branch": {
			DB:       branchDB(),
			Man:      &ManagerMock{},
			Body:     `{"type":"scheduled-reminder","branchId":"` + _branchID.String() + `","blockId":"nope","settings":{}}`,
			RespCode: http.StatusNotFound,
			RespBody: `{"code":"document.hook_block_not_found","message":"block not found in the branch"}`,
		},
		"Error returned by man.CreateHook": {
			DB: branchDB(),
			Man: &ManagerMock{
				CreateHookFunc: func(context.Context, hookCore.CreateInput, xid.ID, string, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
			Creates:  1,
		},
		"Successful creation on a block": {
			DB:       branchDB(),
			Man:      &ManagerMock{CreateHookFunc: created},
			Body:     `{"type":"scheduled-reminder","branchId":"` + _branchID.String() + `","blockId":"b1","settings":{"scale":"linear"}}`,
			RespCode: http.StatusCreated,
			Creates:  1,
		},
		"Successful creation": {
			DB:       branchDB(),
			Man:      &ManagerMock{CreateHookFunc: created},
			Body:     validBody,
			RespCode: http.StatusCreated,
			Creates:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, c.Man)
			got := recordNotifications(hdl)

			rec := httptest.NewRecorder()

			hdl.CreateDocumentHook(rec, hookRequest(http.MethodPost, c.Body, c.NoSession, c.OmitDoc, true, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assertNotified(t, *got)

			if c.RespBody != "" {
				assert.JSONEq(t, c.RespBody, rec.Body.String())
			}

			ff := c.Man.CreateHookCalls()
			require.Len(t, ff, c.Creates)

			if c.Creates > 0 {
				assert.Equal(t, _branchID, ff[0].Ci.BranchID)
				assert.Equal(t, hookCore.TypeScheduledReminder, ff[0].Ci.Type)
				assert.Equal(t, _documentID, ff[0].DocumentID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
				assert.Equal(t, "u1", ff[0].UpdatedBy)
			}
		})
	}
}

func Test_Handler_UpdateDocumentHook(t *testing.T) {
	validBody := `{"settings":{"scale":"linear","duration":"48h"}}`

	otherBranch := scheduledHook()
	otherBranch.BranchID = null.ValueFrom(xid.New())

	otherDocument := scheduledHook()
	otherDocument.DocumentID = null.ValueFrom(xid.New())

	cc := map[string]struct {
		DB         *DBMock
		Man        *ManagerMock
		NoSession  bool
		OmitDoc    bool
		OmitHook   bool
		OmitBranch bool
		Body       string
		RespCode   int
		RespBody   string
		Updates    int
	}{
		"No session in context": {
			DB:        storedHookDB(scheduledHook()),
			Man:       &ManagerMock{},
			NoSession: true,
			Body:      validBody,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitDoc:  true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Missing hook ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitHook: true,
			Body:     validBody,
			RespCode: http.StatusNotFound,
		},
		"Missing branch ID query parameter": {
			DB:         storedHookDB(scheduledHook()),
			Man:        &ManagerMock{},
			OmitBranch: true,
			Body:       validBody,
			RespCode:   http.StatusBadRequest,
			RespBody:   `{"code":"request.invalid_form","message":"invalid form data"}`,
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Man:      &ManagerMock{},
			Body:     validBody,
			RespCode: http.StatusInternalServerError,
		},
		"Hook of another document": {
			DB:       storedHookDB(otherDocument),
			Man:      &ManagerMock{},
			Body:     validBody,
			RespCode: http.StatusNotFound,
			RespBody: `{"code":"document.hook_mismatch","message":"hook does not belong to the document"}`,
		},
		"Hook of another branch": {
			DB:       storedHookDB(otherBranch),
			Man:      &ManagerMock{},
			Body:     validBody,
			RespCode: http.StatusNotFound,
			RespBody: `{"code":"document.hook_mismatch","message":"hook does not belong to the document"}`,
		},
		"Invalid JSON body": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by man.UpdateHook": {
			DB: storedHookDB(scheduledHook()),
			Man: &ManagerMock{
				UpdateHookFunc: func(context.Context, xid.ID, string, hookCore.UpdateInput, string) (*hookCore.Hook, error) {
					return nil, hookCore.ErrUpstreamUnavailable
				},
			},
			Body:     validBody,
			RespCode: http.StatusFailedDependency,
			RespBody: `{"code":"document_hook.upstream_unavailable","message":"the service the hook checks is unavailable"}`,
			Updates:  1,
		},
		"Successful update": {
			DB:       storedHookDB(scheduledHook()),
			Man:      storedHookManager(),
			Body:     validBody,
			RespCode: http.StatusOK,
			Updates:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, c.Man)
			got := recordNotifications(hdl)

			rec := httptest.NewRecorder()

			hdl.UpdateDocumentHook(rec, hookRequest(http.MethodPut, c.Body, c.NoSession, c.OmitDoc, c.OmitHook, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)
			assertNotified(t, *got)

			if c.RespBody != "" {
				assert.JSONEq(t, c.RespBody, rec.Body.String())
			}

			ff := c.Man.UpdateHookCalls()
			require.Len(t, ff, c.Updates)

			if c.Updates > 0 {
				assert.Equal(t, _hookID, ff[0].ID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
				assert.JSONEq(t, `{"scale":"linear","duration":"48h"}`, string(ff[0].UI.Settings))
				assert.Equal(t, "u1", ff[0].UpdatedBy)
			}
		})
	}
}

func Test_Handler_ResetDocumentHook(t *testing.T) {
	otherDocument := scheduledHook()
	otherDocument.DocumentID = null.ValueFrom(xid.New())

	cc := map[string]struct {
		DB        *DBMock
		Man       *ManagerMock
		NoSession bool
		OmitDoc   bool
		OmitHook  bool
		RespCode  int
		Resets    int
	}{
		"No session in context": {
			DB:        storedHookDB(scheduledHook()),
			Man:       &ManagerMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing hook ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitHook: true,
			RespCode: http.StatusNotFound,
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Man:      &ManagerMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Hook of another document": {
			DB:       storedHookDB(otherDocument),
			Man:      &ManagerMock{},
			RespCode: http.StatusNotFound,
		},
		"Error returned by man.ResetHook": {
			DB: storedHookDB(scheduledHook()),
			Man: &ManagerMock{
				ResetHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Resets:   1,
		},
		"Successful reset": {
			DB:       storedHookDB(scheduledHook()),
			Man:      storedHookManager(),
			RespCode: http.StatusOK,
			Resets:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, c.Man)
			got := recordNotifications(hdl)

			rec := httptest.NewRecorder()

			// a reset stays on the public route, which names no branch.
			hdl.ResetDocumentHook(rec, hookRequest(http.MethodPut, "", c.NoSession, c.OmitDoc, c.OmitHook, true))

			assert.Equal(t, c.RespCode, rec.Code)
			assertNotified(t, *got)

			ff := c.Man.ResetHookCalls()
			require.Len(t, ff, c.Resets)

			if c.Resets > 0 {
				assert.Equal(t, _hookID, ff[0].ID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
			}
		})
	}
}

func Test_Handler_DeleteDocumentHook(t *testing.T) {
	otherBranch := scheduledHook()
	otherBranch.BranchID = null.ValueFrom(xid.New())

	otherDocument := scheduledHook()
	otherDocument.DocumentID = null.ValueFrom(xid.New())

	cc := map[string]struct {
		DB         *DBMock
		Man        *ManagerMock
		NoSession  bool
		OmitDoc    bool
		OmitHook   bool
		OmitBranch bool
		RespCode   int
		Deletes    int
	}{
		"No session in context": {
			DB:        storedHookDB(scheduledHook()),
			Man:       &ManagerMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Missing document ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitDoc:  true,
			RespCode: http.StatusNotFound,
		},
		"Missing hook ID parameter": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			OmitHook: true,
			RespCode: http.StatusNotFound,
		},
		"Missing branch ID query parameter": {
			DB:         storedHookDB(scheduledHook()),
			Man:        &ManagerMock{},
			OmitBranch: true,
			RespCode:   http.StatusBadRequest,
		},
		"Hook fetch error": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hookCore.Hook, error) {
					return nil, errors.New("boom")
				},
			},
			Man:      &ManagerMock{},
			RespCode: http.StatusInternalServerError,
		},
		"Hook of another document": {
			DB:       storedHookDB(otherDocument),
			Man:      &ManagerMock{},
			RespCode: http.StatusNotFound,
		},
		"Hook of another branch": {
			DB:       storedHookDB(otherBranch),
			Man:      &ManagerMock{},
			RespCode: http.StatusNotFound,
		},
		"Error returned by man.DeleteHook": {
			DB: storedHookDB(scheduledHook()),
			Man: &ManagerMock{
				DeleteHookFunc: func(context.Context, xid.ID, string, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deletes:  1,
		},
		"Successful deletion": {
			DB:       storedHookDB(scheduledHook()),
			Man:      &ManagerMock{},
			RespCode: http.StatusNoContent,
			Deletes:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, c.Man)
			got := recordNotifications(hdl)

			rec := httptest.NewRecorder()

			hdl.DeleteDocumentHook(rec, hookRequest(http.MethodDelete, "", c.NoSession, c.OmitDoc, c.OmitHook, c.OmitBranch))

			assert.Equal(t, c.RespCode, rec.Code)
			assertNotified(t, *got)

			ff := c.Man.DeleteHookCalls()
			require.Len(t, ff, c.Deletes)

			if c.Deletes > 0 {
				assert.Equal(t, _hookID, ff[0].ID)
				assert.Equal(t, "org1", ff[0].OrganizationID)
				assert.Equal(t, "u1", ff[0].UpdatedBy)
			}
		})
	}
}
