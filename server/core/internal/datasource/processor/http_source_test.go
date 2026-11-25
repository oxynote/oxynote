package processor

import (
	"bytes"
	"cmp"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testPath is the path the httpClient tests request.
const _testPath = "/api/v1/test"

// prepHTTPInput returns an input pointing at the given URL, or at a fake
// server serving h when the URL is empty. The cleanup drops the shared
// transport's keep-alive connections so goleak stays clean.
func prepHTTPInput(t *testing.T, target string, h http.Handler) *InputMock {
	t.Helper()

	if target == "" {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)

		target = srv.URL
	}

	t.Cleanup(func() {
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
	})

	return &InputMock{
		URLFunc: func() string { return target },
	}
}

func Test_httpClient_fetch(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  string
		Err     error
	}{
		"Error returned by httpClient.authClient": {
			Inp: &InputMock{
				CredentialsFunc: func() credentials.Credentials { return stubCredentials(`{`) },
			},
			Err: assert.AnError,
		},
		"Error returned by httpClient.doRequest": {
			Inp: &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Err: assert.AnError,
		},
		"Unauthorized": {
			Handler: testutil.JSONHandler(_testPath, http.StatusUnauthorized, `no org id`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Endpoint not found": {
			Handler: testutil.JSONHandler("/elsewhere", http.StatusOK, `{}`),
			Err:     _errEndpointNotFound,
		},
		"Redirect is not followed": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != _testPath {
					assert.Fail(t, "the redirect was followed")
				}

				http.Redirect(w, r, "/internal/admin", http.StatusFound)
			}),
			Err: _errRedirected,
		},
		"Rate limited": {
			Handler: testutil.JSONHandler(_testPath, http.StatusTooManyRequests, `{"message":"too many requests"}`),
			Err:     _errRateLimited,
		},
		"Server error": {
			Handler: testutil.JSONHandler(_testPath, http.StatusGatewayTimeout, "request timed out\n"),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Rejected request in plain text": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, "parse error\n"),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Rejected request wrapped by a proxy": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, `{"message":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Rejected request in the Prometheus shape": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, `{"status":"error","error":"bad step"}`),
			Err:     NewInvalidQueryError("bad step"),
		},
		"Rejected request as JSON without a message": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, `{"status":"error"}`),
			Err:     NewInvalidQueryError(`{"status":"error"}`),
		},
		"Rejected request without a body": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, ``),
			Err:     NewInvalidQueryError("the data source rejected the query"),
		},
		"Rejected request longer than the cap": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, strings.Repeat("x", _maxErrorMessageSize+10)),
			Err:     NewInvalidQueryError(strings.Repeat("x", _maxErrorMessageSize) + "…"),
		},
		"Proxy message longer than the cap": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, `{"message":"`+strings.Repeat("y", _maxErrorMessageSize+10)+`"}`),
			Err:     NewInvalidQueryError(strings.Repeat("y", _maxErrorMessageSize) + "…"),
		},
		"Successful request": {
			Handler: testutil.JSONHandler(_testPath, http.StatusOK, `{"data":["env"]}`),
			Result:  `{"data":["env"]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp
			if inp == nil {
				inp = prepHTTPInput(t, "", c.Handler)
			} else {
				t.Cleanup(http.DefaultTransport.(*http.Transport).CloseIdleConnections)
			}

			hc := &httpClient{inp: inp}

			body, err := hc.fetch(context.Background(), http.MethodGet, _testPath, nil, nil)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, string(body))
		})
	}
}

