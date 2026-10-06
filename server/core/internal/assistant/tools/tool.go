package tools

import (
	"context"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
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

	// Traits are the facts that decide how the assistant treats the
	// tool. The zero value is a plain read.
	Traits Traits
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
// outside this process — it states in its Info's Traits, so there is one
// answer to ask and nothing to hold in step.
type Tool interface {
	// Info should return the tool's model-facing description and traits.
	Info() Info

	// Execute should perform the tool's work and return the result
	// serialised for the model. inp is the call's arguments together
	// with every read and write the tools share, scoped to the session's
	// organisation.
	Execute(inp *input) (string, error)
}

// titler is a read that announces itself while it runs. A read without
// a title runs silently; a write is announced by its summary.
type titler interface {
	// Title should return a short line describing what the tool is
	// about to do, or an empty string when its arguments name nothing it
	// can describe.
	Title(inp DescribeInput) string
}

// summarizer is a write that describes the change it proposes, for the
// card asking the user to approve it. Every write is one.
type summarizer interface {
	// Summary should describe the change without performing it, and
	// reject arguments it cannot read: a payload Execute would refuse is
	// not worth asking the user to approve.
	Summary(inp DescribeInput) (ActionSummary, error)
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
	// error. It is the only way into a call's
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

// OffloadReader is the retrieval half of the store holding tool results
// that were too large to keep in the conversation. The persist
// package's Offload satisfies it.
type OffloadReader interface {
	// Read should return the payload stored at the path, or an error
	// explaining that it is gone.
	Read(ctx context.Context, path string) (string, error)
}
