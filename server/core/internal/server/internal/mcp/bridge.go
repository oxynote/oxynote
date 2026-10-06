package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// annotations declares a tool's intent so MCP clients can gate calls:
// reads are read-only, writes say whether they can destroy content, and
// every tool says whether it reaches outside the organization's own
// documents.
func annotations(e tools.Entry) *mcp.ToolAnnotations {
	tr := e.Info.Traits

	// a data-source tool reaches whatever its connection points at.
	out := &mcp.ToolAnnotations{
		ReadOnlyHint:  !tr.Write,
		OpenWorldHint: new(tr.DataSource),
	}

	if tr.Write {
		out.DestructiveHint = new(tr.Destructive || tr.Overwrites)
	}

	return out
}

// toolHandler adapts a registry tool to the MCP call contract. Execution
// failures become isError results rather than protocol errors, so the
// model sees the tool's own error text and can self-correct.
func (h *Handler) toolHandler(e tools.Entry) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}

		out, err := e.Tool.Run(ctx, args)
		if err != nil {
			//nolint:nilerr // execution failures are isError results, not protocol errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
	}
}
