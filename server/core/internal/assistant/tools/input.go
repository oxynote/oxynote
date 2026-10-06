package tools

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// ErrUnknownTag is what a tag lookup reports for an id that names
// nothing in the session's organisation. Another organisation's id lands
// here too, for the same reason as ErrUnknownDocument.
var ErrUnknownTag = errors.New("no tag with that id in this organisation; call list_tags for the ids that exist")

// errUnknownHook is what a hook lookup reports for an id the addressed
// document does not hold. A hook of another document, another
// organisation or a deleted document lands here too: the tools cannot
// be used to reach a hook through a document it is not on.
var errUnknownHook = errors.New("no hook with that id on this document; call list_hooks for the ids it has")

// Deps carries the wiring a tool set is built from: the services every
// tool reaches through and the (organization, user) pair every call is
// scoped to.
//
// It is built once per session. The per-call input a tool actually sees
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

	// searchTrigger runs the search-job worker once a write that queued a
	// job has committed.
	searchTrigger SearchTrigger

	// runners hands out the runner a data-source tool reads through.
	runners DataSourceRunners

	// githubMan tells create_hook whether the GitHub App is configured.
	githubMan *github.Manager

	// webchangeClient tells create_hook whether changedetection.io is
	// configured.
	webchangeClient *webchange.Client

	// hookMan runs the hook writes.
	hookMan HookManager

	// applier is the edit client for content mutations and the
	// rename/set-icon ops that must propagate to connected editors.
	applier EditApplier

	// tree notifies tree-change subscribers after the assistant
	// mutates the document tree.
	tree TreeNotifier

	// tags notifies tag-tree subscribers after the assistant changes
	// a tag or what carries it.
	tags TagNotifier

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
	searchTrigger SearchTrigger,
	runners DataSourceRunners,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	hookMan HookManager,
	applier EditApplier,
	tree TreeNotifier,
	tags TagNotifier,
	offload OffloadReader,
	orgID, userID string,
) *Deps {
	return &Deps{
		log: log.With(
			"component", "assistant-tools",
			"org_id", orgID,
			"user_id", userID,
		),
		db:              db,
		search:          searcher,
		searchTrigger:   searchTrigger,
		runners:         runners,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		hookMan:         hookMan,
		applier:         applier,
		tree:            tree,
		tags:            tags,
		offload:         offload,
		orgID:           orgID,
		userID:          userID,
	}
}

// input is one tool call: the session's wiring plus the context and
// arguments of the call being served. Execute receives it whole; Title
// and Summary receive it as a DescribeInput, which reaches nothing that
// writes.
type input struct {
	*Deps

	// name is the tool being called, so a warning names it.
	name Name

	// ctx is the context of this call. It carries the agent session
	// values a tool may consult, so it belongs to the call rather than
	// to the session.
	ctx context.Context //nolint:containedctx // the input is the call, and is rebuilt per call

	// args is the raw JSON the model supplied.
	args json.RawMessage

	// branches holds the branches this call has fetched, by branch id.
	// A call reads its branch in several steps, and they share one
	// fetch.
	branches map[xid.ID]*document.Document
}

// newInput creates a fresh instance of input for one tool call.
func (d *Deps) newInput(ctx context.Context, name Name, args json.RawMessage) *input {
	return &input{Deps: d, name: name, ctx: ctx, args: args}
}

// Context returns the context of the call being served.
func (i *input) Context() context.Context {
	return i.ctx
}

// Decode decodes the call's arguments into dst and validates them.
//
// Decoding uses json/v2 because its errors name the argument they
// failed on. A domain type that parses itself — an id, a timestamp, an
// enum — reports only that the value is bad; the path json/v2 adds is
// what makes the message actionable for the model.
//
// A provider calling a tool that declares no parameters may send no
// arguments at all rather than an empty object, so an empty payload
// reads as one; Validate still decides whether that is acceptable.
func (i *input) Decode(dst Args) error {
	args := i.args
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}

	if err := jsonv2.Unmarshal(args, dst); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	return dst.Validate()
}

