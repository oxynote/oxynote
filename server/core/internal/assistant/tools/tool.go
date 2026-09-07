package tools

import (
	"context"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/rs/xid"
)

// Info is a tool's model-facing description: the name the model calls,
// the prose it reads to decide when to call it, and the JSON-schema
// fragment describing the arguments.
//
// It is a value rather than a method call on a live tool because a
// description never depends on a request — the same tool describes
// itself identically in every session.
type Info struct {
	// Name is the tool's canonical identifier.
	Name Name

	// Description is what the model reads to decide whether this is the
	// tool it wants. It is pinned by testdata/tool_schemas.golden.
	Description string

	// Properties is the JSON-schema fragment enumerating the arguments,
	// keyed by argument name.
	Properties map[string]any

	// Required lists the arguments the model must supply. Empty means
	// the schema says nothing about required arguments.
	Required []string
}

// Schema returns the tool's arguments as a JSON-schema object.
// Properties are already a schema fragment, so this only wraps them —
// which keeps each tool's own schema the single source of truth for
// every surface that has to describe it, rather than restating it in a
// second vocabulary per surface.
func (info Info) Schema() map[string]any {
	out := map[string]any{
		"type":       "object",
		"properties": info.Properties,
	}

	// a tool with no required arguments says nothing about them: an
	// empty array is noise, and a null is not a valid "required".
	if len(info.Required) > 0 {
		out["required"] = info.Required
	}

	return out
}

// Traits are the facts about a tool that decide how the assistant
// treats it. A tool states them in one place in its own file rather
// than being interrogated for them from several.
type Traits struct {
	// Write indicates the tool mutates a document. It gates the tool
	// behind user confirmation and protects its result from being
	// cleared by the context middlewares — the model has to keep
	// knowing what it changed, while a stale read can always be taken
	// again. It is also the only thing asked: a tool declaring it is
	// what Summary is called on.
	Write bool

	// Destructive indicates the tool removes content. These are
	// confirmed every time, even inside a turn the user auto-approved:
	// their tool descriptions promise as much, and an approve-all meant
	// for text edits is not consent to delete a document.
	Destructive bool

	// Overwrites indicates the write replaces content the caller did
	// not name: the target's nested blocks go with it, and so do the
	// uids comments and hooks hang off. It is what MCP's destructive
	// hint reports, and it is deliberately not Destructive — a client
	// deciding whether to auto-approve needs to know, while an
	// approve-all inside a chat turn is meant to cover exactly these
	// edits.
	Overwrites bool

	// DataSource indicates the tool reads one of the organisation's
	// outbound data-source connections rather than its documents. A
	// surface that gates access by scope asks it: over MCP these tools
	// need their own grant, since querying an organisation's databases
	// is not the same permission as reading its documents.
	DataSource bool

	// Internal keeps a tool off surfaces that serve the registry to
	// clients outside this process. It marks a tool that only makes
	// sense inside an assistant conversation, so an MCP client — which
	// holds none of the conversation state such a tool addresses —
	// never sees it offered.
	Internal bool
}

// Tool is a tool the model can call. What a tool is — whether it
// mutates, whether it survives an approve-all, whether it belongs
// outside this process — it states in Traits rather than in the set of
// interfaces it satisfies, so there is one answer to ask and nothing to
// hold in step.
type Tool interface {
	// Info should return the tool's model-facing description.
	Info() Info

	// Traits should return the facts that decide how the assistant
	// treats this tool.
	Traits() Traits

	// Title should return a short line describing what the tool is
	// about to do, or an empty string for tools too noisy or too
	// generic to announce, and reject arguments it cannot read.
	Title(inp DescribeInput) (string, error)

	// Summary should describe the change the tool is proposing, for
	// the card asking the user to approve it, without performing it. It
	// should reject arguments it cannot read: a payload Execute would
	// refuse is not worth asking the user to approve.
	//
	// Only a tool whose Traits report a write is ever asked; the rest
	// embed plainSummary.
	Summary(inp DescribeInput) (ActionSummary, error)

	// Execute should perform the tool's work and return the result
	// serialised for the model.
	Execute(inp Input) (string, error)
}

