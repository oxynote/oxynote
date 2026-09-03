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

// _resourceBranchSegment separates a document id from a branch id in a
// resource URI: oxynote://documents/{id}/branches/{branch_id} reads that
// branch, and every resource names one.
const _resourceBranchSegment = "/branches/"

// annotations declares a tool's intent so MCP clients can gate calls:
// reads are read-only, writes say whether they can destroy content. A
// write counts as destructive here when it removes content outright and
// also when it overwrites content the caller did not name — both lose
// work a client would want to confirm first, which is the question the
// hint answers. A document tool works on the organization's own
// documents, so its world is closed; a data-source tool reaches whatever
// the connection points at, which is not.
func annotations(e tools.Entry) *mcp.ToolAnnotations {
	out := &mcp.ToolAnnotations{
		ReadOnlyHint:  !e.Write,
		OpenWorldHint: new(e.DataSource),
	}

	if e.Write {
		out.DestructiveHint = new(e.Destructive || e.Overwrites)
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

		// a call links back to every branch it changed, so the client
		// can follow the edit straight to its target.
		for _, t := range res.Documents {
			content = append(content, &mcp.ResourceLink{
				URI:      _resourceURIPrefix + t.DocumentID.String() + _resourceBranchSegment + t.BranchID.String(),
				Name:     t.DocumentID.String(),
				MIMEType: _documentMIMEType,
			})
		}

		return &mcp.CallToolResult{Content: content}, nil
	}
}