// FetchDataSource returns the data source the id names.
//
// The lookup is the cross-org safety check: FetchDataSource scopes by
// organisation, so an id belonging to another one is as absent as an id
// belonging to nobody, and the model is told the same thing either way.
func (i *input) FetchDataSource(dataSourceID xid.ID) (*datasource.DataSource, error) {
	ds, err := i.db.FetchDataSource(i.ctx, dataSourceID, i.orgID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, errUnknownDataSource
		}

		return nil, fmt.Errorf("fetching data source: %w", err)
	}

	return ds, nil
}

// FetchDataSources returns every data source the organisation owns.
func (i *input) FetchDataSources() ([]datasource.DataSource, error) {
	return i.db.FetchDataSources(i.ctx, i.orgID)
}

// CheckDataSources refuses blocks, nested ones included, that name a
// data source the organisation does not own.
//
// It sits with the write rather than with the schema because it is the
// only check that needs the database: markup.Build can say the id is
// a string, but only a lookup can say it addresses something, and a
// metric block pointing at nothing renders as a broken chart the user
// then has to fix by hand. The ids are the strings the blocks store, so
// they are parsed here.
func (i *input) CheckDataSources(blocks []document.Block) error {
	for _, b := range blocks {
		if raw := b.Attrs.Get(document.AttrDataSourceID).String(); raw != "" {
			id, err := xid.FromString(raw)
			if err != nil {
				return fmt.Errorf("metric %s %q: %w", document.AttrDataSourceID, raw, err)
			}

			if _, err := i.FetchDataSource(id); err != nil {
				return fmt.Errorf("metric %s %q: %w", document.AttrDataSourceID, raw, err)
			}
		}

		if err := i.CheckDataSources(b.Content); err != nil {
			return err
		}
	}

	return nil
}

// DataSourceRunner returns the runner that reads the data source the id
// names, looked up through FetchDataSource.
func (i *input) DataSourceRunner(id xid.ID) (datasource.Runner, error) {
	ds, err := i.FetchDataSource(id)
	if err != nil {
		return nil, err
	}

	return i.runners.Runner(*ds), nil
}

// FetchDocument returns the document on its default branch. An id naming
// nothing in the session's organisation is reported as such rather
// than in the driver's own words, which say nothing a caller can act
// on.
func (i *input) FetchDocument(documentID xid.ID) (*document.Document, error) {
	doc, err := i.db.FetchDocument(i.ctx, documentID, i.orgID, document.DefaultBranch)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, ErrUnknownDocument
		}

		return nil, err
	}

	return doc, nil
}

// FetchBranch returns the document on the branch branchID names. A branch id
// resolves on its own, so one belonging to another document is refused
// as unknown to this one, and an unknown branch is reported with the
// ones the document does have.
func (i *input) FetchBranch(documentID, branchID xid.ID) (*document.Document, error) {
	if doc, ok := i.branches[branchID]; ok && doc.ID == documentID {
		return doc, nil
	}

	doc, err := i.db.FetchDocumentByBranchID(i.ctx, branchID, i.orgID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, i.unknownBranch(documentID, branchID)
		}

		return nil, err
	}

	if doc.ID != documentID {
		return nil, i.unknownBranch(documentID, branchID)
	}

	if i.branches == nil {
		i.branches = map[xid.ID]*document.Document{}
	}

	i.branches[branchID] = doc

	return doc, nil
}

// unknownBranch tells a branch the document does not have from a
// document the organisation does not have: the branch list is empty
// only for the latter, since every document has at least one.
func (i *input) unknownBranch(documentID, branchID xid.ID) error {
	branches, err := i.FetchDocumentBranches(documentID)
	if err != nil {
		return err
	}

	return fmt.Errorf("branch %s: %w; the branches are %s", branchID, ErrUnknownBranch, branchLabels(branches))
}

// FetchDocumentBranches lists every branch of the document. A document the
// organisation does not have lists no branches, and is reported as
// unknown rather than as branchless.
func (i *input) FetchDocumentBranches(documentID xid.ID) ([]document.BranchSummary, error) {
	branches, err := i.db.FetchDocumentBranches(i.ctx, documentID, i.orgID)
	if err != nil {
		return nil, fmt.Errorf("fetching branches: %w", err)
	}

	if len(branches) == 0 {
		return nil, ErrUnknownDocument
	}

	return branches, nil
}

