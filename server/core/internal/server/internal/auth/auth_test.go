package auth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Middleware(t *testing.T) {
	t.Parallel()

	type tcase struct {
		URL       string
		Resp      httpmock.Responder
		Cookies   []*http.Cookie
		NextCode  int
		AuthCalls int
		RespCode  int
		RespJSON  string
		Session   Session

		// After runs once the response is asserted, for a case that has to
		// inspect what the middleware sent upstream.
		After func(t *testing.T)
	}

	cc := map[string]tcase{
		"Error returned by http.NewRequestWithContext": {
			URL:      "http://test.com/\x7f",
			RespCode: http.StatusInternalServerError,
			RespJSON: `{"code":"general","message":"internal server error"}`,
		},
		"Error returned by http.Client.Do": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.ConnectionFailure,
			AuthCalls: 1,
			RespCode:  http.StatusInternalServerError,
			RespJSON:  `{"code":"general","message":"internal server error"}`,
		},
		"Non-200 status returned by http.Client.Do": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.NewStringResponder(http.StatusUnauthorized, ""),
			AuthCalls: 1,
			RespCode:  http.StatusUnauthorized,
			RespJSON:  `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Error returned by io.ReadAll": {
			URL: "http://test.com/get-session",
			Resp: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(iotest.ErrReader(assert.AnError)),
				}, nil
			},
			AuthCalls: 1,
			RespCode:  http.StatusInternalServerError,
			RespJSON:  `{"code":"general","message":"internal server error"}`,
		},
		"Error returned by json.Unmarshal": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.NewStringResponder(http.StatusOK, "{"),
			AuthCalls: 1,
			RespCode:  http.StatusInternalServerError,
			RespJSON:  `{"code":"general","message":"internal server error"}`,
		},
		"Empty user ID": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"","activeOrganizationId":"org1"}}`),
			AuthCalls: 1,
			RespCode:  http.StatusUnauthorized,
			RespJSON:  `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		// a session can lack an active organization before the user picks
		// one; every handler scopes by it, so it must read as unauthenticated.
		"Empty active organization ID": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"u1","activeOrganizationId":""}}`),
			AuthCalls: 1,
			RespCode:  http.StatusUnauthorized,
			RespJSON:  `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Successful authentication": {
			URL:       "http://test.com/get-session",
			Resp:      httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"u1","activeOrganizationId":"org1"}}`),
			NextCode:  http.StatusNoContent,
			AuthCalls: 1,
			RespCode:  http.StatusNoContent,
			Session: Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			},
		},
		"Cookies are forwarded to the auth service": func() tcase {
			var gotCookies []*http.Cookie

			return tcase{
				URL: "http://test.com/get-session",
				Resp: func(r *http.Request) (*http.Response, error) {
					gotCookies = r.Cookies()

					return httpmock.NewStringResponse(
						http.StatusOK,
						`{"session":{"userId":"u1","activeOrganizationId":"org1"}}`,
					), nil
				},
				Cookies: []*http.Cookie{
					{Name: "session_token", Value: "tkn", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode},
					{Name: "other", Value: "val", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode},
				},
				NextCode:  http.StatusNoContent,
				AuthCalls: 1,
				RespCode:  http.StatusNoContent,
				Session: Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				},
				After: func(t *testing.T) {
					require.Len(t, gotCookies, 2)
					assert.Equal(t, "session_token", gotCookies[0].Name)
					assert.Equal(t, "tkn", gotCookies[0].Value)
					assert.Equal(t, "other", gotCookies[1].Name)
					assert.Equal(t, "val", gotCookies[1].Value)
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := testutil.MockHTTP()

			// httpmock counts only calls a responder handles. The fallback
			// makes a call to an unregistered URL count too.
			mt.RegisterNoResponder(httpmock.ConnectionFailure)

			if c.Resp != nil {
				mt.RegisterResponder(http.MethodGet, c.URL, c.Resp)
			}

			var (
				nextCalls  int
				gotSession Session
			)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalls++

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

			for _, ck := range c.Cookies {
				req.AddCookie(ck)
			}

			rec := httptest.NewRecorder()

			hdl.ServeHTTP(rec, req)

			testutil.AssertResponse(t, rec, c.RespCode, c.RespJSON)
			assert.Equal(t, c.AuthCalls, mt.GetTotalCallCount())

			if c.NextCode == 0 {
				assert.Zero(t, nextCalls)
				return
			}

			assert.Equal(t, 1, nextCalls)
			assert.Equal(t, c.Session, gotSession)

			if c.After != nil {
				c.After(t)
			}
		})
	}
}

func Test_HasActiveOrganization(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		UserID         string
		OrganizationID string
		Result         bool
	}{
		"User and organization": {UserID: "u1", OrganizationID: "org1", Result: true},
		"Empty user":            {OrganizationID: "org1"},
		"Empty organization":    {UserID: "u1"},
		"Both empty":            {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, HasActiveOrganization(c.UserID, c.OrganizationID))
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

func Test_FilterOrganizationDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	session := AddSessionToContext(context.Background(), Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})

	cc := map[string]struct {
		Ctx    context.Context
		Result bool
	}{
		"No session in context": {
			Ctx: wsserver.NewTopicParamsContext(context.Background(), map[string]string{"documentId": docID.String()}),
		},
		"No document in context": {
			Ctx: session,
		},
		"Another document": {
			Ctx: wsserver.NewTopicParamsContext(session, map[string]string{"documentId": xid.New().String()}),
		},
		"Another organization": {
			Ctx: wsserver.NewTopicParamsContext(AddSessionToContext(context.Background(), Session{
				UserID:               "u1",
				ActiveOrganizationID: "org2",
			}), map[string]string{"documentId": docID.String()}),
		},
		"Matching organization and document": {
			Ctx:    wsserver.NewTopicParamsContext(session, map[string]string{"documentId": docID.String()}),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, FilterOrganizationDocument("org1", docID)(c.Ctx, "topic"))
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

func Test_RequireSession(t *testing.T) {
	t.Parallel()

	session := Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}

	cc := map[string]struct {
		Ctx      context.Context
		Result   Session
		OK       bool
		RespCode int
		RespJSON string
	}{
		"No session in context": {
			Ctx:      context.Background(),
			RespCode: http.StatusUnauthorized,
			RespJSON: `{"code":"account.not_authenticated","message":"not authenticated"}`,
		},
		"Session present": {
			Ctx:      AddSessionToContext(context.Background(), session),
			Result:   session,
			OK:       true,
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			req = req.WithContext(c.Ctx)
			rec := httptest.NewRecorder()

			got, ok := RequireSession(slog.New(slog.DiscardHandler), rec, req)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, got)
			testutil.AssertResponse(t, rec, c.RespCode, c.RespJSON)
		})
	}
}

func Test_ExtractSessionFromContext(t *testing.T) {
	t.Parallel()

	session := Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}

	cc := map[string]struct {
		Ctx    context.Context
		Result Session
		Err    error
	}{
		"No session in context": {
			Ctx: context.Background(),
			Err: httpserver.ErrNotAuthenticated,
		},
		"Session present": {
			Ctx:    AddSessionToContext(context.Background(), session),
			Result: session,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := ExtractSessionFromContext(c.Ctx)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}