func Test_httpClient_fetchJSON(t *testing.T) {
	cc := map[string]struct {
		Handler http.Handler
		Result  map[string][]string
		Err     error
	}{
		"Error returned by httpClient.fetchRaw": {
			Handler: testutil.JSONHandler(_testPath, http.StatusBadRequest, "parse error\n"),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Body that does not decode": {
			Handler: testutil.JSONHandler(_testPath, http.StatusOK, `{`),
			Err:     assert.AnError,
		},
		"Successful request": {
			Handler: testutil.JSONHandler(_testPath, http.StatusOK, `{"data":["env"]}`),
			Result:  map[string][]string{"data": {"env"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hc := &httpClient{inp: prepHTTPInput(t, "", c.Handler)}

			res, err := hc.fetchJSON[map[string][]string](context.Background(), _testPath, nil, nil)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_httpClient_doRequest(t *testing.T) {
	cc := map[string]struct {
		URL     string
		Handler http.Handler
		Method  string
		Status  int
		Body    string
		Err     error
	}{
		"Params go in a POST body": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Empty(t, r.URL.RawQuery)
				assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
				assert.NoError(t, r.ParseForm())
				assert.Equal(t, "1", r.PostForm.Get("start"))

				_, err := w.Write([]byte("body"))
				assert.NoError(t, err)
			}),
			Method: http.MethodPost,
			Status: http.StatusOK,
			Body:   "body",
		},
		"Invalid URL": {
			URL: "://",
			Err: assert.AnError,
		},
		"Unreachable server": {
			URL: "http://127.0.0.1:1",
			Err: assert.AnError,
		},
		"Answer over the size cap": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write(bytes.Repeat([]byte("x"), _maxHTTPResponseSize+1))
				assert.NoError(t, err)
			}),
			Err: ErrResponseTooLarge,
		},
		"Body cut short": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "10")

				_, err := w.Write([]byte("abc"))
				assert.NoError(t, err)
			}),
			Err: assert.AnError,
		},
		"Path prefix params and header reach the server": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/proxy"+_testPath, r.URL.Path)
				assert.Equal(t, "1", r.URL.Query().Get("start"))
				assert.Equal(t, "yes", r.Header.Get("X-Test"))
				w.WriteHeader(http.StatusTeapot)

				_, err := w.Write([]byte("body"))
				assert.NoError(t, err)
			}),
			Status: http.StatusTeapot,
			Body:   "body",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := prepHTTPInput(t, c.URL, c.Handler)
			if c.Handler != nil {
				base := inp.URL()
				inp.URLFunc = func() string { return base + "/proxy" }
			}

			hc := &httpClient{inp: inp}

			status, body, err := hc.doRequest(
				context.Background(),
				&http.Client{},
				cmp.Or(c.Method, http.MethodGet),
				_testPath,
				map[string][]string{"start": {"1"}},
				http.Header{"X-Test": {"yes"}},
			)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, status)
			assert.Equal(t, c.Body, string(body))
		})
	}
}

func Test_httpClient_authClient(t *testing.T) {
	cc := map[string]struct {
		URL         string
		Credentials credentials.Credentials

		// TLS indicates whether the data source is served over TLS, with
		// its own certificate as the credentials' CA.
		TLS bool

		// Auth is the Authorization header the data source receives.
		Auth string

		Err error
	}{
		"Credentials that do not decode": {
			URL:         "http://loki.test",
			Credentials: stubCredentials(`{`),
			Err:         assert.AnError,
		},
		"Credentials without a kind": {
			URL:         "http://loki.test",
			Credentials: stubCredentials(`{"username":"user","password":"pass"}`),
			Err:         credentials.ErrInvalidKind,
		},
		"Error returned by ownTransport.roundTripper": {
			URL:         "http://loki.test",
			Credentials: caCredentials("not a certificate"),
			Err:         credentials.ErrInvalidCACertificate,
		},
		"No credentials": {},
		"Basic credentials": {
			Credentials: stubCredentials(`{"kind":"basic","username":"user","password":"pass"}`),
			Auth:        "Basic dXNlcjpwYXNz",
		},
		"Token credentials": {
			Credentials: stubCredentials(`{"kind":"token","token":"secret"}`),
			Auth:        "Bearer secret",
		},
		// credentials holding only a CA certificate authenticate nobody.
		"Only a CA certificate": {
			TLS: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var auth atomic.Value

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth.Store(r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusNoContent)
			})

			var inp *InputMock

			switch {
			case c.URL != "":
				inp = &InputMock{URLFunc: func() string { return c.URL }}
			case c.TLS:
				inp = prepTLSInput(t, h, "")
			default:
				inp = prepHTTPInput(t, "", h)
			}

			if !c.TLS {
				inp.CredentialsFunc = func() credentials.Credentials { return c.Credentials }
			}

			hc := &httpClient{inp: inp}

			t.Cleanup(hc.transport.close)

			client, err := hc.authClient()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, client.CheckRedirect)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, inp.URL(), http.NoBody)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, c.Auth, auth.Load())
		})
	}
}
