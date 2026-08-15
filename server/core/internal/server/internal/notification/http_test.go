package notification

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	notificationCore "github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	rcv := &fakeReceiver{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), db, rcv)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, rcv, hdl.notifier)
}

func Test_Handler_FetchManyNotifications(t *testing.T) {
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
			ff := db.FetchManyNotificationsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, "u1", ff[0].UserID)
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
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasFetchCalled(0),
			),
		},
		"Invalid query parameters": {
			DB:    &DBMock{},
			Query: "?limit=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasFetchCalled(0),
			),
		},
		"Notification fetch error": {
			DB: &DBMock{
				FetchManyNotificationsFunc: func(context.Context, string, string, httpserver.Query) ([]*notificationCore.Notification, uint64, error) {
					return nil, 0, errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasFetchCalled(1),
			),
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchManyNotificationsFunc: func(context.Context, string, string, httpserver.Query) ([]*notificationCore.Notification, uint64, error) {
					return []*notificationCore.Notification{
						{
							ID:             xid.NilID(),
							UserID:         "u1",
							OrganizationID: "org1",
						},
					}, 3, nil
				},
			},
			Checks: checks(
				func(t *testing.T, _ *DBMock, rec *httptest.ResponseRecorder) {
					assert.Equal(t, http.StatusOK, rec.Code)
					assert.Contains(t, rec.Body.String(), `"pageCount":3`)
					assert.Contains(t, rec.Body.String(), `"userId":"u1"`)
				},
				wasFetchCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchManyNotifications(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_FetchNotificationsCount(t *testing.T) {
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

	wasCountCalled := func(count int, read bool) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.FetchNotificationCountCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, "u1", ff[0].UserID)
			assert.Equal(t, read, ff[0].Read)
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
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasCountCalled(0, false),
			),
		},
		"Invalid form data": {
			DB:    &DBMock{},
			Query: "?read=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_form","message":"invalid form data"}`),
				wasCountCalled(0, false),
			),
		},
		"Count fetch error": {
			DB: &DBMock{
				FetchNotificationCountFunc: func(context.Context, string, string, bool) (uint64, error) {
					return 0, errors.New("boom")
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasCountCalled(1, false),
			),
		},
		"Successful count fetch": {
			DB: &DBMock{
				FetchNotificationCountFunc: func(context.Context, string, string, bool) (uint64, error) {
					return 5, nil
				},
			},
			Query: "?read=true",
			Checks: checks(
				hasResp(http.StatusOK, `{"count":5}`),
				wasCountCalled(1, true),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/"+c.Query, http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchNotificationsCount(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}

func Test_Handler_MarkReadManyNotifications(t *testing.T) {
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

	ntID := xid.New()

	wasMarkReadCalled := func(count, ids int) check {
		return func(t *testing.T, db *DBMock, _ *httptest.ResponseRecorder) {
			ff := db.MarkReadByNotificationsIDsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrganizationID)
			assert.Equal(t, "u1", ff[0].UserID)
			require.Len(t, ff[0].Ids, ids)

			if ids > 0 {
				assert.Equal(t, ntID, ff[0].Ids[0])
			}
		}
	}

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Body      string
		Checks    []check
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      `{"ids":[]}`,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasMarkReadCalled(0, 0),
			),
		},
		"Invalid JSON body": {
			DB:   &DBMock{},
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasMarkReadCalled(0, 0),
			),
		},
		"Mark read error": {
			DB: &DBMock{
				MarkReadByNotificationsIDsFunc: func(context.Context, string, string, []xid.ID) error {
					return errors.New("boom")
				},
			},
			Body: `{"ids":["` + ntID.String() + `"]}`,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasMarkReadCalled(1, 1),
			),
		},
		"Successful mark read of all notifications": {
			DB:   &DBMock{},
			Body: `{}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasMarkReadCalled(1, 0),
			),
		},
		"Successful mark read by IDs": {
			DB:   &DBMock{},
			Body: `{"ids":["` + ntID.String() + `"]}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasMarkReadCalled(1, 1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log: slog.New(slog.DiscardHandler),
				db:  c.DB,
			}

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Body))

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.MarkReadManyNotifications(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.DB, rec)
			}
		})
	}
}
