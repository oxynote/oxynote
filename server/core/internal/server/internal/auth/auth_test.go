package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// testMiddlewareForwardsCookies is a case of Middleware, run as a subtest of it.
func testMiddlewareForwardsCookies(t *testing.T) {
	t.Parallel()

	client, mt := testutil.MockHTTP()

	var gotCookies []*http.Cookie

	mt.RegisterResponder(http.MethodGet, "http://test.com/get-session",
		func(r *http.Request) (*http.Response, error) {
			gotCookies = r.Cookies()

			return httpmock.NewStringResponse(
				http.StatusOK,
				`{"session":{"userId":"u1","activeOrganizationId":"org1"}}`,
			), nil
		},
	)

	hdl := Middleware(
		slog.New(slog.DiscardHandler),
		Options{BetterAuthURL: "http://test.com/get-session"},
		client,
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "tkn", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	req.AddCookie(&http.Cookie{Name: "other", Value: "val", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})

	rec := httptest.NewRecorder()

	hdl.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, gotCookies, 2)
	assert.Equal(t, "session_token", gotCookies[0].Name)
	assert.Equal(t, "tkn", gotCookies[0].Value)
	assert.Equal(t, "other", gotCookies[1].Name)
	assert.Equal(t, "val", gotCookies[1].Value)
}

func Test_Middleware(t *testing.T) {
	t.Parallel()

	t.Run("Cookies are forwarded to the auth service", testMiddlewareForwardsCookies)

	cc := map[string]struct {
		URL      string
		Resp     httpmock.Responder
		NextCode int
		RespCode int
		RespJSON string
		Session  Session
	}{
		"Invalid request creation": {
			URL:      "http://test.com/\x7f",
			RespCode: http.StatusInternalServerError,
			RespJSON: `{"code":"general","message":"internal server error"}`,
		},
		"Transport error": {
			URL:      "http://test.com/get-session",
			Resp:     httpmock.ConnectionFailure,
			RespCode: http.StatusInternalServerError,
			RespJSON: `{"code":"general","message":"internal server error"}`,
		},
		"Non-200 response": {
			URL:      "http://test.com/get-session",
			Resp:     httpmock.NewStringResponder(http.StatusUnauthorized, ""),
			RespCode: http.StatusUnauthorized,
			RespJSON: `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Malformed response body": {
			URL:      "http://test.com/get-session",
			Resp:     httpmock.NewStringResponder(http.StatusOK, "{"),
			RespCode: http.StatusInternalServerError,
			RespJSON: `{"code":"general","message":"internal server error"}`,
		},
		"Empty user ID": {
			URL:      "http://test.com/get-session",
			Resp:     httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"","activeOrganizationId":"org1"}}`),
			RespCode: http.StatusUnauthorized,
			RespJSON: `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Successful authentication": {
			URL:      "http://test.com/get-session",
			Resp:     httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"u1","activeOrganizationId":"org1"}}`),
			NextCode: http.StatusNoContent,
			RespCode: http.StatusNoContent,
			Session: Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := testutil.MockHTTP()

			if c.Resp != nil {
				mt.RegisterResponder(http.MethodGet, c.URL, c.Resp)
			}

			var (
				nextCalled bool
				gotSession Session
			)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true

				var err error

				gotSession, err = ExtractSessionFromContext(r.Context())
				assert.NoError(t, err)

				w.WriteHeader(c.NextCode)
			})

			hdl := Middleware(
				slog.New(slog.DiscardHandler),
				Options{BetterAuthURL: c.URL},
				client,
			)(next)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			rec := httptest.NewRecorder()

			hdl.ServeHTTP(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
			} else {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			if c.NextCode == 0 {
				assert.False(t, nextCalled)
				return
			}

			assert.True(t, nextCalled)
			assert.Equal(t, c.Session, gotSession)
		})
	}
}

func Test_FilterOrganization(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Ctx    context.Context
		Result bool
	}{
		"No session in context": {
			Ctx:    context.Background(),
			Result: false,
		},
		"Mismatched organization": {
			Ctx: AddSessionToContext(context.Background(), Session{
				UserID:               "u1",
				ActiveOrganizationID: "org2",
			}),
			Result: false,
		},
		"Matching organization": {
			Ctx: AddSessionToContext(context.Background(), Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			}),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, FilterOrganization("org1")(c.Ctx, "topic"))
		})
	}
}

func Test_FilterUser(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Ctx    context.Context
		Result bool
	}{
		"No session in context": {
			Ctx:    context.Background(),
			Result: false,
		},
		"Mismatched organization": {
			Ctx: AddSessionToContext(context.Background(), Session{
				UserID:               "u1",
				ActiveOrganizationID: "org2",
			}),
			Result: false,
		},
		"Mismatched user": {
			Ctx: AddSessionToContext(context.Background(), Session{
				UserID:               "u2",
				ActiveOrganizationID: "org1",
			}),
			Result: false,
		},
		"Matching organization and user": {
			Ctx: AddSessionToContext(context.Background(), Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			}),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, FilterUser("org1", "u1")(c.Ctx, "topic"))
		})
	}
}

func Test_AddSessionToContext(t *testing.T) {
	t.Parallel()

	session := Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}

	got, err := ExtractSessionFromContext(AddSessionToContext(context.Background(), session))
	require.NoError(t, err)
	assert.Equal(t, session, got)
}

func Test_ExtractSessionFromContext(t *testing.T) {
	t.Parallel()

	// a context without a session is the unauthenticated case.
	_, err := ExtractSessionFromContext(context.Background())
	testutil.AssertEqualError(t, httpserver.ErrNotAuthenticated, err)

	session := Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}

	got, err := ExtractSessionFromContext(AddSessionToContext(context.Background(), session))
	require.NoError(t, err)
	assert.Equal(t, session, got)
}
