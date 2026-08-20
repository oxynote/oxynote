// Package tools owns the AI assistant's tool surface. Each tool the
// model can call lives in its own file holding its schema, its
// implementation, and the words shown to the user while it runs; Set
// gathers them for one session. All tools execute server-side — there
// is no client-side forwarding.
package tools

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/cloudwego/eino/components/tool"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// Name is the canonical identifier of one tool the assistant
// exposes to the model. Names use snake_case; they double as the
// model-facing tool name and the protocol confirm-action's "tool"
// field.
type Name string

// Read tools — never confirmed. The assistant runs these
// immediately on every relevant turn.
const (
	NameListDocuments       Name = "list_documents"
	NameGetDocument         Name = "get_document"
	NameReadDocumentSummary Name = "read_document_summary"
	NameReadBlock           Name = "read_block"
	NameSearchDocuments     Name = "search_documents"
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

// Set is the tools one session offers the model: an immutable registry
// built once from an Input and handed straight to the agent.
//
// It is the only place that names every tool. What a tool is — whether
// it needs confirming, whether it survives an approve-all, what the
// user is told while it runs — is asked of the tool itself, so there is
// no second list to keep in step with this one.
type Set struct {
	// tools maps each tool's name to the value that serves it, already
	// wrapped in its confirmation gate where one applies.
	tools map[Name]tool.InvokableTool

	// writes lists the tools that mutate a document.
	writes []string
}

// New creates a fresh instance of Set.
func New(inp *Input) *Set {
	registerConfirmTypes()

	all := []struct {
		Name Name
		Tool tool.InvokableTool
	}{
		{NameListDocuments, &listDocuments{inp}},
		{NameGetDocument, &getDocument{inp}},
		{NameReadDocumentSummary, &readDocumentSummary{inp}},
		{NameReadBlock, &readBlock{inp}},
		{NameSearchDocuments, &searchDocuments{inp}},

		{NameCreateDocument, &createDocument{inp}},
		{NameDeleteDocument, &deleteDocument{inp}},
		{NameRenameDocument, &renameDocument{inp}},
		{NameSetDocumentIcon, &setDocumentIcon{inp}},
		{NameMoveDocument, &moveDocument{inp}},
		{NameInsertBlock, &insertBlock{inp}},
		{NameAppendBlock, &appendBlock{inp}},
		{NamePrependBlock, &prependBlock{inp}},
		{NameReplaceBlock, &replaceBlock{inp}},
		{NameUpdateBlockText, &updateBlockText{inp}},
		{NameUpdateBlockAttrs, &updateBlockAttrs{inp}},
		{NameDeleteBlock, &deleteBlock{inp}},
	}

	s := &Set{tools: make(map[Name]tool.InvokableTool, len(all))}

	for _, e := range all {
		t := e.Tool

		// a tool that can describe a pending write is a tool that
		// performs one, so the gate goes on here rather than being
		// something every write has to remember to ask for.
		if c, ok := t.(Confirmer); ok {
			_, destructive := t.(Destructive)

			t = &confirming{InvokableTool: t, summary: c, destructive: destructive}

			s.writes = append(s.writes, string(e.Name))
		}

		s.tools[e.Name] = t
	}

	return s
}

// Tools returns every tool, ready to hand to the agent.
func (s *Set) Tools() []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t)
	}

	return out
}

// Label returns a short line describing what the named tool is about to
// do, or an empty string for tools too noisy or too generic to
// announce. An unknown name is not an error: the agent also runs tools
// this set does not own, such as the offloaded-result reader.
func (s *Set) Label(ctx context.Context, name Name, args json.RawMessage) string {
	t, ok := s.tools[name]
	if !ok {
		return ""
	}

	l, ok := unwrap(t).(labeller)
	if !ok {
		// NOCOV: every tool in the registry implements labeller.
		return ""
	}

	return l.Label(ctx, args)
}

// WriteNames returns every tool that mutates a document.
//
// The context middlewares use it to decide what must never be cleared
// from the conversation: the model has to keep knowing what it changed,
// while a stale read can always be taken again. The list is exactly the
// set of tools gated behind confirmation, so the two cannot drift.
func (s *Set) WriteNames() []string {
	return slices.Clone(s.writes)
}

