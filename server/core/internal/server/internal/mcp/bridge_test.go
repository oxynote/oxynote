package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRunner is a minimal registry tool for exercising the bridge
// without the real registry: fixed outcome, optional error.
type stubRunner struct {
	// out is what Run reports as its output.
	out string

	// runErr fails Run when set.
	runErr error

	// calls records the arguments of every Run call.
	calls []json.RawMessage
}

// Run records the call and returns the configured outcome.
func (s *stubRunner) Run(_ context.Context, args json.RawMessage) (string, error) {
	s.calls = append(s.calls, args)

	return s.out, s.runErr
}

func Test_annotations(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Entry       tools.Entry
		ReadOnly    bool
		OpenWorld   bool
		Destructive *bool
		Idempotent  bool
	}{
		"Read tool": {
			Entry:    tools.Entry{},
			ReadOnly: true,
		},
		"Data-source tool reaches an open world": {
			Entry:     tools.Entry{Info: tools.Info{Traits: tools.Traits{DataSource: true}}},
			ReadOnly:  true,
			OpenWorld: true,
		},
		"Additive write tool": {
			Entry:       tools.Entry{Info: tools.Info{Traits: tools.Traits{Write: true}}},
			Destructive: new(bool),
		},
		"Destructive write tool": {
			Entry: tools.Entry{Info: tools.Info{Traits: tools.Traits{Write: true, Destructive: true}}},
			Destructive: func() *bool {
				v := true
				return &v
			}(),
		},
		"Idempotent write tool": {
			Entry:       tools.Entry{Info: tools.Info{Traits: tools.Traits{Write: true, Idempotent: true}}},
			Destructive: new(bool),
			Idempotent:  true,
		},
		// only a write says whether repeating it matters.
		"Idempotent trait on a read is not reported": {
			Entry:    tools.Entry{Info: tools.Info{Traits: tools.Traits{Idempotent: true}}},
			ReadOnly: true,
		},
		"Overwriting write tool is destructive to a client": {
			Entry: tools.Entry{Info: tools.Info{Traits: tools.Traits{Write: true, Overwrites: true}}},
			Destructive: func() *bool {
				v := true
				return &v
			}(),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := annotations(c.Entry)

			assert.Equal(t, c.ReadOnly, got.ReadOnlyHint)
			assert.Equal(t, c.Destructive, got.DestructiveHint)
			assert.Equal(t, c.Idempotent, got.IdempotentHint)
			require.NotNil(t, got.OpenWorldHint)
			assert.Equal(t, c.OpenWorld, *got.OpenWorldHint)
		})
	}
}

func Test_Handler_toolHandler(t *testing.T) {
	t.Parallel()

	hdl := &Handler{log: slog.New(slog.DiscardHandler)}

	cc := map[string]struct {
		Entry   tools.Entry
		Args    string
		IsError bool
		Text    string
	}{
		"Tool failure becomes an isError result": {
			Entry:   tools.Entry{Tool: &stubRunner{runErr: assert.AnError}},
			Args:    `{}`,
			IsError: true,
			Text:    assert.AnError.Error(),
		},
		"Output of a call is its text": {
			Entry: tools.Entry{Tool: &stubRunner{out: `{"ok":true}`}, Info: tools.Info{Traits: tools.Traits{Write: true}}},
			Args:  `{}`,
			Text:  `{"ok":true}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := hdl.toolHandler(c.Entry)(context.Background(), &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "stub",
					Arguments: json.RawMessage(c.Args),
				},
			})
			require.NoError(t, err)
			require.NotNil(t, res)

			assert.Equal(t, c.IsError, res.IsError)

			require.NotEmpty(t, res.Content)

			text, ok := res.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			assert.Equal(t, c.Text, text.Text)

			assert.Len(t, res.Content, 1)

			runner, ok := c.Entry.Tool.(*stubRunner)
			require.True(t, ok)
			assert.Equal(t, []json.RawMessage{json.RawMessage(c.Args)}, runner.calls)
		})
	}
}
