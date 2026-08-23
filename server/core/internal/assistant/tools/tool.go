package tools

import (
	"context"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
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
		_keyType:     _typeObject,
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

	// Document should return the document the id names, for a
	// description that has to name its subject.
	Document(id xid.ID) (*document.Document, error)

	// DataSource should return the data source the id names, for a
	// description that has to name its subject.
	DataSource(dataSourceID xid.ID) (*datasource.DataSource, error)
}

// DataSources is the organisation's outbound data-source connections as
// a tool reads them. Every method is already scoped to the session's
// organisation, so a tool can never reach another one's connections.
type DataSources interface {
	// DataSources should return every data source the organisation
	// owns.
	DataSources() ([]datasource.DataSource, error)

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
	// Document should return the document's default branch.
	Document(id xid.ID) (*document.Document, error)

	// DocumentContent should return the document's parsed main-branch
	// content, for the ops that only need the block tree.
	DocumentContent(id xid.ID) (document.Content, error)

	// DocumentTree should return every document in the organisation as
	// a nested summary tree.
	DocumentTree() (document.Summaries, error)

	// DocumentChildren should return the direct children of parentID; a
	// null parent means the organisation's root.
	DocumentChildren(parentID null.Value[xid.ID]) (document.Summaries, error)
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

	// MoveDocument should re-parent the document, refusing a parent that
	// does not exist or that would put the document under itself.
	MoveDocument(id xid.ID, parentID null.Value[xid.ID]) error

	// NotifyTreeChange should tell subscribers that the tree under
	// parentID changed.
	NotifyTreeChange(parentID null.Value[xid.ID])

	// NotifyTreeChangeForDocument should look up the document's current
	// parent and fire a tree-change for it, for the ops whose arguments
	// carry no parent.
	NotifyTreeChangeForDocument(documentID xid.ID)
}

// Editor applies content changes to a live document through the
// realtime service, so connected editors see them as they land.
type Editor interface {
	// ApplyEdit should ship an operation batch to the realtime service
	// for the named document's default branch and surface the per-op
	// result.
	ApplyEdit(documentID xid.ID, ops []edit.Operation) (string, error)

	// ValidatePlacement should check that a block is legal next to, or
	// in place of, the block referenceUID names.
	ValidatePlacement(documentID xid.ID, referenceUID string, b block.Block) error
}

// Input is everything a tool needs to do its work: the call's arguments
// and context, plus the organisation's documents and the services that
// change them.
//
// It is a facade over the narrower surfaces above rather than a list of
// its own, and it is built per call, so a tool holds no state between
// them.
type Input interface {
	DescribeInput
	Documents
	DocumentWriter
	DataSources
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
