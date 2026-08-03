// Package tools owns the AI assistant's tool surface: the read and
// write tools the model can call, the per-tool input schemas
// exposed to Anthropic, and the dispatcher that turns a tool_use
// block into a tool_result. All tools execute server-side — there
// is no client-side forwarding.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/heimdall/internal/document"
	"github.com/oxynote/heimdall/internal/document/liveedit"
	"github.com/oxynote/heimdall/internal/document/searchgw"
	"github.com/rs/xid"
)

// Name is the canonical identifier of one tool the assistant
// exposes to Anthropic. Names use snake_case; they double as the
// Anthropic tool_use.name and the protocol confirm-action's "tool"
// field.
type Name string

// Read tools — never confirmed. The assistant runs these
// immediately on every relevant turn.
const (
	NameListDocuments        Name = "list_documents"
	NameGetDocument          Name = "get_document"
	NameReadDocumentSummary  Name = "read_document_summary"
	NameReadBlock            Name = "read_block"
	NameSearchDocuments      Name = "search_documents"
)

// Write tools — always confirmed. Deletes are the most
// destructive but the same confirm gate covers all of them.
const (
	NameCreateDocument   Name = "create_document"
	NameDeleteDocument   Name = "delete_document"
	NameRenameDocument   Name = "rename_document"
	NameSetDocumentIcon  Name = "set_document_icon"
	NameMoveDocument     Name = "move_document"
	NameInsertBlock      Name = "insert_block"
	NameAppendBlock      Name = "append_block"
	NamePrependBlock     Name = "prepend_block"
	NameReplaceBlock     Name = "replace_block"
	NameUpdateBlockText  Name = "update_block_text"
	NameUpdateBlockAttrs Name = "update_block_attrs"
	NameDeleteBlock      Name = "delete_block"
)

// IsWrite reports whether the named tool requires user confirmation
// before execution. Unknown names default to false (treated as a
// read) so a typo in a tool_use name never silently bypasses the
// confirm gate.
func IsWrite(name Name) bool {
	switch name {
	case NameCreateDocument,
		NameDeleteDocument,
		NameRenameDocument,
		NameSetDocumentIcon,
		NameMoveDocument,
		NameInsertBlock,
		NameAppendBlock,
		NamePrependBlock,
		NameReplaceBlock,
		NameUpdateBlockText,
		NameUpdateBlockAttrs,
		NameDeleteBlock:
		return true
	}

	return false
}

// IsValid reports whether name is one of the assistant's known
// tools.
func IsValid(name Name) bool {
	switch name {
	case NameListDocuments,
		NameGetDocument,
		NameReadDocumentSummary,
		NameReadBlock,
		NameSearchDocuments,
		NameCreateDocument,
		NameDeleteDocument,
		NameRenameDocument,
		NameSetDocumentIcon,
		NameMoveDocument,
		NameInsertBlock,
		NameAppendBlock,
		NamePrependBlock,
		NameReplaceBlock,
		NameUpdateBlockText,
		NameUpdateBlockAttrs,
		NameDeleteBlock:
		return true
	}

	return false
}

// DB is the persistence surface the tools manager requires. The
// db package's agent satisfies it.
type DB interface {
	// FetchDocumentTree returns all documents for the org as a
	// nested summary tree (sort_index order). Used by
	// list_documents.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)

	// FetchDocumentTreeByDocumentParentID returns the children of
	// parentID within the org (nil parentID = top-level). Used by
	// list_documents.
	FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (document.Summaries, error)

	// FetchDocument returns full document content for the named
	// branch. Used by get_document, read_document_summary,
	// read_block.
	FetchDocument(ctx context.Context, id xid.ID, organizationID string, branchName string) (*document.Document, error)

	// FetchMainBranchContent returns the parsed main-branch
	// content of the document. Used when an op only needs the
	// content tree (no branch metadata).
	FetchMainBranchContent(ctx context.Context, docID xid.ID, organizationID string) (document.DocumentContent, error)

	// InsertDocument creates a new document. Used by
	// create_document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// UpsertDocumentMaintainers adds the given user ids to a
	// document's maintainer set (idempotent via ON CONFLICT). Used
	// by create_document to make the requesting user a maintainer
	// of any document the assistant creates on their behalf.
	UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error

	// DeleteDocument removes a document. Used by delete_document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) error

	// UpdateDocumentParentID re-parents a document. Used by
	// move_document.
	UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error

	// CheckDocumentExists reports whether the document exists in
	// the given org. Used by move_document to validate the new
	// parent before issuing UPDATE.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error
}