// plainSummary implements the Tool interface to avoid Summary
// re-implementation in tools that propose nothing.
type plainSummary struct{}

// Summary describes nothing: a tool that does not write is never gated,
// so it is never asked what it is about to change.
func (plainSummary) Summary(_ DescribeInput) (ActionSummary, error) {
	return ActionSummary{}, nil
}

// plainTraits implements the Tool interface to avoid Traits
// re-implementation in tools that are plain reads.
type plainTraits struct{}

// Traits reports a plain read: no write, no outbound connection, nothing
// internal.
func (plainTraits) Traits() Traits {
	return Traits{}
}

// plainTitle implements the Tool interface to avoid Title
// re-implementation in tools too generic to announce.
type plainTitle struct{}

// Title returns no status line.
func (plainTitle) Title(_ DescribeInput) (string, error) {
	return "", nil
}

// Args is one tool call's decoded arguments. Validate is what Decode
// runs once the payload is read, so a tool states what it requires next
// to the fields that carry it, and nothing downstream of Decode sees a
// payload the tool cannot act on.
type Args interface {
	// Validate should reject arguments the tool cannot act on: a
	// required one that is missing, or a value outside its range.
	Validate() error
}

// DescribeInput is what a tool is handed when it is asked to describe
// itself — for the status line while it runs, or the card asking the
// user to approve it. It carries the arguments the tool was about to
// receive plus the read-only lookups needed to name their subject.
//
// It deliberately reaches nothing that mutates: describing a pending
// write must never perform it, and here that is enforced by the type
// rather than by remembering.
type DescribeInput interface {
	// Context should return the context of the call being described.
	Context() context.Context

	// Decode should decode the call's arguments into dst and validate
	// them, reporting a payload that is malformed or incomplete as an
	// error naming the tool. It is the only way into a call's
	// arguments: a description that cannot read them says so rather
	// than describing a call from zero values.
	Decode(dst Args) error

	// FetchDocument should return the document the id names, on its default
	// branch, for a description that has to name its subject.
	FetchDocument(documentID xid.ID) (*document.Document, error)

	// FetchBranch should return the document on the branch branchID names,
	// for a description that has to name the branch it targets.
	FetchBranch(documentID, branchID xid.ID) (*document.Document, error)

	// FetchDataSource should return the data source the id names, for a
	// description that has to name its subject.
	FetchDataSource(dataSourceID xid.ID) (*datasource.DataSource, error)

	// DescendantCount should report how many documents sit under the
	// named one, at any depth, for a description that has to say how
	// far a cascade reaches.
	DescendantCount(id xid.ID) (int, error)

	// FetchTag should return the tag the id names, with the documents
	// carrying it, for a description that has to name its subject.
	FetchTag(tagID xid.ID) (*tag.Summary, error)

	// FetchHook should return the hook the id names on the document, for
	// a description that has to name its subject.
	FetchHook(documentID, hookID xid.ID) (*hook.Hook, error)
}

// DataSources is the organisation's outbound data-source connections as
// a tool reads them. Every method is already scoped to the session's
// organisation, so a tool can never reach another one's connections.
type DataSources interface {
	// FetchDataSources should return every data source the organisation
	// owns.
	FetchDataSources() ([]datasource.DataSource, error)

	// DataSourceRunner should return the runner that operates the data
	// source the id names, or an error when the organisation owns no
	// data source with that id.
	DataSourceRunner(id xid.ID) (datasource.Runner, error)

	// CheckDataSources should refuse a write naming a data source the
	// organisation does not own.
	CheckDataSources(ids []string) error
}