// unwrap returns the tool underneath a confirmation gate, or the tool
// itself when it is not gated.
func unwrap(t tool.InvokableTool) tool.InvokableTool {
	if c, ok := t.(*confirming); ok {
		return c.InvokableTool
	}

	return t
}

// labeller is implemented by every tool in the registry, so it can
// describe itself while it runs.
type labeller interface {
	// Label should return a short line describing what the tool is
	// about to do, or an empty string to run without announcement.
	Label(ctx context.Context, args json.RawMessage) string
}

// DB is the persistence surface the tools require. The db package's
// agent satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both DB db
//nolint:interfacebloat // the list is exactly what the tools call, and splitting it by nothing but count would only hide that
type DB interface {
	sqlutil.DB

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
	FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*document.Document, error)

	// FetchMainBranchContent returns the parsed main-branch
	// content of the document. Used when an op only needs the
	// content tree (no branch metadata).
	FetchMainBranchContent(ctx context.Context, docID xid.ID, organizationID string) (document.Content, error)

	// InsertDocument creates a new document. Used by
	// create_document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// UpsertDocumentMaintainers adds the given user ids to a
	// document's maintainer set (idempotent via ON CONFLICT). Used
	// by create_document to make the requesting user a maintainer
	// of any document the assistant creates on their behalf.
	UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error

	// InsertDocumentSearchJob should queue the search index update for a
	// document. Used by create_document, whose document would otherwise
	// stay unindexed until its first edit.
	InsertDocumentSearchJob(ctx context.Context, diff search.BlocksDifference) error

	// DeleteDocument removes a document. Used by delete_document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) error

	// UpdateDocumentParentID re-parents a document. Used by
	// move_document.
	UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error

	// CheckDocumentExists reports whether the document exists in
	// the given org. Used by move_document to validate the new
	// parent before issuing UPDATE.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error

	// CheckDocumentCycle reports whether making parentID the parent
	// of id would create a cycle in the document tree. Used by
	// move_document to reject self and descendant parents.
	CheckDocumentCycle(ctx context.Context, id, parentID xid.ID, organizationID string) (bool, error)
}

// Tx is the transactional half of DB, so a tool whose write spans tables can
// commit or abandon all of it at once.
//
//go:generate ../../../scripts/codegen/mock -t both Tx tx
type Tx interface {
	sqlutil.Tx

	// InsertDocument should create a new document. Used by create_document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// UpsertDocumentMaintainers should add the given user ids to a
	// document's maintainer set. Used by create_document.
	UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error

	// InsertDocumentSearchJob should queue the search index update for a
	// document. Used by create_document.
	InsertDocumentSearchJob(ctx context.Context, diff search.BlocksDifference) error
}

// Searcher is the full-text search surface search_documents uses.
// The document/search Meilisearch client satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both Searcher searcher
type Searcher interface {
	// SearchDocumentBlocks returns blocks whose text matches the
	// query, scoped to the organization and capped at limit hits.
	SearchDocumentBlocks(ctx context.Context, organizationID, query string, limit int) ([]search.Block, error)
}

// TreeNotifier publishes document-tree-change events so connected
// clients can refresh their sidebar after assistant-driven creates,
// deletes, moves, renames, or icon changes. The server document handler
// satisfies this interface via its NotifyTreeChange method.
//
//go:generate ../../../scripts/codegen/mock -t both TreeNotifier tree_notifier
type TreeNotifier interface {
	// NotifyTreeChange tells subscribers that the tree under
	// parentID (a null value means the root) changed in
	// organizationID. Implementations must be safe to call
	// concurrently.
	NotifyTreeChange(organizationID string, parentID null.Value[xid.ID])
}

// EditApplier is the live-document mutation surface the write tools
// use for content edits and the rename/set-icon ops. The edit.Client
// satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both EditApplier edit_applier
type EditApplier interface {
	// Apply should ship the operation batch to the realtime service
	// for the (documentID, branchID) document and return the per-op
	// outcome.
	Apply(ctx context.Context, documentID, branchID string, ops []edit.Operation) (edit.Result, error)
}
