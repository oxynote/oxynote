package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_toolInfo asserts every tool still presents the model the
// exact description it did before the tools were split into their own
// files. The golden file was captured from the previous implementation.
//
// This is the load-bearing test of the restructure: seventeen schemas
// were retyped by hand, and a dropped required field or a truncated
// description degrades the assistant in a way no compile error catches.
func Test_toolInfo(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile("testdata/tool_schemas.golden")
	require.NoError(t, err)

	s := New(NewInput(slog.New(slog.DiscardHandler), nil, nil, nil, nil, "org", "user"))

	got := map[string]any{}

	for _, bt := range s.Tools() {
		info, ierr := bt.Info(context.Background())
		require.NoError(t, ierr)

		raw, merr := json.Marshal(info)
		require.NoError(t, merr)

		var v any
		require.NoError(t, json.Unmarshal(raw, &v))

		got[info.Name] = v
	}

	data, err := json.Marshal(got)
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(data))
}

func Test_stringProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{"type": "string", "description": "a doc"}, stringProp("a doc"))
}

func Test_documentIDProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		map[string]any{"document_id": map[string]any{"type": "string", "description": "a doc"}},
		documentIDProp("a doc"))
}
