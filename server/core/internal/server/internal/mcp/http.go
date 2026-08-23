// Package mcp serves the Model Context Protocol surface: the
// assistant's tools and the organization's documents over streamable
// HTTP, authenticated with OAuth bearer tokens instead of cookie
// sessions.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// OAuth scopes the MCP surface understands. auth-realtime advertises
// the same three values in its authorization-server metadata.
const (
	// ScopeDocumentRead grants the document read tools and all resource
	// reads.
	ScopeDocumentRead = "documents:read"

	// ScopeDocumentWrite grants the document write tools.
	ScopeDocumentWrite = "documents:write"

	// ScopeDataSourceRead grants the data-source tools. Reading an
	// organisation's documents and querying the databases and metric
	// stores it connects to are different permissions, so they are
	// different grants.
	ScopeDataSourceRead = "data-sources:read"
)

// Options configures the MCP surface.
type Options struct {
	// SessionURL is the full URL of auth-realtime's internal MCP
	// session endpoint used to validate bearer tokens.
	SessionURL string

	// ResourceURL is the canonical public URL of this MCP server
	// (RFC 8707). The RFC 9728 metadata URL advertised in
	// WWW-Authenticate challenges is derived from it.
	ResourceURL string
}

// Validate checks whether the options are valid.
func (o Options) Validate() error {
	if o.SessionURL == "" {
		return errors.New("missing mcp session url")
	}

	if o.ResourceURL == "" {
		return errors.New("missing mcp resource url")
	}

	return nil
}

// Handler serves the MCP endpoint.
type Handler struct {
	log     *slog.Logger
	man     Manager
	db      DB
	handler http.Handler
}

// NewHandler creates a fresh instance of Handler.
func NewHandler(
	log *slog.Logger,
	man Manager,
	db DB,
	client *http.Client,
	opts Options,
) (*Handler, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	metadataURL, err := resourceMetadataURL(opts.ResourceURL)
	if err != nil {
		return nil, fmt.Errorf("deriving resource metadata url: %w", err)
	}

	h := &Handler{
		log: log.With("component", "mcp-handler"),
		man: man,
		db:  db,
	}

	streamable := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return h.server(r)
		},
		&mcp.StreamableHTTPOptions{
			Stateless: true,
			Logger:    h.log,
		},
	)

	h.handler = sdkauth.RequireBearerToken(
		newVerifier(client, opts.SessionURL),
		&sdkauth.RequireBearerTokenOptions{
			ResourceMetadataURL: metadataURL,
		},
	)(streamable)

	return h, nil
}

// ServeHTTP serves the MCP surface: bearer validation first, then the
// streamable HTTP transport.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// resourceMetadataURL derives the RFC 9728 protected-resource metadata
// URL for the given resource: the well-known prefix is inserted between
// the origin and the resource's path.
func resourceMetadataURL(resource string) (string, error) {
	u, err := url.Parse(resource)
	if err != nil {
		return "", err
	}

	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("resource url must be absolute")
	}

	u.Path = "/.well-known/oauth-protected-resource" + u.Path

	return u.String(), nil
}

// Manager builds per-identity tool registries. The assistant's Manager
// satisfies it.
//
//go:generate ../../../../scripts/codegen/mock -t internal Manager
type Manager interface {
	// ToolSet should build the tool registry scoped to the given
	// (organization, user) pair.
	ToolSet(orgID, userID string) *tools.Set
}

// DB is the persistence surface the MCP resources require. The db
// package's agent satisfies it.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentTree should return all documents for the org as a
	// nested summary tree. Used to list document resources.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)
}
