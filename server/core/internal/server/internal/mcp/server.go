package mcp

import (
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
)

// server builds the MCP server for one request's identity: the tools
// the token's scopes allow, plus the organization's documents as
// resources. Stateless transport means every request gets a fresh
// build, which keeps the tool list and the resource list in step with
// the token and the document tree.
//
// The cost of that is one FetchDocumentTree query per request holding
// the read scope, whatever the request turns out to be — the transport
// builds the server before it parses the JSON-RPC body, so there is no
// method to branch on here. Registering the resources later, from a
// receiving middleware, would scope the query to resources/list but
// would also make AddResource fire inside a live session, where it
// schedules a resources/list_changed notification the client never
// asked for. One indexed query is the cheaper trade.
func (h *Handler) server(r *http.Request) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "oxynote",
		Title:   "Oxynote",
		Version: buildinfo.Full().Version.String(),
	}, &mcp.ServerOptions{
		Logger: h.log,
	})

	session, err := sessionFromContext(r.Context())
	if err != nil {
		// NOCOV: the bearer middleware rejects the request before the
		// server is built when no valid token is present.
		h.log.Error("mcp session missing from request context")

		return srv
	}

	set := h.man.ToolSet(session.OrganizationID, session.UserID)

	read := slices.Contains(session.Scopes, ScopeDocumentRead)
	write := slices.Contains(session.Scopes, ScopeDocumentWrite)
	dataSources := slices.Contains(session.Scopes, ScopeDataSourceRead)

	for _, e := range set.Entries() {
		// an internal tool addresses conversation state this surface has
		// none of, so offering it would only invite a call that cannot
		// succeed.
		if e.Internal {
			continue
		}

		// a data-source tool reaches outside Oxynote entirely, so it
		// answers to its own scope rather than to the document ones.
		if e.DataSource {
			if !dataSources {
				continue
			}
		} else if e.Write && !write || !e.Write && !read {
			continue
		}

		// the wire shape is the tool's own description: same name, same
		// prose, same argument schema the assistant's model sees.
		srv.AddTool(&mcp.Tool{
			Name:        string(e.Name),
			Description: e.Info.Description,
			InputSchema: e.Info.Schema(),
			Annotations: annotations(e),
		}, h.toolHandler(e))
	}

	if read {
		h.addResources(r.Context(), srv, session, set)
	}

	return srv
}
