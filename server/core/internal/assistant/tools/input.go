package tools

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// Deps carries the wiring a tool set is built from: the services every
// tool reaches through and the (organization, user) pair every call is
// scoped to.
//
// It is built once per session. The per-call Input a tool actually sees
// is assembled from it by the eino adapter, which is the only thing
// that knows a call's context and arguments.
type Deps struct {
	// log is scoped to the session's (org, user) and used to record
	// per-tool outcomes so we can diagnose AI loops without
	// re-running the conversation.
	log *slog.Logger

	// db is the persistence used by read tools and the non-content
	// write tools (create/delete/move).
	db DB

	// search is the full-text index behind search_documents.
	search Searcher

	// jobs is the way into the search-job queue: it decides whether a
	// deployment indexes at all, so writes queue through it instead of
	// hitting the database directly.
	jobs *search.Jobs

	// runners hands out the runner a data-source tool reads through.
	runners DataSourceRunners

	// applier is the edit client for content mutations and the
	// rename/set-icon ops that must propagate to connected editors.
	applier EditApplier

	// tree notifies tree-change subscribers after the assistant
	// mutates the document tree.
	tree TreeNotifier

	// offload retrieves results parked outside the conversation.
	offload OffloadReader

	// orgID scopes every tool call to one organization.
	orgID string

	// userID identifies the user the assistant is acting for. Used
	// when a tool creates audit-relevant rows (the created_by fields
	// on a new document, for instance).
	userID string
}

// NewDeps creates a fresh instance of Deps. Every dependency is
// required; nil values surface as nil-pointer panics on the first tool
// call rather than at startup, but in practice the cmd-level wiring
// passes all of them.
func NewDeps(
	log *slog.Logger,
	db DB,
	searcher Searcher,
	jobs *search.Jobs,
	runners DataSourceRunners,
	applier EditApplier,
	tree TreeNotifier,
	offload OffloadReader,
	orgID, userID string,
) *Deps {
	return &Deps{
		log: log.With(
			"component", "assistant-tools",
			"org_id", orgID,
			"user_id", userID,
		),
		db:      db,
		search:  searcher,
		jobs:    jobs,
		runners: runners,
		applier: applier,
		tree:    tree,
		offload: offload,
		orgID:   orgID,
		userID:  userID,
	}
}

// input is one tool call: the session's wiring plus the context and
// arguments of the call being served. It satisfies both DescribeInput
// and Input; which of the two a tool receives is decided by the method
// being called, not by what is in here.
type input struct {
	*Deps

	// name is the tool being called, so a rejected argument can say
	// which tool rejected it.
	name Name

	// ctx is the context of this call. It carries the agent session
	// values a tool may consult, so it belongs to the call rather than
	// to the session.
	ctx context.Context //nolint:containedctx // the input is the call, and is rebuilt per call

	// args is the raw JSON the model supplied.
	args json.RawMessage

	// touched lists the documents this call changed, in the order the
	// writes below recorded them.
	touched []xid.ID
}

// recordTouched notes a document this call changed. Every write in this
// package goes through one of the four methods below, so recording it
// here is what makes Result.Documents right by construction rather than
// by a convention about argument names.
func (i *input) recordTouched(documentID xid.ID) {
	if documentID.IsNil() || slices.Contains(i.touched, documentID) {
		return
	}

	i.touched = append(i.touched, documentID)
}

// newInput creates a fresh instance of input for one tool call.
func (d *Deps) newInput(ctx context.Context, name Name, args json.RawMessage) *input {
	return &input{Deps: d, name: name, ctx: ctx, args: args}
}

// Context returns the context of the call being served.
func (i *input) Context() context.Context {
	return i.ctx
}

// Decode decodes the call's arguments into dst and validates them,
// naming the tool that rejected them.
//
// Decoding uses json/v2 because its errors name the argument they
// failed on. A domain type that parses itself — an id, a timestamp, an
// enum — reports only that the value is bad; the path json/v2 adds is
// what makes the message actionable for the model.
func (i *input) Decode(dst Args) error {
	if err := jsonv2.Unmarshal(i.args, dst); err != nil {
		return fmt.Errorf("%s: invalid input: %w", i.name, err)
	}

	if err := dst.Validate(); err != nil {
		return fmt.Errorf("%s: %w", i.name, err)
	}

	return nil
}