// Searcher is the full-text search surface search_documents uses.
// The document/searchgw Meilisearch client satisfies it.
type Searcher interface {
	// SearchDocumentBlocks returns blocks whose text matches the
	// query, scoped to the organization and capped at limit hits.
	SearchDocumentBlocks(ctx context.Context, organizationID, query string, limit int) ([]searchgw.Block, error)
}

// TreeNotifier publishes document-tree-change events so connected
// clients can refresh their sidebar after assistant-driven creates,
// deletes, moves, renames, or icon changes. The dochandler.Handler
// satisfies this interface via its NotifyTreeChange method.
type TreeNotifier interface {
	// NotifyTreeChange tells subscribers that the tree under
	// parentID (a null value means the root) changed in
	// organizationID. Implementations must be safe to call
	// concurrently.
	NotifyTreeChange(organizationID string, parentID null.Value[xid.ID])
}

// Manager dispatches AI tool calls. One Manager is constructed per
// session and is scoped to a single (organization, user) pair so
// cross-org access is impossible by construction.
type Manager struct {
	// log is scoped to the session's (org, user) and used to record
	// per-tool outcomes so we can diagnose AI loops without
	// re-running the conversation.
	log *slog.Logger

	// db is the persistence used by read tools and the
	// non-content write tools (create/delete/move).
	db DB

	// search is the full-text index behind search_documents.
	search Searcher

	// liveedit is the live edit client for content mutations and
	// the rename/set-icon ops that must propagate to connected
	// editors.
	liveedit *liveedit.Client

	// tree notifies tree-change subscribers after the assistant
	// mutates the document tree.
	tree TreeNotifier

	// orgID scopes every tool call to one organization.
	orgID string

	// userID identifies the user the assistant is acting for. Used
	// when a tool creates audit-relevant rows (the created_by
	// fields on a new document, for instance).
	userID string
}

// NewManager constructs a Manager. Every dependency is required;
// nil values surface as nil-pointer panics on the first tool call
// rather than at startup, but in practice the cmd-level wiring
// passes all of them.
func NewManager(log *slog.Logger, db DB, search Searcher, liveedit *liveedit.Client, tree TreeNotifier, orgID, userID string) *Manager {
	return &Manager{
		log: log.With(
			"component", "assistant-tools",
			"org_id", orgID,
			"user_id", userID,
		),
		db:       db,
		search:   search,
		liveedit: liveedit,
		tree:     tree,
		orgID:    orgID,
		userID:   userID,
	}
}

// Execute runs the named tool with the supplied JSON arguments and
// returns the JSON tool_result the caller should hand back to
// Anthropic. Errors from the tool (validation, persistence, RPC) are
// returned as Go errors; the caller surfaces them as a structured
// tool_result on the model's behalf.
func (m *Manager) Execute(ctx context.Context, name Name, args json.RawMessage) (json.RawMessage, error) {
	switch name {
	case NameListDocuments:
		return m.listDocuments(ctx, args)
	case NameGetDocument:
		return m.getDocument(ctx, args)
	case NameReadDocumentSummary:
		return m.readDocumentSummary(ctx, args)
	case NameReadBlock:
		return m.readBlock(ctx, args)
	case NameSearchDocuments:
		return m.searchDocuments(ctx, args)
	case NameCreateDocument:
		return m.createDocument(ctx, args)
	case NameDeleteDocument:
		return m.deleteDocument(ctx, args)
	case NameRenameDocument:
		return m.renameDocument(ctx, args)
	case NameSetDocumentIcon:
		return m.setDocumentIcon(ctx, args)
	case NameMoveDocument:
		return m.moveDocument(ctx, args)
	case NameInsertBlock:
		return m.insertBlock(ctx, args)
	case NameAppendBlock:
		return m.appendBlock(ctx, args)
	case NamePrependBlock:
		return m.prependBlock(ctx, args)
	case NameReplaceBlock:
		return m.replaceBlock(ctx, args)
	case NameUpdateBlockText:
		return m.updateBlockText(ctx, args)
	case NameUpdateBlockAttrs:
		return m.updateBlockAttrs(ctx, args)
	case NameDeleteBlock:
		return m.deleteBlock(ctx, args)
	}

	return nil, fmt.Errorf("unknown tool: %s", name)
}
