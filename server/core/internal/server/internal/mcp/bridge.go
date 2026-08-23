package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// _resourceURIPrefix addresses a document in the oxynote resource
// scheme; the id after the prefix is the document's xid.
const _resourceURIPrefix = "oxynote://documents/"

// annotations declares a tool's intent so MCP clients can gate calls:
// reads are read-only, writes say whether they can destroy content. A
// document tool works on the organization's own documents, so its world
// is closed; a data-source tool reaches whatever the connection points
// at, which is not.
func annotations(e tools.Entry) *mcp.ToolAnnotations {
	out := &mcp.ToolAnnotations{
		ReadOnlyHint:  !e.Write,
		OpenWorldHint: new(e.DataSource),
	}

	if e.Write {
		out.DestructiveHint = new(e.Destructive)
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

		res, err := e.Tool.Run(ctx, args)
		if err != nil {
			//nolint:nilerr // execution failures are isError results, not protocol errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		content := []mcp.Content{&mcp.TextContent{Text: res.Output}}

		// a call links back to every document it changed, so the client
		// can follow the edit straight to its target.
		for _, id := range res.Documents {
			content = append(content, &mcp.ResourceLink{
				URI:      _resourceURIPrefix + id.String(),
				Name:     id.String(),
				MIMEType: _documentMIMEType,
			})
		}

		return &mcp.CallToolResult{Content: content}, nil
	}
}
