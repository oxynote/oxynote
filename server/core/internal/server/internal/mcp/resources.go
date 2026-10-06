package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// _documentMIMEType is the media type of a document resource body: the
// JSON the get_document tool produces.
const _documentMIMEType = "application/json"

// addResources registers the organization's documents as MCP resources,
// plus the URI template that names them. Reading a resource goes
// through the same read tool the assistant uses, so the body is exactly
// the shape the model already knows.
func (h *Handler) addResources(
	ctx context.Context,
	srv *mcp.Server,
	session Session,
	set *tools.Set,
) {
	read := h.readDocument(set)

	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "document-branch",
		Title:       "Oxynote document branch",
		Description: "One branch of an Oxynote document, by document and branch id: its metadata, its branches and its content as XML markup, one element per block.",
		MIMEType:    _documentMIMEType,
		URITemplate: _resourceURITemplate,
	}, read)

	tree, err := h.db.FetchDocumentTree(ctx, session.OrganizationID)
	if err != nil {
		// listing failure degrades the resource list, not the request:
		// tools remain fully usable and the template still resolves
		// direct reads.
		h.log.Error(
			"cannot list documents for resources",
			slog.String("organization_id", session.OrganizationID),
			slog.String("error", err.Error()),
		)

		return
	}

	// each document is listed on its default branch, which is what a
	// reader lands on without picking one.
	for _, s := range tree.Descendants() {
		srv.AddResource(&mcp.Resource{
			URI:      resourceURI(s.ID, s.DefaultBranchID),
			Name:     s.DocumentName,
			MIMEType: _documentMIMEType,
		}, read)
	}
}

// readDocument serves a document resource read through the get_document
// tool from the request's own tool set, so organization scoping and
// output shape stay identical to a tool call.
func (h *Handler) readDocument(set *tools.Set) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		// a resource names a document and one of its branches; anything
		// short of that names nothing.
		id, branchID, ok := parseResourceURI(req.Params.URI)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		raw, err := json.Marshal(map[string]string{"document_id": id.String(), "branch_id": branchID.String()})
		if err != nil {
			// NOCOV: two string fields always marshal.
			return nil, err
		}

		get, ok := set.Entry(tools.NameGetDocument)
		if !ok {
			// NOCOV: every tool set registers get_document.
			return nil, errors.New("get_document tool not registered")
		}

		out, err := get.Tool.Run(ctx, json.RawMessage(raw))
		if err != nil {
			if errutil.IsNotFound(err) || errors.Is(err, tools.ErrUnknownDocument) || errors.Is(err, tools.ErrUnknownBranch) {
				return nil, mcp.ResourceNotFoundError(req.Params.URI)
			}

			// an internal failure must not read as "document does not
			// exist" to the client.
			return nil, err
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: _documentMIMEType,
				Text:     out,
			}},
		}, nil
	}
}
