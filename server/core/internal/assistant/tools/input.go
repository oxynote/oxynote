package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// Input carries everything a tool needs to do its work. Every tool
// embeds one, so a tool file names only the arguments it takes and the
// work it does, never the plumbing it needs to get there.
//
// It is built per session and scoped to a single (organization, user)
// pair, so cross-org access is impossible by construction.
type Input struct {
	// log is scoped to the session's (org, user) and used to record
	// per-tool outcomes so we can diagnose AI loops without
	// re-running the conversation.
	log *slog.Logger

	// db is the persistence used by read tools and the non-content
	// write tools (create/delete/move).
	db DB

	// search is the full-text index behind search_documents.
	search Searcher

	// applier is the edit client for content mutations and the
	// rename/set-icon ops that must propagate to connected editors.
	applier EditApplier

	// tree notifies tree-change subscribers after the assistant
	// mutates the document tree.
	tree TreeNotifier

	// orgID scopes every tool call to one organization.
	orgID string

	// userID identifies the user the assistant is acting for. Used
	// when a tool creates audit-relevant rows (the created_by fields
	// on a new document, for instance).
	userID string
}

// NewInput creates a fresh instance of Input. Every dependency is
// required; nil values surface as nil-pointer panics on the first tool
// call rather than at startup, but in practice the cmd-level wiring
// passes all of them.
func NewInput(
	log *slog.Logger,
	db DB,
	searcher Searcher,
	applier EditApplier,
	tree TreeNotifier,
	orgID, userID string,
) *Input {
	return &Input{
		log: log.With(
			"component", "assistant-tools",
			"org_id", orgID,
			"user_id", userID,
		),
		db:      db,
		search:  searcher,
		applier: applier,
		tree:    tree,
		orgID:   orgID,
		userID:  userID,
	}
}

// docRef wraps the (documentID, branchID) pair the edit client needs to
// address a live Y.Doc. The branch is resolved to the document's
// default branch — multi-branch editing is out of scope for the
// assistant.
type docRef struct {
	// DocumentID is the document's id in string form.
	DocumentID string

	// BranchID is the id of the document's default branch.
	BranchID string
}

// resolveDoc loads the default branch of the given document and returns
// the ids the edit client needs. The lookup also acts as the cross-org
// safety check — db.FetchDocument scopes by orgID so a docID from
// another organisation surfaces as NotFound.
func (i *Input) resolveDoc(ctx context.Context, documentID string) (docRef, error) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return docRef{}, fmt.Errorf("document_id is not a valid xid: %w", err)
	}

	doc, err := i.db.FetchDocument(ctx, docID, i.orgID, document.DefaultBranch)
	if err != nil {
		return docRef{}, fmt.Errorf("fetching document: %w", err)
	}

	return docRef{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
	}, nil
}

// applyEdit is the shared tail of every content-mutating write tool: it
// resolves the document to a (documentID, branchID) pair, ships the
// operation batch to Node, and surfaces the per-op result. Outcomes are
// logged so partial failures on the Node side (uid not found, malformed
// block) are visible without re-running the conversation.
func (i *Input) applyEdit(
	ctx context.Context,
	documentID string,
	ops []edit.Operation,
) (string, error) {
	ref, err := i.resolveDoc(ctx, documentID)
	if err != nil {
		i.log.Warn(
			"edit resolve failed",
			slog.String("document_id", documentID),
			slog.String("error", err.Error()),
		)

		return "", err
	}

	res, err := i.applier.Apply(ctx, ref.DocumentID, ref.BranchID, ops)
	if err != nil {
		i.log.Error(
			"edit apply failed",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("op_count", len(ops)),
			slog.String("error", err.Error()),
		)

		return "", fmt.Errorf("applying edit: %w", err)
	}

	if len(res.Errors) > 0 {
		i.log.Warn(
			"edit partial failure",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("applied", res.Applied),
			slog.Any("errors", res.Errors),
		)
	} else {
		i.log.Debug(
			"edit applied",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("applied", res.Applied),
		)
	}

	return result(res)
}

// validatePlacement validates a block that is about to land next to, or
// in place of, the block referenceUID names. Types the document root
// accepts are legal wherever their parent takes them, so only a macro
// internal — a titled_code, metric or param_list, which the editor's
// schema binds to its container — has to look at where it is going.
func (i *Input) validatePlacement(
	ctx context.Context,
	documentID, referenceUID string,
	b block.Block,
) error {
	if err := block.Validate(b); err != nil {
		return err
	}

	if block.ValidateAsRoot(b) == nil {
		return nil
	}

	docID, err := xid.FromString(documentID)
	if err != nil {
		return fmt.Errorf("document_id is not a valid xid: %w", err)
	}

	content, err := i.db.FetchMainBranchContent(ctx, docID, i.orgID)
	if err != nil {
		return fmt.Errorf("fetching content: %w", err)
	}

	for _, rb := range content.Content.Content {
		if uid, ok := rb.UID(); ok && uid == referenceUID {
			return block.ValidateAsRoot(b)
		}
	}

	return nil
}

// notifyTreeChange invokes the tree notifier when one is configured.
// Safe to call from any tool — a nil notifier silently no-ops so tests
// that don't wire one don't trip.
func (i *Input) notifyTreeChange(parentID null.Value[xid.ID]) {
	if i.tree == nil {
		return
	}

	i.tree.NotifyTreeChange(i.orgID, parentID)
}