// FetchDocumentBlock finds one block of the branch by uid, in the
// persisted content.
func (i *input) FetchDocumentBlock(documentID, branchID xid.ID, blockUID string) (document.Block, error) {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return document.Block{}, fmt.Errorf("fetching content: %w", err)
	}

	b, ok := doc.Content.FindByUID(blockUID)
	if !ok {
		return document.Block{}, fmt.Errorf("block %s: %w", blockUID, errUnknownBlock)
	}

	return b, nil
}

// DescendantCount reports how many documents sit under the named one at
// any depth. A document that is not in the tree has none, which is what
// a caller asking about a missing id should hear.
func (i *input) DescendantCount(id xid.ID) (int, error) {
	tree, err := i.FetchDocumentTree()
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

// FetchDocumentTree returns every document in the organisation as a nested
// summary tree.
func (i *input) FetchDocumentTree() (document.Summaries, error) {
	return i.db.FetchDocumentTree(i.ctx, i.orgID)
}

// FetchDocumentChildren returns the direct children of parentID.
func (i *input) FetchDocumentChildren(parentID null.Value[xid.ID]) (document.Summaries, error) {
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
			if errutil.IsNotFound(err) {
				return fmt.Errorf("parent %s: %w", doc.ParentID.V, errUnknownParent)
			}

			return fmt.Errorf("checking parent: %w", err)
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
	if err := tx.InsertSearchJob(i.ctx, search.BranchScope(i.orgID, doc.ID, doc.BranchID)); err != nil {
		return fmt.Errorf("insert search job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.searchTrigger.Trigger()
	i.notifyTreeChange(doc.ParentID)

	return nil
}

// errCyclicParent is what a move reports when the new parent is the
// document itself or sits under it.
var errCyclicParent = errors.New("a document cannot be moved under itself or one of its descendants; pick a parent outside its subtree")

// errUnknownBlock is what a block read or write reports for a uid the
// document does not hold. The message names the read that lists the
// current uids, since a stale uid is the usual cause.
var errUnknownBlock = errors.New("no block with that uid in this document; call get_document for the current uids")

// errUnknownParent is what a create or move reports for a parent id
// that names no document in the organisation. It says where the ids the
// model may use come from.
var errUnknownParent = errors.New("no document with that id to use as parent; call list_documents for the ids that exist")

// ErrUnknownDocument is what a document lookup reports for an id that
// names nothing in the session's organisation. Another organisation's
// id lands here too, which is the point: the tools cannot be used to
// discover that a document exists elsewhere.
var ErrUnknownDocument = errors.New("no document with that id in this organisation; call list_documents for the ids that exist")

// DeleteDocument removes the document. The delete reports the ids of the
// destroyed subtree, and their search-index removal is queued in the
// same transaction: after the commit nothing else knows what went away.
// An empty subtree means the delete matched nothing, which is reported
// as an error rather than a silent success.
func (i *input) DeleteDocument(id xid.ID) error {
	// the parent is read before the row goes, to tell the subtree that
	// changed. A document that cannot be read leaves the root's.
	var parentID null.Value[xid.ID]

	if doc, err := i.FetchDocument(id); err == nil {
		parentID = doc.ParentID
	}

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
		return ErrUnknownDocument
	}

	for _, id := range ids {
		if err := tx.InsertSearchJob(i.ctx, search.DocumentScope(i.orgID, id)); err != nil {
			return fmt.Errorf("insert search job: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.searchTrigger.Trigger()
	i.notifyTreeChange(parentID)

	return nil
}

// MoveDocument re-parents the document doc describes.
//
// The destination is validated here rather than by the caller: a move
// that lands under a missing parent, or under the document's own
// subtree, is a broken tree, and the check belongs with the write that
// would cause it.
func (i *input) MoveDocument(doc *document.Document, parentID null.Value[xid.ID]) error {
	if parentID.Valid { //nolint:nestif // the branching is sequential and readable
		if err := i.db.CheckDocumentExists(i.ctx, parentID.V, i.orgID); err != nil {
			if errutil.IsNotFound(err) {
				return fmt.Errorf("new parent %s: %w", parentID.V, errUnknownParent)
			}

			return fmt.Errorf("checking parent: %w", err)
		}

		cycle, err := i.db.CheckDocumentCycle(i.ctx, doc.ID, parentID.V, i.orgID)
		if err != nil {
			return fmt.Errorf("parent check: %w", err)
		}

		if cycle {
			return errCyclicParent
		}
	}

	if err := i.db.UpdateDocumentParentID(i.ctx, doc.ID, parentID, i.orgID); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	// the move changes the shape of both subtrees; when they are the
	// same one, one notification covers it.
	if doc.ParentID != parentID {
		i.notifyTreeChange(doc.ParentID)
	}

	i.notifyTreeChange(parentID)

	return nil
}

// RenameDocument sets the document's name and icon through the realtime
// service, so an open editor shows them as they land. An empty name or
// icon stays as it is.
func (i *input) RenameDocument(doc *document.Document, name, icon string) error {
	var ops []edit.Operation

	if name != "" {
		ops = append(ops, edit.SetName(name))
	}

	if icon != "" {
		ops = append(ops, edit.SetIcon(icon))
	}

	if len(ops) == 0 {
		return nil
	}

	if err := i.ApplyEdit(doc.ID, doc.BranchID, ops); err != nil {
		return err
	}

	i.notifyTreeChange(doc.ParentID)

	return nil
}

// Warn records something a tool carried on through.
func (i *input) Warn(msg string, attrs ...slog.Attr) {
	i.log.LogAttrs(i.ctx, slog.LevelWarn, msg,
		append([]slog.Attr{slog.String("tool", string(i.name))}, attrs...)...)
}

// errBranchProtected is what a content write reports for a branch that
// is protected. Reading it stays allowed; the caller is told which of
// the document's branches would take the write instead.
var errBranchProtected = errors.New("this branch is protected and takes no edits")

// ErrUnknownBranch is what a branch lookup reports for an id the
// document does not have. The document's branches travel with it, since
// a stale or guessed id is the usual cause.
var ErrUnknownBranch = errors.New("this document has no branch with that id")

// protectedBranch builds the refusal for a write to a protected branch,
// naming the document's unprotected branches so the caller has
// somewhere to write, or saying there is none.
func (i *input) protectedBranch(doc *document.Document) error {
	branches, err := i.FetchDocumentBranches(doc.ID)
	if err != nil {
		return fmt.Errorf("branch %s: %w; %w", doc.BranchName, errBranchProtected, err)
	}

	open := slices.DeleteFunc(branches, func(b document.BranchSummary) bool { return b.Protected })
	if len(open) == 0 {
		return fmt.Errorf("branch %s: %w; the document has no unprotected branch to write to", doc.BranchName, errBranchProtected)
	}

	return fmt.Errorf("branch %s: %w; write to one of %s", doc.BranchName, errBranchProtected, branchLabels(open))
}

// branchLabels lists the branches, each as "name (id)": the name is what
// a person calls it and the id is what a tool call takes.
func branchLabels(branches []document.BranchSummary) string {
	labels := make([]string, 0, len(branches))

	for _, b := range branches {
		labels = append(labels, fmt.Sprintf("%s (%s)", b.BranchName, b.BranchID))
	}

	return strings.Join(labels, ", ")
}

// ApplyEdit is the shared tail of every content-mutating write tool: it
// loads the branch, ships the operation batch to Node, and returns the
// operation Node refused, if any, in which case nothing was applied. What
// the write produced is the tool's own to describe, which spares a second
// fetch of the document. The branch lookup is scoped to the organisation,
// so a document of another one is refused as unknown.
func (i *input) ApplyEdit(documentID, branchID xid.ID, ops []edit.Operation) error {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return fmt.Errorf("fetching document: %w", err)
	}

	// a protected branch takes no write but core's own, and this is not
	// one: the operations endpoint applies the batch to the live Y.Doc
	// regardless, and only the persist behind it is refused. Without
	// this the call reports success, the change shows up in every open
	// editor, and it is gone at the next load.
	if doc.Protected {
		return i.protectedBranch(doc)
	}

	res, err := i.applier.Apply(
		i.ctx,
		documentID,
		branchID,
		ops,
		i.userID,
		false,
	)
	if err != nil {
		return fmt.Errorf("applying edit: %w", err)
	}

	// an operation refused is a failed call: the MCP bridge then marks
	// the result isError, and the assistant sees a failure it can
	// correct. A batch applies whole or not at all, so nothing landed.
	if err := res.Err(len(ops)); err != nil {
		return fmt.Errorf("applying edit: %w", err)
	}

	i.log.Debug(
		"edit applied",
		slog.String("document_id", documentID.String()),
		slog.String("branch_id", branchID.String()),
		slog.Int("op_count", len(ops)),
	)

	return nil
}

// notifyTreeChange invokes the tree notifier when one is configured.
// Safe to call from any tool — a nil notifier silently no-ops so tests
// that don't wire one don't trip.
func (i *input) notifyTreeChange(parentID null.Value[xid.ID]) {
	if i.tree == nil {
		return
	}

	i.tree.NotifyTreeChange(i.orgID, parentID)
}

// FetchTagTree returns every tag in the organisation in display order, each
// with the documents whose default branch carries it and whether the
// session's user hides it.
func (i *input) FetchTagTree() (tag.Summaries, error) {
	return i.db.FetchTagTree(i.ctx, i.orgID, i.userID)
}

// FetchTag returns the tag the id names, with the documents carrying it. The
// tree is the one read that carries a tag's name, colour and documents
// together, so a lookup walks it rather than asking for a second shape.
func (i *input) FetchTag(tagID xid.ID) (*tag.Summary, error) {
	tree, err := i.FetchTagTree()
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	for _, s := range tree {
		if s.ID == tagID {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("tag %s: %w", tagID, ErrUnknownTag)
}

// FetchBranchTags returns the tags the branch branchID names carries, in the
// tags' display order.
func (i *input) FetchBranchTags(documentID, branchID xid.ID) ([]tag.Tag, error) {
	return i.db.FetchBranchTags(i.ctx, i.orgID, documentID, branchID)
}

// CreateTag inserts the tag at the end of the organisation's tags. A
// name the organisation already uses is refused in the repository's own
// words, which name the clash.
func (i *input) CreateTag(t tag.Tag) error {
	if err := i.db.InsertTag(i.ctx, t); err != nil {
		return err
	}

	i.notifyTagTreeChange()

	return nil
}

// UpdateTag renames and/or recolours the tag the id names.
func (i *input) UpdateTag(tagID xid.ID, inp tag.UpdateInput) error {
	if err := i.db.UpdateTag(i.ctx, i.orgID, tagID, inp); err != nil {
		if errutil.IsNotFound(err) {
			return fmt.Errorf("tag %s: %w", tagID, ErrUnknownTag)
		}

		return err
	}

	i.notifyTagTreeChange()

	return nil
}

// DeleteTag removes the tag the id names together with every assignment
// of it.
func (i *input) DeleteTag(tagID xid.ID) error {
	if err := i.db.DeleteTag(i.ctx, tagID, i.orgID); err != nil {
		if errutil.IsNotFound(err) {
			return fmt.Errorf("tag %s: %w", tagID, ErrUnknownTag)
		}

		return err
	}

	i.notifyTagTreeChange()

	return nil
}

// AssignTag makes the branch branchID names carry the tag. The branch is
// resolved first so an unknown document or branch is reported as such;
// what the repository then fails to find can only be the tag.
func (i *input) AssignTag(documentID, branchID, tagID xid.ID) error {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return err
	}

	if err := i.db.AssignBranchTag(i.ctx, i.orgID, doc.ID, doc.BranchID, tagID); err != nil {
		if errutil.IsNotFound(err) {
			return fmt.Errorf("tag %s: %w", tagID, ErrUnknownTag)
		}

		return err
	}

	i.notifyTagTreeChange()
	i.notifyBranchTagsChange(doc.ID, doc.BranchID)

	return nil
}

// UnassignTag stops the branch branchID names carrying the tag. The
// repository treats a tag the branch does not carry as nothing to do, so
// the tag is looked up here for the call to report an id that names
// nothing.
func (i *input) UnassignTag(documentID, branchID, tagID xid.ID) error {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return err
	}

	if _, err := i.FetchTag(tagID); err != nil {
		return err
	}

	if err := i.db.UnassignBranchTag(i.ctx, i.orgID, doc.ID, doc.BranchID, tagID); err != nil {
		return err
	}

	i.notifyTagTreeChange()
	i.notifyBranchTagsChange(doc.ID, doc.BranchID)

	return nil
}

// MoveTag moves the tag to the given position among the organisation's
// tags, which are then rewritten in the resulting order. The position is
// a 0-based index into the display order, and one outside it is refused.
func (i *input) MoveTag(tagID xid.ID, sortIndex int) error {
	tree, err := i.FetchTagTree()
	if err != nil {
		return fmt.Errorf("fetching tags: %w", err)
	}

	moved, err := tree.Swap(tagID, sortIndex)
	if err != nil {
		if errutil.IsNotFound(err) {
			return fmt.Errorf("tag %s: %w", tagID, ErrUnknownTag)
		}

		return err
	}

	if err := i.db.UpdateTagTree(i.ctx, moved, i.orgID); err != nil {
		return err
	}

	i.notifyTagTreeChange()

	return nil
}

// notifyTagTreeChange invokes the tag notifier when one is configured.
// A nil notifier silently no-ops, like the document tree's.
func (i *input) notifyTagTreeChange() {
	if i.tags == nil {
		return
	}

	i.tags.NotifyTreeChange(i.orgID)
}

// notifyBranchTagsChange invokes the tag notifier for one branch's tags
// when one is configured. A nil notifier silently no-ops.
func (i *input) notifyBranchTagsChange(documentID, branchID xid.ID) {
	if i.tags == nil {
		return
	}

	i.tags.NotifyBranchTagsChange(i.orgID, documentID, branchID)
}

// FetchHooks returns every hook on the branch branchID names, refusing a
// branch the document does not have.
func (i *input) FetchHooks(documentID, branchID xid.ID) ([]hook.Hook, error) {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return nil, err
	}

	hooks, err := i.db.FetchDocumentHooksByBranchID(i.ctx, doc.BranchID, i.orgID)
	if err != nil {
		return nil, fmt.Errorf("fetching hooks: %w", err)
	}

	return hooks, nil
}

// FetchHook returns the hook the id names on the document. The lookup is
// scoped to the organisation, and a hook of another document, or of a
// document that was deleted and left it behind, is refused as unknown to
// the one addressed.
func (i *input) FetchHook(documentID, hookID xid.ID) (*hook.Hook, error) {
	hk, err := i.db.FetchDocumentHook(i.ctx, hookID, i.orgID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return nil, fmt.Errorf("hook %s on document %s: %w", hookID, documentID, errUnknownHook)
		}

		return nil, fmt.Errorf("fetching hook: %w", err)
	}

	if !hk.DocumentID.Valid || hk.DocumentID.V != documentID {
		return nil, fmt.Errorf("hook %s on document %s: %w", hookID, documentID, errUnknownHook)
	}

	return hk, nil
}

// CreateHook creates a hook on the branch branchID names, anchored to the
// block blockUID names when one is given. The branch is checked here, so
// a hook cannot land on a branch of another document. A type whose
// integration the deployment lacks is refused before anything is created.
func (i *input) CreateHook(documentID, branchID xid.ID, blockUID string, tp hook.Type, settings processor.Settings) (*hook.Hook, error) {
	doc, err := i.FetchBranch(documentID, branchID)
	if err != nil {
		return nil, err
	}

	// the manager would refuse this too, but the integration's own error
	// tells the model what is missing.
	switch tp {
	case hook.TypeGithubTracking:
		if !i.githubMan.Configured() {
			return nil, fmt.Errorf("%s: %w", tp, github.ErrNotConfigured)
		}
	case hook.TypeURLWatcher:
		if !i.webchangeClient.Configured() {
			return nil, fmt.Errorf("%s: %w", tp, webchange.ErrNotConfigured)
		}
	case hook.TypeScheduledReminder, hook.TypeContainerImageWatcher:
		// nothing outside the deployment to check.
	}

	hk, err := i.hookMan.CreateHook(i.ctx, hook.CreateInput{
		Type:     tp,
		BranchID: doc.BranchID,
		BlockID:  null.NewString(blockUID, blockUID != ""),
		Settings: settings,
	}, doc.ID, i.orgID, i.userID)
	if err != nil {
		if errors.Is(err, hook.ErrBlockNotFound) {
			return nil, fmt.Errorf("block %s: %w", blockUID, errUnknownBlock)
		}

		return nil, err
	}

	return hk, nil
}

// UpdateHook replaces the hook's settings and resets its score and state,
// as a fresh hook with those settings would have them.
func (i *input) UpdateHook(hk *hook.Hook, settings processor.Settings) error {
	updated, err := i.hookMan.UpdateHook(i.ctx, hk.ID, hk.DocumentID.V, i.orgID, hook.UpdateInput{Settings: settings}, i.userID)
	if err != nil {
		return err
	}

	*hk = *updated

	return nil
}

// ResetHook restores the hook's score and state, keeping its settings.
func (i *input) ResetHook(hk *hook.Hook) error {
	reset, err := i.hookMan.ResetHook(i.ctx, hk.ID, hk.DocumentID.V, i.orgID)
	if err != nil {
		return err
	}

	*hk = *reset

	return nil
}

// DeleteHook tears down what the hook holds outside the document and
// removes its row.
func (i *input) DeleteHook(hk *hook.Hook) error {
	if err := i.hookMan.DeleteHook(i.ctx, hk.ID, hk.DocumentID.V, i.orgID, i.userID); err != nil {
		return err
	}

	return nil
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

// DB is the persistence surface the tools require, composed from the
// surfaces below. The db package's agent satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t both DB db
type DB interface {
	sqlutil.DB
	DocumentDB
	DataSourceDB
	TagDB
	HookDB
}

// DocumentDB is the document tree, its branches and their content as
// the tools read and change them.
type DocumentDB interface {
	// FetchDocumentTree should return all documents for the org as a
	// nested summary tree (sort_index order). Used by
	// list_documents.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)

	// FetchDocumentTreeByDocumentParentID should return the children of
	// parentID within the org (nil parentID = top-level). Used by
	// list_documents.
	FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (document.Summaries, error)

	// FetchDocument should return the document on the named branch.
	// Used by the document-level tools, which act on the default
	// branch.
	FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*document.Document, error)

	// FetchDocumentBranches should list the document's branches. Used
	// by get_document, and to tell an unknown branch from an unknown
	// document.
	FetchDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) ([]document.BranchSummary, error)

	// FetchDocumentByBranchID should return the document the branch id
	// names, on that branch. Used for every branch-targeted call.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// UpdateDocumentParentID should re-parent a document. Used by
	// update_document.
	UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error

	// CheckDocumentExists should report whether the document exists in
	// the given org. Used by update_document to validate the new
	// parent before issuing UPDATE.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error

	// CheckDocumentCycle should report whether making parentID the parent
	// of id would create a cycle in the document tree. Used by
	// update_document to reject self and descendant parents.
	CheckDocumentCycle(ctx context.Context, id, parentID xid.ID, organizationID string) (bool, error)
}

// DataSourceDB is the organisation's data sources as the tools look
// them up.
type DataSourceDB interface {
	// FetchDataSource should return a data source by id within the org.
	// Used by every data-source tool to resolve what it was asked about.
	FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasource.DataSource, error)

	// FetchDataSources should return every data source the org owns.
	// Used by list_data_sources.
	FetchDataSources(ctx context.Context, organizationID string) ([]datasource.DataSource, error)
}

