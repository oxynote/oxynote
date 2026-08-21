package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEinoTool is a minimal eino tool for exercising the bridge without
// the real registry: fixed info, fixed output, optional errors.
type stubEinoTool struct {
	// infoErr fails Info when set.
	infoErr error

	// out is what InvokableRun returns.
	out string

	// runErr fails InvokableRun when set.
	runErr error
}

// Info returns a fixed minimal description.
func (s *stubEinoTool) Info(context.Context) (*schema.ToolInfo, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}

	return &schema.ToolInfo{Name: "stub", Desc: "stub tool"}, nil
}

// InvokableRun returns the configured output.
func (s *stubEinoTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return s.out, s.runErr
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

func Test_toolDef(t *testing.T) {
	t.Parallel()

	// error
	def, err := toolDef(context.Background(), tools.Entry{Tool: &stubEinoTool{infoErr: assert.AnError}})
	require.ErrorContains(t, err, "describing tool")
	assert.Nil(t, def)

	// success
	e := entryFor(t, tools.NameGetDocument)

	def, err = toolDef(context.Background(), e)
	require.NoError(t, err)
	require.NotNil(t, def)

	assert.Equal(t, "get_document", def.Name)
	assert.NotEmpty(t, def.Description)
	require.NotNil(t, def.Annotations)
	assert.True(t, def.Annotations.ReadOnlyHint)

	// the input schema survives the eino → MCP conversion intact.
	raw, ok := def.InputSchema.(json.RawMessage)
	require.True(t, ok)
	assert.Contains(t, string(raw), `"document_id"`)
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
		Entry   tools.Entry
		Args    string
		IsError bool
		Text    string
		LinkURI string
	}{
		"Tool failure becomes an isError result": {
			Entry:   tools.Entry{Tool: &stubEinoTool{runErr: assert.AnError}},
			Args:    `{}`,
			IsError: true,
			Text:    assert.AnError.Error(),
		},
		"Read result carries no resource link": {
			Entry: tools.Entry{Tool: &stubEinoTool{out: `{"ok":true}`}},
			Args:  `{"document_id":"doc1"}`,
			Text:  `{"ok":true}`,
		},
		"Write result links the document from its arguments": {
			Entry:   tools.Entry{Tool: &stubEinoTool{out: `{"ok":true}`}, Traits: tools.Traits{Write: true}},
			Args:    `{"document_id":"doc1"}`,
			Text:    `{"ok":true}`,
			LinkURI: _resourceURIPrefix + "doc1",
		},
		"Write result links the document from its output": {
			Entry:   tools.Entry{Tool: &stubEinoTool{out: `{"document_id":"doc2"}`}, Traits: tools.Traits{Write: true}},
			Args:    ``,
			Text:    `{"document_id":"doc2"}`,
			LinkURI: _resourceURIPrefix + "doc2",
		},
		"Write result without a document id has no link": {
			Entry: tools.Entry{Tool: &stubEinoTool{out: `{"ok":true}`}, Traits: tools.Traits{Write: true}},
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

			if c.LinkURI == "" {
				assert.Len(t, res.Content, 1)
				return
			}

			require.Len(t, res.Content, 2)

			link, ok := res.Content[1].(*mcp.ResourceLink)
			require.True(t, ok)
			assert.Equal(t, c.LinkURI, link.URI)
		})
	}
}

func Test_documentID(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   string
		Out    string
		Result string
	}{
		"ID in the arguments": {
			Args:   `{"document_id":"doc1"}`,
			Out:    `{}`,
			Result: "doc1",
		},
		"ID in the output only": {
			Args:   `{}`,
			Out:    `{"document_id":"doc2"}`,
			Result: "doc2",
		},
		"No id anywhere": {
			Args: `{}`,
			Out:  `{}`,
		},
		"Malformed arguments and output": {
			Args: `{`,
			Out:  `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, documentID(json.RawMessage(c.Args), json.RawMessage(c.Out)))
		})
	}
}
