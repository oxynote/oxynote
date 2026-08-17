package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewClient(t *testing.T) {
	t.Parallel()

	// nil http client falls back to the default client
	c := NewClient(nil, "http://test.com")
	require.NotNil(t, c)
	assert.Same(t, http.DefaultClient, c.httpClient)
	assert.Equal(t, "http://test.com", c.baseURL)

	// explicit http client is kept
	hc := &http.Client{}
	c = NewClient(hc, "http://test.com")
	assert.Same(t, hc, c.httpClient)
}

func Test_Client_Apply(t *testing.T) {
	t.Parallel()

	type capture struct {
		path   string
		method string
		body   map[string]any
	}

	tests := map[string]struct {
		Ops             []Operation
		BaseURL         string
		CloseEarly      bool
		TruncateBody    bool
		StatusCode      int
		ResponseBody    string
		ExpectedApplied int
		ExpectedErrors  []OpError
		ExpectErr       bool
		ExpectedPath    string

		// NoRequest asserts the operation failed before any HTTP round trip.
		NoRequest bool
	}{
		"Expansion failure skips the request": {
			// an unknown canonical type fails at Expand time, before the
			// client builds a request at all.
			Ops:       []Operation{Append(block.Block{Type: "totally_not_a_real_type"})},
			ExpectErr: true,
			NoRequest: true,
		},
		"Successful batch reports applied count": {
			Ops: []Operation{
				Append(block.Block{Type: block.BlockParagraph, Text: "hi"}),
				Delete("uid-1"),
			},
			StatusCode:      200,
			ResponseBody:    `{"applied": 2, "errors": []}`,
			ExpectedApplied: 2,
			ExpectedErrors:  []OpError{},
			ExpectedPath:    "/api/internal/documents/doc-1/branches/branch-1/operations",
		},
		"Partial failure surfaces per-op errors": {
			Ops: []Operation{
				Delete("missing"),
				Append(block.Block{Type: block.BlockParagraph, Text: "ok"}),
			},
			StatusCode:      200,
			ResponseBody:    `{"applied": 1, "errors": [{"index": 0, "message": "block_uid not found: missing"}]}`,
			ExpectedApplied: 1,
			ExpectedErrors: []OpError{
				{Index: 0, Message: "block_uid not found: missing"},
			},
			ExpectedPath: "/api/internal/documents/doc-1/branches/branch-1/operations",
		},
		"HTTP error becomes a Go error": {
			Ops: []Operation{
				Delete("x"),
			},
			StatusCode:   500,
			ResponseBody: `{"error": "boom"}`,
			ExpectErr:    true,
		},
		"Empty ops returns empty result without HTTP call": {
			Ops:             nil,
			StatusCode:      0,
			ExpectedApplied: 0,
		},
		"Invalid base URL fails before the request": {
			Ops:       []Operation{Delete("x")},
			BaseURL:   "://bad",
			ExpectErr: true,
		},
		"Unencodable attrs fail at marshal time": {
			Ops: []Operation{
				UpdateAttrs("uid", map[string]any{"bad": make(chan int)}),
			},
			ExpectErr: true,
		},
		"Transport failure becomes a Go error": {
			Ops:        []Operation{Delete("x")},
			CloseEarly: true,
			ExpectErr:  true,
		},
		"Unreadable error body still reports the status": {
			Ops:          []Operation{Delete("x")},
			StatusCode:   500,
			TruncateBody: true,
			ExpectErr:    true,
		},
		"Malformed success response fails decoding": {
			Ops:          []Operation{Delete("x")},
			StatusCode:   200,
			ResponseBody: `{not json`,
			ExpectErr:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var captured capture

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured.path = r.URL.Path
				captured.method = r.Method

				raw, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.NoError(t, json.Unmarshal(raw, &captured.body))

				// declare a longer body than is written so the
				// client's error-body read fails mid-stream
				if tc.TruncateBody {
					w.Header().Set("Content-Length", "100")
				}

				w.WriteHeader(tc.StatusCode)

				_, err = w.Write([]byte(tc.ResponseBody))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			baseURL := srv.URL
			if tc.BaseURL != "" {
				baseURL = tc.BaseURL
			}

			if tc.CloseEarly {
				srv.Close()
			}

			c := NewClient(srv.Client(), baseURL)

			res, err := c.Apply(context.Background(), "doc-1", "branch-1", tc.Ops)
			if tc.ExpectErr {
				require.Error(t, err)

				if tc.NoRequest {
					assert.Empty(t, captured.method, "expansion failure must short-circuit before any HTTP request")
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedApplied, res.Applied)
			assert.Equal(t, tc.ExpectedErrors, res.Errors)

			if len(tc.Ops) > 0 {
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, tc.ExpectedPath, captured.path)

				ops, ok := captured.body["operations"].([]any)
				require.True(t, ok, "request body should carry operations array")
				assert.Len(t, ops, len(tc.Ops))
			}
		})
	}
}
