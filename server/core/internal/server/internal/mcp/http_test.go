package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	goleak.VerifyTestMain(m)
}

// _testSessionURL is the internal MCP session endpoint the mocked
// transport answers for.
const _testSessionURL = "http://auth.test/api/internal/mcp/session"

// _testResourceURL is the canonical MCP resource URL used across tests.
const _testResourceURL = "http://front.test/core/api/mcp"

// discardLog returns a logger that swallows all output.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// stubToolsDB builds the tools-package DB mock every test tool set
// runs against.
func stubToolsDB() *toolsMock.DB {
	return &toolsMock.DB{}
}

// stubSet builds a real tool set over the given tools DB mock, so the
// bridged tools behave exactly as the assistant's would.
func stubSet(db *toolsMock.DB) *tools.Set {
	return tools.New(tools.NewDeps(
		discardLog(),
		db,
		&toolsMock.Searcher{},
		&toolsMock.EditApplier{},
		nil,
		nil,
		"org1",
		"user1",
	))
}

// prepHandler builds a Handler whose bearer middleware talks to a
// mocked session endpoint answering with the given scopes, and whose
// tool set runs over the given tools DB mock.
func prepHandler(t *testing.T, scopes []string, toolsDB *toolsMock.DB, db *DBMock) *Handler {
	t.Helper()

	client, mt := testutil.MockHTTP()

	scopesJSON, err := json.Marshal(scopes)
	require.NoError(t, err)

	mt.RegisterResponder(http.MethodGet, _testSessionURL, httpmock.NewStringResponder(
		http.StatusOK,
		fmt.Sprintf(
			`{"userId":"user1","organizationId":"org1","clientId":"client1","scopes":%s,"expiresAt":32503680000}`,
			scopesJSON,
		),
	))

	man := &ManagerMock{
		ToolSetFunc: func(string, string) *tools.Set {
			return stubSet(toolsDB)
		},
	}

	hdl, err := NewHandler(discardLog(), man, db, client, Options{
		SessionURL:  _testSessionURL,
		ResourceURL: _testResourceURL,
	})
	require.NoError(t, err)

	return hdl
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

	for line := range strings.SplitSeq(rec.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		var payload map[string]any

		require.NoError(t, json.Unmarshal([]byte(data), &payload))

		return rec.Code, payload
	}

	return rec.Code, nil
}

// listedTools runs tools/list through the handler and returns each
// definition exactly as it went over the wire, keyed by tool name.
func listedTools(t *testing.T, hdl http.Handler) map[string]map[string]any {
	t.Helper()

	code, payload := rpc(t, hdl, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, payload)

	result, ok := payload["result"].(map[string]any)
	require.True(t, ok, "payload: %v", payload)

	rawTools, ok := result["tools"].([]any)
	require.True(t, ok)

	defs := make(map[string]map[string]any, len(rawTools))

	for _, rt := range rawTools {
		tm, tok := rt.(map[string]any)
		require.True(t, tok)

		name, nok := tm["name"].(string)
		require.True(t, nok)

		defs[name] = tm
	}

	return defs
}

func Test_Options_Validate(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, Options{}.Validate(), "missing mcp session url")
	assert.EqualError(t, Options{SessionURL: _testSessionURL}.Validate(), "missing mcp resource url")
	assert.NoError(t, Options{SessionURL: _testSessionURL, ResourceURL: _testResourceURL}.Validate())
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	// error: invalid options
	hdl, err := NewHandler(discardLog(), &ManagerMock{}, &DBMock{}, http.DefaultClient, Options{})
	require.Error(t, err)
	assert.Nil(t, hdl)

	// error: underivable metadata url
	hdl, err = NewHandler(discardLog(), &ManagerMock{}, &DBMock{}, http.DefaultClient, Options{
		SessionURL:  _testSessionURL,
		ResourceURL: "://bad",
	})
	require.ErrorContains(t, err, "deriving resource metadata url")
	assert.Nil(t, hdl)

	// success
	man := &ManagerMock{}
	db := &DBMock{}

	hdl, err = NewHandler(discardLog(), man, db, http.DefaultClient, Options{
		SessionURL:  _testSessionURL,
		ResourceURL: _testResourceURL,
	})
	require.NoError(t, err)
	require.NotNil(t, hdl)

	assert.NotNil(t, hdl.log)
	assert.Same(t, man, hdl.man)
	assert.Same(t, db, hdl.db)
	assert.NotNil(t, hdl.handler)
}

func Test_Handler_ServeHTTP(t *testing.T) {
	t.Parallel()

	t.Run("Missing bearer token", func(t *testing.T) {
		t.Parallel()

		hdl := prepHandler(t, []string{ScopeRead}, stubToolsDB(), &DBMock{})

		req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader("{}"))
		rec := httptest.NewRecorder()

		hdl.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(
			t,
			rec.Header().Get("WWW-Authenticate"),
			`resource_metadata="http://front.test/.well-known/oauth-protected-resource/core/api/mcp"`,
		)
	})

	t.Run("Rejected bearer token", func(t *testing.T) {
		t.Parallel()

		client, mt := testutil.MockHTTP()
		mt.RegisterResponder(http.MethodGet, _testSessionURL, httpmock.NewStringResponder(http.StatusUnauthorized, ""))

		hdl, err := NewHandler(discardLog(), &ManagerMock{}, &DBMock{}, client, Options{
			SessionURL:  _testSessionURL,
			ResourceURL: _testResourceURL,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer tok")

		rec := httptest.NewRecorder()
		hdl.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "resource_metadata")
	})

	t.Run("Successful initialize", func(t *testing.T) {
		t.Parallel()

		hdl := prepHandler(t, []string{ScopeRead}, stubToolsDB(), &DBMock{})

		code, payload := rpc(
			t,
			hdl,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		)
		require.Equal(t, http.StatusOK, code)
		require.NotNil(t, payload)

		result, ok := payload["result"].(map[string]any)
		require.True(t, ok, "payload: %v", payload)

		info, ok := result["serverInfo"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "oxynote", info["name"])
	})
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
