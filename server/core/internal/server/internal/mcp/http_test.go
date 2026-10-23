package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.IgnoreBleveWorkers())
}

// _testSessionURL is the internal MCP session endpoint the mocked
// transport answers for.
const _testSessionURL = "http://auth.test/api/internal/mcp/session"

// _testResourceURL is the canonical MCP resource URL used across tests.
const _testResourceURL = "http://front.test/core/api/mcp"

// _serverBuilds is how many times the stateless transport builds the
// server for one request: once to check the protocol version and once to
// serve the call.
const _serverBuilds = 2

// stubSearchQueue satisfies the tools' SearchQueue. Its methods are never
// called.
type stubSearchQueue struct {
	tools.SearchQueue
}

// stubSet builds a real tool set over the given tools DB mock, so the
// bridged tools behave exactly as the assistant's would.
func stubSet(db *toolsMock.DB) *tools.Set {
	return tools.New(tools.NewDeps(
		slog.New(slog.DiscardHandler),
		db,
		&toolsMock.Searcher{},
		stubSearchQueue{},
		nil,
		nil,
		nil,
		nil,
		nil,
		&toolsMock.EditApplier{},
		nil,
		nil,
		nil,
		"org1",
		"user1",
	))
}

// stubSession answers the session endpoint with a valid session
// carrying the given scopes.
func stubSession(t *testing.T, scopes []string) httpmock.Responder {
	t.Helper()

	scopesJSON, err := json.Marshal(scopes)
	require.NoError(t, err)

	return httpmock.NewStringResponder(
		http.StatusOK,
		fmt.Sprintf(
			`{"userId":"user1","organizationId":"org1","clientId":"client1","scopes":%s,"expiresAt":32503680000}`,
			scopesJSON,
		),
	)
}

// prepHandler builds a Handler whose bearer middleware talks to a
// mocked session endpoint answering with the given responder. The
// returned transport counts the session endpoint calls.
func prepHandler(
	t *testing.T,
	session httpmock.Responder,
	man *ManagerMock,
	db *DBMock,
) (*Handler, *httpmock.MockTransport) {
	t.Helper()

	client, mt := testutil.MockHTTP()
	mt.RegisterResponder(http.MethodGet, _testSessionURL, session)

	hdl, err := NewHandler(slog.New(slog.DiscardHandler), man, db, client, Options{
		SessionURL:  _testSessionURL,
		ResourceURL: _testResourceURL,
	})
	require.NoError(t, err)

	return hdl, mt
}

// rpc posts one JSON-RPC message to the handler with a bearer token and
// returns the HTTP status plus the decoded JSON-RPC response payload
// pulled out of the SSE stream (nil when the response carried no data).
func rpc(t *testing.T, hdl http.Handler, body string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer tok")

	rec := httptest.NewRecorder()
	hdl.ServeHTTP(rec, req)

	return rec.Code, ssePayload(t, rec.Body.String())
}

// ssePayload decodes the first JSON-RPC message of an SSE stream, or
// returns nil when the stream carried no data.
func ssePayload(t *testing.T, body string) map[string]any {
	t.Helper()

	for line := range strings.SplitSeq(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		var payload map[string]any

		require.NoError(t, json.Unmarshal([]byte(data), &payload))

		return payload
	}

	return nil
}

func Test_Options_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Options Options
		Err     error
	}{
		"Missing session url": {
			Err: errors.New("missing mcp session url"),
		},
		"Missing resource url": {
			Options: Options{SessionURL: _testSessionURL},
			Err:     errors.New("missing mcp resource url"),
		},
		"Valid options": {
			Options: Options{SessionURL: _testSessionURL, ResourceURL: _testResourceURL},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Options.Validate())
		})
	}
}

func Test_Options_Enabled(t *testing.T) {
	t.Parallel()

	assert.False(t, Options{SessionURL: _testSessionURL}.Enabled())
	assert.True(t, Options{ResourceURL: _testResourceURL}.Enabled())
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Options Options
		Err     error
	}{
		"Invalid options": {
			Err: errors.New("missing mcp session url"),
		},
		"Underivable metadata url": {
			Options: Options{SessionURL: _testSessionURL, ResourceURL: "://bad"},
			Err: fmt.Errorf(
				"deriving resource metadata url: %w",
				&url.Error{Op: "parse", URL: "://bad", Err: errors.New("missing protocol scheme")},
			),
		},
		"Successful creation": {
			Options: Options{SessionURL: _testSessionURL, ResourceURL: _testResourceURL},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := &ManagerMock{}
			db := &DBMock{}

			hdl, err := NewHandler(slog.New(slog.DiscardHandler), man, db, http.DefaultClient, c.Options)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, hdl)

				return
			}

			require.NotNil(t, hdl)

			assert.NotNil(t, hdl.log)
			assert.Same(t, man, hdl.man)
			assert.Same(t, db, hdl.db)
			assert.NotNil(t, hdl.handler)
		})
	}
}

