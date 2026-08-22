package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _stubSessionJSON is a full internal-endpoint payload for the
// success paths.
const _stubSessionJSON = `{
	"userId": "user1",
	"organizationId": "org1",
	"clientId": "client1",
	"scopes": ["documents:read", "documents:write"],
	"expiresAt": 1755772800
}`

func Test_newVerifier(t *testing.T) {
	t.Parallel()

	// error
	client, mt := testutil.MockHTTP()
	mt.RegisterResponder(http.MethodGet, _testSessionURL, httpmock.NewStringResponder(http.StatusUnauthorized, ""))

	verifier := newVerifier(client, _testSessionURL)

	info, err := verifier(context.Background(), "tok", nil)
	assert.Error(t, err)
	assert.Nil(t, info)

	// success
	mt.RegisterResponder(http.MethodGet, _testSessionURL, httpmock.NewStringResponder(http.StatusOK, _stubSessionJSON))

	info, err = verifier(context.Background(), "tok", nil)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, []string{"documents:read", "documents:write"}, info.Scopes)
	assert.Equal(t, time.Unix(1755772800, 0), info.Expiration)
	assert.Equal(t, "user1", info.UserID)
	assert.Equal(t, Session{
		UserID:         "user1",
		OrganizationID: "org1",
		ClientID:       "client1",
		Scopes:         []string{"documents:read", "documents:write"},
		ExpiresAt:      1755772800,
	}, info.Extra[_sessionExtraKey])
}

func Test_sessionFromContext(t *testing.T) {
	t.Parallel()

	// runWith pushes the given TokenInfo through the go-sdk bearer
	// middleware — the only writer of its private context key — and
	// captures what sessionFromContext sees inside the request.
	runWith := func(t *testing.T, info *sdkauth.TokenInfo) (Session, error) {
		t.Helper()

		var (
			session Session
			err     error
		)

		mw := sdkauth.RequireBearerToken(
			func(context.Context, string, *http.Request) (*sdkauth.TokenInfo, error) {
				return info, nil
			},
			nil,
		)

		hdl := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			session, err = sessionFromContext(r.Context())
		}))

		req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)
		req.Header.Set("Authorization", "Bearer tok")

		hdl.ServeHTTP(httptest.NewRecorder(), req)

		return session, err
	}

	cc := map[string]struct {
		Context context.Context //nolint:containedctx // table input
		Info    *sdkauth.TokenInfo
		Result  Session
		Err     error
	}{
		"No token info in context": {
			Context: context.Background(),
			Err:     httpserver.ErrNotAuthenticated,
		},
		"Token info without an mcp session": {
			Info: &sdkauth.TokenInfo{
				Expiration: time.Now().Add(time.Hour),
			},
			Err: httpserver.ErrNotAuthenticated,
		},
		"Session present": {
			Info: &sdkauth.TokenInfo{
				Expiration: time.Now().Add(time.Hour),
				Extra: map[string]any{
					_sessionExtraKey: Session{
						UserID:         "user1",
						OrganizationID: "org1",
					},
				},
			},
			Result: Session{
				UserID:         "user1",
				OrganizationID: "org1",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				session Session
				err     error
			)

			if c.Context != nil {
				session, err = sessionFromContext(c.Context)
			} else {
				session, err = runWith(t, c.Info)
			}

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, session)
		})
	}
}

func Test_fetchSession(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Resp   httpmock.Responder
		Result Session
		Err    error
	}{
		"Error building the request": {
			URL: "://bad",
			Err: assert.AnError,
		},
		"Error returned by client.Do": {
			URL: _testSessionURL,
			Resp: httpmock.NewErrorResponder(
				assert.AnError,
			),
			Err: assert.AnError,
		},
		"Unauthorized response": {
			URL:  _testSessionURL,
			Resp: httpmock.NewStringResponder(http.StatusUnauthorized, ""),
			Err:  fmt.Errorf("%w: rejected by auth service", sdkauth.ErrInvalidToken),
		},
		"Unexpected status": {
			URL:  _testSessionURL,
			Resp: httpmock.NewStringResponder(http.StatusInternalServerError, ""),
			Err:  fmt.Errorf("mcp session endpoint returned status %d", http.StatusInternalServerError),
		},
		"Malformed JSON": {
			URL:  _testSessionURL,
			Resp: httpmock.NewStringResponder(http.StatusOK, "{"),
			Err:  assert.AnError,
		},
		"Incomplete session payload": {
			URL:  _testSessionURL,
			Resp: httpmock.NewStringResponder(http.StatusOK, `{"userId":"user1"}`),
			Err:  fmt.Errorf("%w: incomplete session payload", sdkauth.ErrInvalidToken),
		},
		"Successful validation": {
			URL: _testSessionURL,
			Resp: func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != "Bearer tok" {
					return httpmock.NewStringResponse(http.StatusBadRequest, ""), nil
				}

				return httpmock.NewStringResponse(http.StatusOK, _stubSessionJSON), nil
			},
			Result: Session{
				UserID:         "user1",
				OrganizationID: "org1",
				ClientID:       "client1",
				Scopes:         []string{"documents:read", "documents:write"},
				ExpiresAt:      1755772800,
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

			session, err := fetchSession(context.Background(), client, c.URL, "tok")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, session)
		})
	}
}