// Documents is the organisation's document tree as a tool reads it.
// Every method is already scoped to the session's organisation, so a
// tool can never reach another one's documents.
type Documents interface {
	// FetchDocument should return the document on its default branch.
	FetchDocument(documentID xid.ID) (*document.Document, error)

	// FetchBranch should return the document on the branch branchID names,
	// refusing a branch the document does not have.
	FetchBranch(documentID, branchID xid.ID) (*document.Document, error)

	// FetchDocumentContent should return the parsed content of the branch
	// branchID names, for the ops that only need the block tree.
	FetchDocumentContent(documentID, branchID xid.ID) (document.Content, error)

	// FetchDocumentBlock should return the block blockUID names on the
	// branch, refusing a uid the branch does not hold.
	FetchDocumentBlock(documentID, branchID xid.ID, blockUID string) (document.Block, error)

	// FetchDocumentBranches should list every branch of the document,
	// refusing a document the organisation does not have.
	FetchDocumentBranches(documentID xid.ID) ([]document.BranchSummary, error)

	// FetchDocumentTree should return every document in the organisation as
	// a nested summary tree.
	FetchDocumentTree() (document.Summaries, error)

	// FetchDocumentChildren should return the direct children of parentID; a
	// null parent means the organisation's root.
	FetchDocumentChildren(parentID null.Value[xid.ID]) (document.Summaries, error)
}

// DocumentWriter changes the shape of the document tree and announces
// what it changed, so connected sidebars stay in step.
type DocumentWriter interface {
	// CreateDocument should insert the document together with its first
	// maintainer and its search job, atomically, refusing a parent that
	// does not exist.
	CreateDocument(doc document.Document) error

	// DeleteDocument should remove the document.
	DeleteDocument(id xid.ID) error

	// MoveDocument should re-parent the document doc describes, refusing
	// a parent that does not exist or that would put the document under
	// itself.
	MoveDocument(doc *document.Document, parentID null.Value[xid.ID]) error

	// NotifyTreeChange should tell subscribers that the tree under
	// parentID changed.
	NotifyTreeChange(parentID null.Value[xid.ID])

	// NotifyTreeChangeForDocument should look up the document's current
	// parent and fire a tree-change for it, for the ops whose arguments
	// carry no parent.
	NotifyTreeChangeForDocument(documentID xid.ID)
}

// Tags is the organisation's tags as a tool reads them. Every method is
// already scoped to the session's organisation.
type Tags interface {
	// FetchTagTree should return every tag in the organisation in display
	// order, each with the documents whose default branch carries it and
	// whether the session's user hides it.
	FetchTagTree() (tag.Summaries, error)

	// FetchBranchTags should return the tags the branch branchID names
	// carries, in the tags' display order.
	FetchBranchTags(documentID, branchID xid.ID) ([]tag.Tag, error)
}

// TagWriter changes the organisation's tags and what carries them, and
// announces the change so connected sidebars stay in step.
type TagWriter interface {
	// CreateTag should insert the tag at the end of the organisation's
	// tags, refusing a name the organisation already uses.
	CreateTag(t tag.Tag) error

	// UpdateTag should rename and/or recolour the tag the id names,
	// refusing an id that names nothing.
	UpdateTag(tagID xid.ID, inp tag.UpdateInput) error

	// DeleteTag should remove the tag the id names and every assignment
	// of it, refusing an id that names nothing.
	DeleteTag(tagID xid.ID) error

	// AssignTag should make the branch branchID names carry the tag,
	// refusing a document, branch or tag that does not exist.
	AssignTag(documentID, branchID, tagID xid.ID) error

	// UnassignTag should stop the branch branchID names carrying the
	// tag, refusing a document, branch or tag that does not exist.
	UnassignTag(documentID, branchID, tagID xid.ID) error

	// MoveTag should move the tag to the 0-based position among the
	// organisation's tags, refusing a position outside them.
	MoveTag(tagID xid.ID, sortIndex int) error

	// NotifyTagTreeChange should tell subscribers that the tag tree
	// changed.
	NotifyTagTreeChange()

	// NotifyBranchTagsChange should tell the document's subscribers that
	// the tags the branch branchID names carries changed, so an open
	// header refreshes its pills.
	NotifyBranchTagsChange(documentID, branchID xid.ID)
}

