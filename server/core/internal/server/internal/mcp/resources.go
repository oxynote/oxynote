package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// _documentMIMEType is the media type of a document resource body: the
// canonical block JSON the read_document_summary tool produces.
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
		Name:        "document",
		Title:       "Oxynote document",
		Description: "An Oxynote document's blocks in canonical JSON form.",
		MIMEType:    _documentMIMEType,
		URITemplate: _resourceURIPrefix + "{id}",
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

	for _, s := range tree.Descendants() {
		srv.AddResource(&mcp.Resource{
			URI:      _resourceURIPrefix + s.ID.String(),
			Name:     s.DocumentName,
			MIMEType: _documentMIMEType,
		}, read)
	}
}

// readDocument serves a document resource read through the
// read_document_summary tool from the request's own tool set, so
// organization scoping and output shape stay identical to a tool call.
func (h *Handler) readDocument(set *tools.Set) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id, ok := strings.CutPrefix(req.Params.URI, _resourceURIPrefix)
		if !ok || id == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		summary, ok := set.Entry(tools.NameReadDocumentSummary)
		if !ok {
			// NOCOV: every tool set registers read_document_summary.
			return nil, errors.New("read_document_summary tool not registered")
		}

		res, err := summary.Tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, id)))
		if err != nil {
			if errutil.IsNotFound(err) {
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
				Text:     res.Output,
			}},
		}, nil
	}
}
