package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/ptrutil"
)

// _resourceURIPrefix addresses a document in the oxynote resource
// scheme; the id after the prefix is the document's xid.
const _resourceURIPrefix = "oxynote://documents/"

// toolDef converts an eino tool's self-description into the MCP wire
// shape: same name, same description, and the same JSON input schema
// the assistant's model sees.
func toolDef(ctx context.Context, e tools.Entry) (*mcp.Tool, error) {
	info, err := e.Tool.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("describing tool: %w", err)
	}

	js, err := info.ToJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("converting params to json schema: %w", err)
	}

	raw, err := json.Marshal(js)
	if err != nil {
		return nil, fmt.Errorf("marshaling json schema: %w", err)
	}

	return &mcp.Tool{
		Name:        string(e.Name),
		Description: info.Desc,
		InputSchema: json.RawMessage(raw),
		Annotations: annotations(e),
	}, nil
}

// annotations declares a tool's intent so MCP clients can gate calls:
// reads are read-only, writes say whether they can destroy content.
// Every tool works on the organization's own documents, so the world
// is closed for all of them.
func annotations(e tools.Entry) *mcp.ToolAnnotations {
	out := &mcp.ToolAnnotations{
		ReadOnlyHint:  !e.Write,
		OpenWorldHint: ptrutil.New(false),
	}

	if e.Write {
		out.DestructiveHint = ptrutil.New(e.Destructive)
	}

	return out
}

// toolHandler adapts an eino tool to the MCP call contract. Execution
// failures become isError results rather than protocol errors, so the
// model sees the tool's own error text and can self-correct.
func (h *Handler) toolHandler(e tools.Entry) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}

		out, err := e.Tool.InvokableRun(ctx, string(args))
		if err != nil {
			//nolint:nilerr // execution failures are isError results, not protocol errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		content := []mcp.Content{&mcp.TextContent{Text: out}}

		// a write links back to the document it touched, so the client
		// can follow the edit straight to its target.
		if e.Write {
			if id := documentID(args, json.RawMessage(out)); id != "" {
				content = append(content, &mcp.ResourceLink{
					URI:      _resourceURIPrefix + id,
					Name:     id,
					MIMEType: _documentMIMEType,
				})
			}
		}

		return &mcp.CallToolResult{Content: content}, nil
	}
}

// documentID pulls the document a write targeted from its arguments,
// falling back to the result for tools that only know their document
// after running (create_document returns the id it minted).
func documentID(args, out json.RawMessage) string {
	var probe struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal(args, &probe); err == nil && probe.DocumentID != "" {
		return probe.DocumentID
	}

	probe.DocumentID = ""

	if err := json.Unmarshal(out, &probe); err != nil {
		return ""
	}

	return probe.DocumentID
}
