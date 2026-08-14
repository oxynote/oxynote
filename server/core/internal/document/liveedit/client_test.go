package liveedit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document/aiblock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Client_Apply(t *testing.T) {
	t.Parallel()

	type capture struct {
		path   string
		method string
		body   map[string]any
	}

	tests := map[string]struct {
		Ops             []Operation
		StatusCode      int
		ResponseBody    string
		ExpectedApplied int
		ExpectedErrors  []OpError
		ExpectErr       bool
		ExpectedPath    string
	}{
		"Successful batch reports applied count": {
			Ops: []Operation{
				Append(aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
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
				Append(aiblock.Block{Type: aiblock.BlockParagraph, Text: "ok"}),
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

				w.WriteHeader(tc.StatusCode)

				_, err = w.Write([]byte(tc.ResponseBody))
				assert.NoError(t, err)
			}))
			t.Cleanup(srv.Close)

			c := NewClient(srv.Client(), srv.URL)

			res, err := c.Apply(context.Background(), "doc-1", "branch-1", tc.Ops)
			if tc.ExpectErr {
				require.Error(t, err)

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

func Test_Client_Apply_ExpansionError(t *testing.T) {
	t.Parallel()

	// An unknown canonical type fails at Expand time, before the
	// HTTP round-trip. Use a sentinel server to detect that no
	// request was sent.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.Client(), srv.URL)

	_, err := c.Apply(context.Background(), "doc", "branch", []Operation{
		Append(aiblock.Block{Type: "totally_not_a_real_type"}),
	})

	require.Error(t, err)
	assert.False(t, called, "expansion failure must short-circuit before any HTTP request")
}
