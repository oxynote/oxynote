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

	// docs is what Run reports as the documents it changed.
	docs []string

	// runErr fails Run when set.
	runErr error
}

// Run returns the configured outcome.
func (s *stubRunner) Run(context.Context, json.RawMessage) (tools.Result, error) {
	return tools.Result{Output: s.out, Documents: s.docs}, s.runErr
}

// entryFor finds the named tool's entry in a fresh stubbed set.
func entryFor(t *testing.T, name tools.Name) tools.Entry {
	t.Helper()

	for _, e := range stubSet(stubToolsDB()).Entries() {
		if e.Name == name {
			return e
		}
	}

	t.Fatalf("tool %s not found", name)

	return tools.Entry{}
}

func Test_annotations(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Entry       tools.Entry
		ReadOnly    bool
		Destructive *bool
	}{
		"Read tool": {
			Entry:    tools.Entry{},
			ReadOnly: true,
		},
		"Additive write tool": {
			Entry:       tools.Entry{Traits: tools.Traits{Write: true}},
			Destructive: new(bool),
		},
		"Destructive write tool": {
			Entry: tools.Entry{Traits: tools.Traits{Write: true, Destructive: true}},
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
			assert.False(t, *got.OpenWorldHint)
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
		Entry    tools.Entry
		Args     string
		IsError  bool
		Text     string
		LinkURIs []string
	}{
		"Tool failure becomes an isError result": {
			Entry:   tools.Entry{Tool: &stubRunner{runErr: assert.AnError}},
			Args:    `{}`,
			IsError: true,
			Text:    assert.AnError.Error(),
		},
		"A call that changed nothing carries no link": {
			Entry: tools.Entry{Tool: &stubRunner{out: `{"ok":true}`}},
			Args:  `{"document_id":"doc1"}`,
			Text:  `{"ok":true}`,
		},
		"A call links the document it changed": {
			Entry:    tools.Entry{Tool: &stubRunner{out: `{"ok":true}`, docs: []string{"doc1"}}, Traits: tools.Traits{Write: true}},
			Args:     `{"document_id":"doc1"}`,
			Text:     `{"ok":true}`,
			LinkURIs: []string{_resourceURIPrefix + "doc1"},
		},
		"A call links every document it changed": {
			Entry:    tools.Entry{Tool: &stubRunner{out: `{"ok":true}`, docs: []string{"doc1", "doc2"}}, Traits: tools.Traits{Write: true}},
			Args:     `{}`,
			Text:     `{"ok":true}`,
			LinkURIs: []string{_resourceURIPrefix + "doc1", _resourceURIPrefix + "doc2"},
		},
		"A write that changed nothing has no link to offer": {
			Entry: tools.Entry{Tool: &stubRunner{out: `{"ok":true}`}, Traits: tools.Traits{Write: true}},
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

			require.Len(t, res.Content, 1+len(c.LinkURIs))

			for n, want := range c.LinkURIs {
				link, lok := res.Content[1+n].(*mcp.ResourceLink)
				require.True(t, lok)
				assert.Equal(t, want, link.URI)
			}
		})
	}
}