// Hooks is the freshness hooks on a document's branches as a tool reads
// them. Every method is already scoped to the session's organisation.
type Hooks interface {
	// FetchHooks should return every hook on the branch branchID names,
	// refusing a branch the document does not have.
	FetchHooks(documentID, branchID xid.ID) ([]hook.Hook, error)
}

// HookWriter changes the hooks on a document's branches and announces
// the change so an open editor stays in step.
type HookWriter interface {
	// CreateHook should create a hook of the type with the settings on
	// the branch branchID names, anchored to the block blockUID names
	// when one is given, refusing a branch or block that does not exist
	// and a type whose integration the deployment lacks.
	CreateHook(documentID, branchID xid.ID, blockUID string, t hook.Type, settings processor.Settings) (*hook.Hook, error)

	// UpdateHook should replace the hook's settings and reset its score
	// and state.
	UpdateHook(hk *hook.Hook, settings processor.Settings) error

	// ResetHook should restore the hook's score and state, keeping its
	// settings.
	ResetHook(hk *hook.Hook) error

	// DeleteHook should tear down what the hook holds outside the
	// document and remove it.
	DeleteHook(hk *hook.Hook) error
}

// Editor applies content changes to a live document through the
// realtime service, so connected editors see them as they land.
type Editor interface {
	// ApplyEdit should ship an operation batch to the realtime service
	// for the branch branchID names of the document, reporting any
	// operation it refused.
	ApplyEdit(documentID, branchID xid.ID, ops []edit.Operation) error

	// ValidatePlacement should check that a block is legal next to, or
	// in place of, the block referenceUID names on the branch.
	ValidatePlacement(documentID, branchID xid.ID, referenceUID string, b block.Block) error

	// ValidateAttrUpdate should check that setting attrs on the block
	// blockUID names on the branch leaves it with attributes its type
	// allows.
	ValidateAttrUpdate(documentID, branchID xid.ID, blockUID string, attrs map[string]any) error

	// ValidateMove should check that the block blockUID names may sit
	// where a move relative to referenceUID would put it on the branch.
	ValidateMove(documentID, branchID xid.ID, blockUID, referenceUID string) error
}

// Input is everything a tool needs to do its work: the call's arguments
// and context, plus the organisation's documents and the services that
// change them.
//
// It is a facade over the narrower surfaces above rather than a list of
// its own, and it is built per call, so a tool holds no state between
// them.
//
//nolint:interfacebloat // facade composed of the narrower surfaces above
type Input interface {
	DescribeInput
	Documents
	DocumentWriter
	DataSources
	Tags
	TagWriter
	Hooks
	HookWriter
	Editor

	// OrganizationID should return the organisation every call is
	// scoped to, for the tools that stamp it onto a new row.
	OrganizationID() string

	// UserID should return the user the assistant is acting for, for
	// the tools that record who acted.
	UserID() string

	// SearchBlocks should return blocks whose text matches the query,
	// capped at limit hits.
	SearchBlocks(query string, limit int) ([]search.Block, error)

	// ReadOffloaded should return a tool result that was moved out of
	// the conversation for size.
	ReadOffloaded(path string) (string, error)

	// Warn should record something a tool decided to carry on through,
	// so a degraded result is diagnosable without re-running the
	// conversation.
	Warn(msg string, attrs ...slog.Attr)
}

// OffloadReader is the retrieval half of the store holding tool results
// that were too large to keep in the conversation. The persist
// package's Offload satisfies it.
type OffloadReader interface {
	// Read should return the payload stored at the path, or an error
	// explaining that it is gone.
	Read(ctx context.Context, path string) (string, error)
}