// TagDB is the organisation's tags and their branch assignments as the
// tools read and change them.
type TagDB interface {
	// FetchTagTree should return the org's tags in display order, each
	// with the documents whose default branch carries it and whether the
	// given user hides it. Used by list_tags and every tag lookup.
	FetchTagTree(ctx context.Context, organizationID, userID string) (tag.Summaries, error)

	// FetchBranchTags should return the tags a document's branch carries
	// in the tags' display order. Used by get_document.
	FetchBranchTags(ctx context.Context, organizationID string, documentID, branchID xid.ID) ([]tag.Tag, error)

	// InsertTag should store a new tag at the end of its org's tags,
	// refusing a name the org already uses. Used by create_tag.
	InsertTag(ctx context.Context, t tag.Tag) error

	// UpdateTag should rename and/or recolour a tag within the org. Used
	// by update_tag.
	UpdateTag(ctx context.Context, organizationID string, id xid.ID, inp tag.UpdateInput) error

	// DeleteTag should remove a tag and every assignment of it. Used by
	// delete_tag.
	DeleteTag(ctx context.Context, id xid.ID, organizationID string) error

	// AssignBranchTag should make a document's branch carry a tag,
	// changing nothing when it already does.
	AssignBranchTag(ctx context.Context, organizationID string, documentID, branchID, tagID xid.ID) error

	// UnassignBranchTag should stop a document's branch carrying a tag,
	// changing nothing when it does not.
	UnassignBranchTag(ctx context.Context, organizationID string, documentID, branchID, tagID xid.ID) error

	// UpdateTagTree should rewrite the display order of the org's tags to
	// the order of the given tree.
	UpdateTagTree(ctx context.Context, tree tag.Summaries, organizationID string) error
}

