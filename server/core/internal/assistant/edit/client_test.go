package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

	cc := map[string]struct {
		Ops             []Operation
		BaseURL         string
		CloseEarly      bool
		TruncateBody    bool
		StatusCode      int
		ResponseBody    string
		ExpectedApplied int
		ExpectedErrors  []OpError
		ExpectedPath    string
		Err             error

		// NoRequest asserts Apply returned before any HTTP round trip.
		NoRequest bool
	}{
		"Expansion failure skips the request": {
			// an unknown canonical type fails at Expand time, before the
			// client builds a request at all.
			Ops:       []Operation{Append(block.Block{Type: "totally_not_a_real_type"})},
			Err:       assert.AnError,
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
			Err:          assert.AnError,
		},
		"Empty ops returns empty result without HTTP call": {
			Ops:             nil,
			StatusCode:      0,
			ExpectedApplied: 0,
			NoRequest:       true,
		},
		"Invalid base URL fails before the request": {
			Ops:       []Operation{Delete("x")},
			BaseURL:   "://bad",
			Err:       assert.AnError,
			NoRequest: true,
		},
		"Unencodable attrs fail at marshal time": {
			Ops: []Operation{
				UpdateAttrs("uid", map[string]any{"bad": make(chan int)}),
			},
			Err:       assert.AnError,
			NoRequest: true,
		},
		"Transport failure becomes a Go error": {
			Ops:        []Operation{Delete("x")},
			CloseEarly: true,
			Err:        assert.AnError,
		},
		"Unreadable error body still reports the status": {
			Ops:          []Operation{Delete("x")},
			StatusCode:   500,
			TruncateBody: true,
			Err:          assert.AnError,
		},
		"Malformed success response fails decoding": {
			Ops:          []Operation{Delete("x")},
			StatusCode:   200,
			ResponseBody: `{not json`,
			Err:          assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
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
				if c.TruncateBody {
					w.Header().Set("Content-Length", "100")
				}

				w.WriteHeader(c.StatusCode)

				_, err = w.Write([]byte(c.ResponseBody))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			baseURL := srv.URL
			if c.BaseURL != "" {
				baseURL = c.BaseURL
			}

			if c.CloseEarly {
				srv.Close()
			}

			client := NewClient(srv.Client(), baseURL)

			res, err := client.Apply(context.Background(), "doc-1", "branch-1", c.Ops)
			testutil.AssertEqualError(t, c.Err, err)

			if c.NoRequest {
				assert.Empty(t, captured.method, "Apply must return before any HTTP request is made")
			}

			if c.Err != nil {
				return
			}

			assert.Equal(t, c.ExpectedApplied, res.Applied)
			assert.Equal(t, c.ExpectedErrors, res.Errors)

			if len(c.Ops) > 0 {
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, c.ExpectedPath, captured.path)

				ops, ok := captured.body["operations"].([]any)
				require.True(t, ok, "request body should carry operations array")
				assert.Len(t, ops, len(c.Ops))
			}
		})
	}
}

func Test_Client_endpoint(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		BaseURL    string
		DocumentID string
		BranchID   string
		Expected   string
		Err        error
	}{
		"Plain IDs pass through untouched": {
			BaseURL:    "http://node:8081",
			DocumentID: "doc-1",
			BranchID:   "branch-1",
			Expected:   "http://node:8081/api/internal/documents/doc-1/branches/branch-1/operations",
		},
		"IDs needing escaping are escaped exactly once": {
			BaseURL:    "http://node:8081",
			DocumentID: "doc 1",
			BranchID:   "branch#1",
			Expected:   "http://node:8081/api/internal/documents/doc%201/branches/branch%231/operations",
		},
		"Invalid base URL fails parsing": {
			BaseURL:    "://bad",
			DocumentID: "doc-1",
			BranchID:   "branch-1",
			Err:        assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := NewClient(nil, c.BaseURL).endpoint(c.DocumentID, c.BranchID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Expected, got)
		})
	}
}