// OrganizationID returns the organisation every call is scoped to.
func (i *input) OrganizationID() string {
	return i.orgID
}

// UserID returns the user the assistant is acting for.
func (i *input) UserID() string {
	return i.userID
}

// DataSource returns the data source the id names.
//
// The lookup is the cross-org safety check: FetchDataSource scopes by
// organisation, so an id belonging to another one is as absent as an id
// belonging to nobody, and the model is told the same thing either way.
func (i *input) DataSource(dataSourceID xid.ID) (*datasource.DataSource, error) {
	ds, err := i.db.FetchDataSource(i.ctx, dataSourceID, i.orgID)
	if err != nil {
		return nil, errUnknownDataSource
	}

	return ds, nil
}

// DataSources returns every data source the organisation owns.
func (i *input) DataSources() ([]datasource.DataSource, error) {
	return i.db.FetchDataSources(i.ctx, i.orgID)
}

// CheckDataSources refuses a write naming a data source the
// organisation does not own.
//
// It sits with the write rather than with the schema because it is the
// only check that needs the database: block.Validate can say the id is
// a string, but only a lookup can say it addresses something, and a
// metric block pointing at nothing renders as a broken chart the user
// then has to fix by hand. The ids are block content, not arguments, so
// they arrive as the strings the document stores and are parsed here.
func (i *input) CheckDataSources(ids []string) error {
	for _, raw := range ids {
		id, err := xid.FromString(raw)
		if err != nil {
			return fmt.Errorf("metric %s %q: %w", document.AttrDataSourceID, raw, err)
		}

		if _, err := i.DataSource(id); err != nil {
			return fmt.Errorf("metric %s %q: %w", document.AttrDataSourceID, raw, err)
		}
	}

	return nil
}

// DataSourceRunner returns the runner that reads the data source the id
// names.
//
// The lookup is the cross-org safety check: FetchDataSource scopes by
// organisation, so an id belonging to another one is as absent as an id
// belonging to nobody, and the model is told the same thing either way.
func (i *input) DataSourceRunner(id xid.ID) (datasource.Runner, error) {
	ds, err := i.db.FetchDataSource(i.ctx, id, i.orgID)
	if err != nil {
		return nil, errUnknownDataSource
	}

	return i.runners.Runner(*ds), nil
}

// Document returns the document's default branch. An id naming nothing
// in the session's organisation is reported as such rather than as the
// driver's own words, which say nothing a caller can act on.
func (i *input) Document(id xid.ID) (*document.Document, error) {
	doc, err := i.db.FetchDocument(i.ctx, id, i.orgID, document.DefaultBranch)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, errUnknownDocument
		}

		return nil, err
	}

	return doc, nil
}

// DocumentContent returns the document's parsed main-branch content.
func (i *input) DocumentContent(id xid.ID) (document.Content, error) {
	return i.db.FetchMainBranchContent(i.ctx, id, i.orgID)
}

// DescendantCount reports how many documents sit under the named one at
// any depth. A document that is not in the tree has none, which is what
// a caller asking about a missing id should hear.
func (i *input) DescendantCount(id xid.ID) (int, error) {
	tree, err := i.DocumentTree()
	if err != nil {
		return 0, fmt.Errorf("fetching document tree: %w", err)
	}

	for _, s := range tree.Descendants() {
		if s.ID == id {
			return len(s.Children.Descendants()), nil
		}
	}

	return 0, nil
}

// DocumentTree returns every document in the organisation as a nested
// summary tree.
func (i *input) DocumentTree() (document.Summaries, error) {
	return i.db.FetchDocumentTree(i.ctx, i.orgID)
}

// DocumentChildren returns the direct children of parentID.
func (i *input) DocumentChildren(parentID null.Value[xid.ID]) (document.Summaries, error) {
	return i.db.FetchDocumentTreeByDocumentParentID(i.ctx, parentID, i.orgID)
}