// HookDB is the freshness hooks on a document's branches as the tools
// read and change them.
type HookDB interface {
	// FetchDocumentHooksByBranchID should return every hook on a branch
	// within the org. Used by list_hooks.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error)

	// FetchDocumentHook should return a hook by id within the org. Used
	// by every hook write to resolve what it was asked about.
	FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hook.Hook, error)
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

	// InsertSearchJob should queue the search job's scope. Used by
	// create_document and delete_document.
	InsertSearchJob(ctx context.Context, job search.Job) error

	// DeleteDocument should remove a document and report the ids of the
	// document and of every cascade-deleted descendant. Used by
	// delete_document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) ([]xid.ID, error)
}

// HookManager runs hook writes.
//
//go:generate ../../../scripts/codegen/mock -t internal HookManager hook_manager
type HookManager interface {
	// CreateHook should create the hook on the branch of the document,
	// credited to updatedBy.
	CreateHook(ctx context.Context, ci hook.CreateInput, documentID xid.ID, organizationID, updatedBy string) (*hook.Hook, error)

	// UpdateHook should replace the settings of the document's hook,
	// credited to updatedBy, and return the stored hook.
	UpdateHook(ctx context.Context, id, documentID xid.ID, organizationID string, ui hook.UpdateInput, updatedBy string) (*hook.Hook, error)

	// DeleteHook should tear the document's hook down and remove it,
	// credited to updatedBy.
	DeleteHook(ctx context.Context, id, documentID xid.ID, organizationID, updatedBy string) error

	// ResetHook should reset the document's hook and return the stored
	// hook.
	ResetHook(ctx context.Context, id, documentID xid.ID, organizationID string) (*hook.Hook, error)
}

// SearchTrigger runs the search-job worker once a job has committed.
//
//go:generate ../../../scripts/codegen/mock -t internal SearchTrigger search_trigger
type SearchTrigger interface {
	// Trigger should run a search-job pass right away.
	Trigger()
}

// Searcher is the full-text search surface search_documents uses.
// The search index satisfies it.
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

// TagNotifier publishes tag-tree-change events so connected clients can
// refresh their sidebar's tag tree after assistant-driven tag writes.
// The server tag handler satisfies this interface via its
// NotifyTreeChange method.
//
//go:generate ../../../scripts/codegen/mock -t internal TagNotifier tag_notifier
type TagNotifier interface {
	// NotifyTreeChange should tell subscribers that the tag tree of
	// organizationID changed. Implementations must be safe to call
	// concurrently.
	NotifyTreeChange(organizationID string)

	// NotifyBranchTagsChange should tell the subscribers of the document
	// that the tags its branch branchID carries changed. Implementations
	// must be safe to call concurrently.
	NotifyBranchTagsChange(organizationID string, documentID, branchID xid.ID)
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
	Apply(ctx context.Context, documentID, branchID xid.ID, ops []edit.Operation, userID string, system bool) (edit.Result, error)
}