// notifyTreeChangeForDocument looks up the document's current parent
// and fires a tree-change for that parent. Used by rename/icon ops
// which don't carry a parent in their args. Failures (e.g. doc fetched
// after delete) silently skip the notification.
func (i *Input) notifyTreeChangeForDocument(ctx context.Context, documentID string) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return
	}

	doc, err := i.db.FetchDocument(ctx, docID, i.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return
	}

	i.notifyTreeChange(doc.ParentID)
}

// lookupDocumentName fetches the document's display name. Failures (bad
// id, not found, transient) return an empty string so the confirm UI
// gracefully falls back to the id alone.
func (i *Input) lookupDocumentName(ctx context.Context, documentID string) string {
	id, err := xid.FromString(documentID)
	if err != nil {
		return ""
	}

	doc, err := i.db.FetchDocument(ctx, id, i.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return ""
	}

	return doc.DocumentName
}

// parseToolArgs unmarshals a tool's JSON args into dst for the
// best-effort label and summary helpers: a malformed args payload
// should degrade the label, not abort the surrounding flow. So we log a
// warning and let dst keep its zero value rather than propagating the
// error. The provider enforces the input schema at the tool-call
// boundary, so a failure here is unusual and worth knowing about.
func (i *Input) parseToolArgs(args json.RawMessage, dst any) {
	if err := json.Unmarshal(args, dst); err != nil {
		i.log.Warn("tool args unmarshal failed",
			slog.String("error", err.Error()),
		)
	}
}

// subject returns the display subject for a tool's label and summary:
// the named document when it can be resolved, a generic fallback
// otherwise.
func (i *Input) subject(ctx context.Context, args json.RawMessage) string {
	docID := i.documentID(args)
	if docID == "" {
		return subjectFor("")
	}

	return subjectFor(i.lookupDocumentName(ctx, docID))
}

// summarize builds a confirmation for a write that targets an existing
// document: the document is resolved once, and the tool supplies only
// the phrasing for its own change.
func (i *Input) summarize(
	ctx context.Context,
	name Name,
	args json.RawMessage,
	phrase func(subject string) string,
) ConfirmActionSummary {
	out := ConfirmActionSummary{Tool: string(name)}

	if docID := i.documentID(args); docID != "" {
		out.DocumentID = docID
		out.DocumentName = i.lookupDocumentName(ctx, docID)
	}

	out.Summary = phrase(subjectFor(out.DocumentName))

	return out
}

// documentID returns the document a tool's args target, or an empty
// string when the args name none.
func (i *Input) documentID(args json.RawMessage) string {
	var probe struct {
		DocumentID string `json:"document_id"`
	}

	i.parseToolArgs(args, &probe)

	return probe.DocumentID
}

// blockType reads the canonical type of the block a write tool was
// handed, for the confirm summary.
func (i *Input) blockType(args json.RawMessage) string {
	var in struct {
		Block struct {
			Type string `json:"type"`
		} `json:"block"`
	}

	i.parseToolArgs(args, &in)

	return in.Block.Type
}

// DB is the persistence surface the tools require. The db package's
// agent satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both DB db
type DB interface {
	sqlutil.DB

	// FetchDocumentTree should return all documents for the org as a
	// nested summary tree (sort_index order). Used by
	// list_documents.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)

	// FetchDocumentTreeByDocumentParentID should return the children of
	// parentID within the org (nil parentID = top-level). Used by
	// list_documents.
	FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (document.Summaries, error)

	// FetchDocument should return full document content for the named
	// branch. Used by get_document, read_document_summary,
	// read_block.
	FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*document.Document, error)

	// FetchMainBranchContent should return the parsed main-branch
	// content of the document. Used when an op only needs the
	// content tree (no branch metadata).
	FetchMainBranchContent(ctx context.Context, docID xid.ID, organizationID string) (document.Content, error)

	// DeleteDocument should remove a document. Used by delete_document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) error

	// UpdateDocumentParentID should re-parent a document. Used by
	// move_document.
	UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error

	// CheckDocumentExists should report whether the document exists in
	// the given org. Used by move_document to validate the new
	// parent before issuing UPDATE.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error

	// CheckDocumentCycle should report whether making parentID the parent
	// of id would create a cycle in the document tree. Used by
	// move_document to reject self and descendant parents.
	CheckDocumentCycle(ctx context.Context, id, parentID xid.ID, organizationID string) (bool, error)
}

// Tx is the transactional half of DB, so a tool whose write spans tables can
// commit or abandon all of it at once.
//
//go:generate ../../../scripts/codegen/mock -t internal Tx tx
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
	// SearchDocumentBlocks should return blocks whose text matches the
	// query, scoped to the organization and capped at limit hits.
	SearchDocumentBlocks(ctx context.Context, organizationID, query string, limit int) ([]search.Block, error)
}

// TreeNotifier publishes document-tree-change events so connected
// clients can refresh their sidebar after assistant-driven creates,
// deletes, moves, renames, or icon changes. The server document handler
// satisfies this interface via its NotifyTreeChange method.
//
//go:generate ../../../scripts/codegen/mock -t internal TreeNotifier tree_notifier
type TreeNotifier interface {
	// NotifyTreeChange should tell subscribers that the tree under
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