// CreateDocument inserts the document, its maintainer row and its
// search job.
//
// The three writes go together: a half-created document the model then
// retries leaves two of them behind, one without a maintainer and
// invisible to search. The parent is checked here rather than by the
// caller, so the invariant travels with the write that depends on it.
func (i *input) CreateDocument(doc document.Document) error {
	if doc.ParentID.Valid {
		if err := i.db.CheckDocumentExists(i.ctx, doc.ParentID.V, i.orgID); err != nil {
			return fmt.Errorf("parent not found: %w", err)
		}
	}

	var tx Tx

	if err := i.db.BeginTx(i.ctx, &tx); err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := tx.InsertDocument(i.ctx, doc); err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	// mirror the HTTP create path: whoever asked for the document
	// becomes its first maintainer so they own it from the start.
	if err := tx.UpsertDocumentMaintainers(i.ctx, doc.ID, i.orgID, []string{i.userID}); err != nil {
		return fmt.Errorf("upsert maintainers: %w", err)
	}

	// without this the document is invisible to search until someone
	// edits it, since only the persist path queues a job.
	if err := i.jobs.Enqueue(i.ctx, tx, search.BlocksDiff(nil, doc.Search())); err != nil {
		return fmt.Errorf("insert search job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.recordTouched(doc.ID)

	return nil
}

// errUnknownDocument is what DeleteDocument reports for an id that
// names nothing in the session's organisation. Another organisation's
// id lands here too, which is the point: the tools cannot be used to
// discover that a document exists elsewhere.
var errUnknownDocument = errors.New("no document with that id in this organisation; call list_documents for the ids that exist")

// DeleteDocument removes the document. It records nothing as touched:
// the document is gone, so there is nothing left to point a caller at.
// The delete reports the ids of the destroyed subtree, and their
// search-index removal is queued in the same transaction: after the
// commit nothing else knows what went away. An empty subtree means the
// delete matched nothing, which is reported as an error rather than a
// silent success.
func (i *input) DeleteDocument(id xid.ID) error {
	var tx Tx

	if err := i.db.BeginTx(i.ctx, &tx); err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	ids, err := tx.DeleteDocument(i.ctx, id, i.orgID)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return errUnknownDocument
	}

	if err := i.jobs.Enqueue(i.ctx, tx, search.BlocksDifference{
		RemovedDocuments: ids,
	}); err != nil {
		return fmt.Errorf("insert search job: %w", err)
	}

	return tx.Commit()
}

// MoveDocument re-parents the document.
//
// The destination is validated here rather than by the caller: a move
// that lands under a missing parent, or under the document's own
// subtree, is a broken tree, and the check belongs with the write that
// would cause it.
func (i *input) MoveDocument(id xid.ID, parentID null.Value[xid.ID]) error {
	if parentID.Valid {
		if err := i.db.CheckDocumentExists(i.ctx, parentID.V, i.orgID); err != nil {
			return fmt.Errorf("new parent not found: %w", err)
		}

		cycle, err := i.db.CheckDocumentCycle(i.ctx, id, parentID.V, i.orgID)
		if err != nil {
			return fmt.Errorf("parent check: %w", err)
		}

		if cycle {
			return errors.New("a document cannot be moved under itself or one of its descendants")
		}
	}

	if err := i.db.UpdateDocumentParentID(i.ctx, id, parentID, i.orgID); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	// the parents' own content is unchanged — only the tree shape
	// around them — so the moved document is the one to record.
	i.recordTouched(id)

	return nil
}

// SearchBlocks returns blocks whose text matches the query.
func (i *input) SearchBlocks(query string, limit int) ([]search.Block, error) {
	return i.search.SearchDocumentBlocks(i.ctx, i.orgID, query, limit)
}

// Warn records something a tool carried on through.
func (i *input) Warn(msg string, attrs ...slog.Attr) {
	i.log.LogAttrs(i.ctx, slog.LevelWarn, msg,
		append([]slog.Attr{slog.String("tool", string(i.name))}, attrs...)...)
}

// ReadOffloaded returns a tool result that was moved out of the
// conversation for size.
func (i *input) ReadOffloaded(path string) (string, error) {
	return i.offload.Read(i.ctx, path)
}

// errBranchProtected is what a content write reports for a document
// whose branch is protected. The message names the tool the caller
// still has, since reading a protected document stays allowed.
var errBranchProtected = errors.New("this document's branch is protected and takes no edits; read it with read_document_summary or ask someone with access to unprotect it")

// resolveDoc loads the default branch of the given document and returns
// the ids the edit client needs. The lookup also acts as the cross-org
// safety check — Document scopes by orgID so a docID from another
// organisation surfaces as NotFound.
func (i *input) resolveDoc(documentID xid.ID) (docRef, error) {
	doc, err := i.Document(documentID)
	if err != nil {
		return docRef{}, fmt.Errorf("fetching document: %w", err)
	}

	// a protected branch takes no write but core's own, and this is not
	// one: the operations endpoint applies the batch to the live Y.Doc
	// regardless, and only the persist behind it is refused. Without
	// this the call reports success, the change shows up in every open
	// editor, and it is gone at the next load.
	if doc.Protected {
		return docRef{}, errBranchProtected
	}

	return docRef{DocumentID: doc.ID, BranchID: doc.BranchID}, nil
}

// ApplyEdit is the shared tail of every content-mutating write tool: it
// resolves the document to a (documentID, branchID) pair, ships the
// operation batch to Node, and surfaces the per-op result. Outcomes are
// logged so partial failures on the Node side (uid not found, malformed
// block) are visible without re-running the conversation.
func (i *input) ApplyEdit(documentID xid.ID, ops []edit.Operation) (string, error) {
	ref, err := i.resolveDoc(documentID)
	if err != nil {
		i.log.Warn(
			"edit resolve failed",
			slog.String("document_id", documentID.String()),
			slog.String("error", err.Error()),
		)

		return "", err
	}

	res, err := i.applier.Apply(i.ctx, ref.DocumentID, ref.BranchID, ops, false)
	if err != nil {
		i.log.Error(
			"edit apply failed",
			slog.String("document_id", ref.DocumentID.String()),
			slog.String("branch_id", ref.BranchID.String()),
			slog.Int("op_count", len(ops)),
			slog.String("error", err.Error()),
		)

		return "", fmt.Errorf("applying edit: %w", err)
	}

	// nothing committed and something to say why is a failed call, not
	// a result describing a no-op: every write tool ships a single
	// operation, so there is no partial success to report. Returning an
	// error is what makes the surfaces treat it as one — the MCP bridge
	// marks the result isError, and the assistant sees a failure it can
	// correct rather than a success it will summarise.
	if res.Applied == 0 && len(res.Errors) > 0 {
		return "", fmt.Errorf("applying edit: %s", joinOpErrors(res.Errors))
	}

	if len(res.Errors) > 0 {
		i.log.Warn(
			"edit partial failure",
			slog.String("document_id", ref.DocumentID.String()),
			slog.String("branch_id", ref.BranchID.String()),
			slog.Int("applied", res.Applied),
			slog.Any("errors", res.Errors),
		)
	} else {
		i.log.Debug(
			"edit applied",
			slog.String("document_id", ref.DocumentID.String()),
			slog.String("branch_id", ref.BranchID.String()),
			slog.Int("applied", res.Applied),
		)
	}

	// the resolved id, not the argument: the caller may have named the
	// document any way resolveDoc accepts.
	i.recordTouched(ref.DocumentID)

	return result(res)
}

// ValidatePlacement validates a block that is about to land next to, or
// in place of, the block referenceUID names. The reference's parent is
// what decides legality — the document root takes the root set, a macro
// container takes its own — so the check resolves the reference in the
// fetched content and validates against that container. The edit
// backend applies no schema of its own, so an illegal type let through
// here would land in the Y.Doc unchallenged.
func (i *input) ValidatePlacement(documentID xid.ID, referenceUID string, b block.Block) error {
	content, err := i.DocumentContent(documentID)
	if err != nil {
		return fmt.Errorf("fetching content: %w", err)
	}

	parent, ok := content.Content.FindParentTypeByUID(referenceUID)
	if !ok {
		// the reference does not resolve; the edit backend reports
		// that as the call's result. Still reject a malformed block.
		return block.Validate(b)
	}

	return block.ValidateInContainer(parent, b)
}

// ValidateAttrUpdate checks the attributes an update would leave on the
// block. The update names some attributes and preserves the rest, so
// the rules run against the merge rather than the payload — otherwise
// setting one attribute would be judged as if every other were absent.
//
// A block whose ProseMirror type has no canonical counterpart is a
// wrapper item (a list item, a macro internal). Those carry attributes
// the canonical model does not describe, so there is nothing to check.
func (i *input) ValidateAttrUpdate(documentID xid.ID, blockUID string, attrs map[string]any) error {
	content, err := i.DocumentContent(documentID)
	if err != nil {
		return fmt.Errorf("fetching content: %w", err)
	}

	target, ok := content.Content.FindByUID(blockUID)
	if !ok {
		// the uid does not resolve; the edit backend reports that as
		// the call's result.
		return nil
	}

	t, ok := block.CanonicalType(target.Type)
	if !ok {
		return nil
	}

	merged := make(document.Attributes, len(target.Attrs)+len(attrs))
	maps.Copy(merged, target.Attrs)
	maps.Copy(merged, attrs)

	return block.ValidateAttrs(t, merged)
}

// ValidateMove checks that the moved block may live where the move
// would put it: among the children of whatever holds the reference.
// Only placement is in question — the block is already in the document,
// so its content has been through validation once already and may
// contain shapes this layer can no longer express.
//
// A wrapper item (a list item, a macro internal) has no canonical type
// to check against a container's allowed set, so it is held to a
// narrower rule instead: it may only land in a container of the kind it
// already sits in, which permits reordering and moving between two
// lists while keeping a list item from landing at the document root.
func (i *input) ValidateMove(documentID xid.ID, blockUID, referenceUID string) error {
	content, err := i.DocumentContent(documentID)
	if err != nil {
		return fmt.Errorf("fetching content: %w", err)
	}

	moved, ok := content.Content.FindByUID(blockUID)
	if !ok {
		// neither uid resolving is the edit backend's report to make.
		return nil
	}

	target, ok := content.Content.FindParentTypeByUID(referenceUID)
	if !ok {
		return nil
	}

	t, ok := block.CanonicalType(moved.Type)
	if !ok {
		source, ok := content.Content.FindParentTypeByUID(blockUID)
		if !ok || source == target {
			return nil
		}

		return fmt.Errorf(
			"%s cannot move from %s into %s; it belongs to the block that holds it",
			moved.Type, source, target,
		)
	}

	return block.AllowedInContainer(target, t)
}

// NotifyTreeChange invokes the tree notifier when one is configured.
// Safe to call from any tool — a nil notifier silently no-ops so tests
// that don't wire one don't trip.
func (i *input) NotifyTreeChange(parentID null.Value[xid.ID]) {
	if i.tree == nil {
		return
	}

	i.tree.NotifyTreeChange(i.orgID, parentID)
}

// NotifyTreeChangeForDocument looks up the document's current parent
// and fires a tree-change for that parent. Used by rename/icon ops
// which don't carry a parent in their args. Failures (e.g. doc fetched
// after delete) silently skip the notification.
func (i *input) NotifyTreeChangeForDocument(documentID xid.ID) {
	doc, err := i.Document(documentID)
	if err != nil || doc == nil {
		return
	}

	i.NotifyTreeChange(doc.ParentID)
}

// docRef wraps the (documentID, branchID) pair the edit client needs to
// address a live Y.Doc. The branch is resolved to the document's
// default branch — multi-branch editing is out of scope for the
// assistant.
type docRef struct {
	// DocumentID is the document's id.
	DocumentID xid.ID

	// BranchID is the id of the document's default branch.
	BranchID xid.ID
}

// DataSourceRunners hands out the runner for a data source. The
// datasource package's Manager satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t internal DataSourceRunners data_source_runners
type DataSourceRunners interface {
	// Runner should return the runner that operates the given data
	// source.
	Runner(ds datasource.DataSource) datasource.Runner
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

	// FetchDataSource should return a data source by id within the org.
	// Used by every data-source tool to resolve what it was asked about.
	FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasource.DataSource, error)

	// FetchDataSources should return every data source the org owns.
	// Used by list_data_sources.
	FetchDataSources(ctx context.Context, organizationID string) ([]datasource.DataSource, error)
}

// Tx is the transactional half of DB, so a tool whose write spans
// tables can commit or abandon all of it at once.
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
	// document. Used by create_document and delete_document.
	InsertDocumentSearchJob(ctx context.Context, diff search.BlocksDifference) error

	// DeleteDocument should remove a document and report the ids of the
	// document and of every cascade-deleted descendant. Used by
	// delete_document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) ([]xid.ID, error)
}

// Searcher is the full-text search surface search_documents uses.
// The document/search Meilisearch client satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both Searcher searcher
type Searcher interface {
	// Configured should report whether search is configured on this
	// deployment.
	Configured() bool

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
	// outcome. A tool's writes are a person's, never core's own, so
	// this package always asks for an ordinary one.
	Apply(ctx context.Context, documentID, branchID xid.ID, ops []edit.Operation, system bool) (edit.Result, error)
}