func Test_Handler_ServeHTTP(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Session                httpmock.Responder
		Header                 http.Header
		Body                   string
		Code                   int
		Check                  func(t *testing.T, rec *httptest.ResponseRecorder)
		SessionCalls           int
		ToolSetCalls           int
		FetchDocumentTreeCalls int
	}{
		"Missing bearer token": {
			Session: stubSession(t, []string{ScopeDocumentRead}),
			Body:    "{}",
			Code:    http.StatusUnauthorized,
			Check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				assert.Contains(
					t,
					rec.Header().Get("WWW-Authenticate"),
					`resource_metadata="http://front.test/.well-known/oauth-protected-resource/core/api/mcp"`,
				)
			},
		},
		"Rejected bearer token": {
			Session: httpmock.NewStringResponder(http.StatusUnauthorized, ""),
			Header:  http.Header{"Authorization": {"Bearer tok"}},
			Body:    "{}",
			Code:    http.StatusUnauthorized,
			Check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "resource_metadata")
			},
			SessionCalls: 1,
		},
		"Successful initialize": {
			Session: stubSession(t, []string{ScopeDocumentRead}),
			Header: http.Header{
				"Content-Type":  {"application/json"},
				"Accept":        {"application/json, text/event-stream"},
				"Authorization": {"Bearer tok"},
			},
			Body: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
			Code: http.StatusOK,
			Check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				payload := ssePayload(t, rec.Body.String())
				require.NotNil(t, payload)

				result, ok := payload["result"].(map[string]any)
				require.True(t, ok, "payload: %v", payload)

				info, ok := result["serverInfo"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "oxynote", info["name"])

				// the instructions are how a connecting agent learns what it is
				// working with; an initialize without them leaves the client
				// with bare tool descriptions.
				instructions, ok := result["instructions"].(string)
				require.True(t, ok, "instructions: %v", result["instructions"])
				assert.Contains(t, instructions, "Oxynote")
				assert.Contains(t, instructions, "## Content")
			},
			SessionCalls:           1,
			ToolSetCalls:           _serverBuilds,
			FetchDocumentTreeCalls: _serverBuilds,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := &ManagerMock{
				ToolSetFunc: func(string, string) *tools.Set {
					return stubSet(&toolsMock.DB{})
				},
			}
			db := &DBMock{}
			hdl, mt := prepHandler(t, c.Session, man, db)

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Body))
			maps.Copy(req.Header, c.Header)

			rec := httptest.NewRecorder()
			hdl.ServeHTTP(rec, req)

			require.Equal(t, c.Code, rec.Code)
			c.Check(t, rec)

			assert.Equal(t, c.SessionCalls, mt.GetTotalCallCount())

			toolSetCalls := man.ToolSetCalls()
			require.Len(t, toolSetCalls, c.ToolSetCalls)

			for _, call := range toolSetCalls {
				assert.Equal(t, "org1", call.OrgID)
				assert.Equal(t, "user1", call.UserID)
			}

			assert.Len(t, db.FetchDocumentTreeCalls(), c.FetchDocumentTreeCalls)
		})
	}
}

func Test_resourceMetadataURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Resource string
		Result   string
		Err      error
	}{
		"Malformed url": {
			Resource: "://bad",
			Err:      assert.AnError,
		},
		"Relative url": {
			Resource: "/core/api/mcp",
			Err:      assert.AnError,
		},
		"Resource with a path": {
			Resource: "http://front.test/core/api/mcp",
			Result:   "http://front.test/.well-known/oauth-protected-resource/core/api/mcp",
		},
		"Resource at the origin root": {
			Resource: "http://front.test",
			Result:   "http://front.test/.well-known/oauth-protected-resource",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := resourceMetadataURL(c.Resource)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}
