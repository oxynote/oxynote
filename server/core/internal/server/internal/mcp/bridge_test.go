package mcp

import (
	"context"
	"encoding/json"
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
}

// Run returns the configured outcome.
func (s *stubRunner) Run(context.Context, json.RawMessage) (string, error) {
	return s.out, s.runErr
}

func Test_annotations(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Entry       tools.Entry
		ReadOnly    bool
		OpenWorld   bool
		Destructive *bool
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
			require.NotNil(t, got.OpenWorldHint)
			assert.Equal(t, c.OpenWorld, *got.OpenWorldHint)
		})
	}
}

func Test_Handler_toolHandler(t *testing.T) {
	t.Parallel()

	hdl := &Handler{log: discardLog()}

	req := func(args string) *mcp.CallToolRequest {
		return &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "stub",
				Arguments: json.RawMessage(args),
			},
		}
	}

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
		"A call's output is its text": {
			Entry: tools.Entry{Tool: &stubRunner{out: `{"ok":true}`}, Info: tools.Info{Traits: tools.Traits{Write: true}}},
			Args:  `{}`,
			Text:  `{"ok":true}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := hdl.toolHandler(c.Entry)(context.Background(), req(c.Args))
			require.NoError(t, err)
			require.NotNil(t, res)

			assert.Equal(t, c.IsError, res.IsError)

			require.NotEmpty(t, res.Content)

			text, ok := res.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			assert.Equal(t, c.Text, text.Text)

			assert.Len(t, res.Content, 1)
		})
	}
}
