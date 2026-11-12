package tools

import (
	"context"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/rs/xid"
)

// Info is a tool's model-facing description: its name, the prose the
// model reads, and the schema of its arguments. It never depends on a
// call.
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
// Properties are already a schema fragment, so this only wraps them,
// and every surface describes a tool from its own schema.
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
	// behind user confirmation, and the context middlewares never clear
	// its result, since the model has to keep knowing what it changed.
	// Only a write has a Summary.
	Write bool

	// Destructive indicates the tool removes content. It is confirmed
	// every time, even inside a turn the user auto-approved.
	Destructive bool

	// Overwrites indicates the write replaces content the caller did not
	// name, such as a replaced block's children. MCP reports it as the
	// destructive hint. An approve-all still covers it, so it is not
	// Destructive.
	Overwrites bool

	// Idempotent indicates a repeated call with the same arguments
	// changes nothing more. MCP reports it as the idempotent hint.
	Idempotent bool

	// DataSource indicates the tool reads one of the organisation's
	// outbound data-source connections. Over MCP these tools need their
	// own scope.
	DataSource bool

	// Internal keeps a tool off surfaces outside this process. Such a
	// tool only makes sense inside an assistant conversation, which an
	// MCP client does not have.
	Internal bool

	// Group names the tools the chat assistant loads together on
	// request. Empty is a tool every conversation starts with. Other
	// surfaces offer every tool either way.
	Group Group

	// Role specifies the least role that may call the tool. Every tool
	// names one, and a tool without one is offered to nobody. A write a
	// viewer may make changes only the caller's own settings.
	Role org.Role
}

// Tool is a tool the model can call. Its fields are the call's
// arguments, decoded and validated before any of its methods runs. What
// it is, such as whether it writes, it states in its Info's Traits.
type Tool interface {
	// Info should return the tool's model-facing description and traits.
	// It reads no field, so the zero value answers it.
	Info() Info

	// Validate should reject arguments the tool cannot act on: a
	// required one that is missing, or a value outside its range.
	Validate() error

	// Exec should perform the call and return its result. inp reaches
	// every read and write, scoped to the session's organisation.
	Exec(inp Input) (any, error)
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
	// reject arguments it cannot act on: a payload Exec would refuse is
	// not worth asking the user to approve.
	Summary(inp DescribeInput) (ActionSummary, error)
}

// DescribeInput is the read-only half of Input: the lookups a tool needs
// to name what a call is about, for its status line or the card asking
// the user to approve it. It reaches nothing that writes, so describing
// a write cannot perform it.
type DescribeInput interface {
	// Context should return the context of the call being described.
	Context() context.Context

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

	// FetchVariableGroup should return the variable group the id names,
	// for a description that has to name its subject.
	FetchVariableGroup(groupID xid.ID) (*variable.GroupTree, error)

	// FetchVariable should return the variable the id names, for a
	// description that has to name its subject.
	FetchVariable(variableID xid.ID) (*variable.Entry, error)

	// FetchHook should return the hook the id names on the document, for
	// a description that has to name its subject.
	FetchHook(documentID, hookID xid.ID) (*hook.Hook, error)
}

// Input is everything a tool call reaches: the call's context, the
// session it runs in, and the reads and writes of every subject.
//
//go:generate ../../../scripts/codegen/mock -t internal Input input
type Input interface {
	DescribeInput
	SessionInput
	DocumentInput
	SearchInput
	TagInput
	VariableInput
	HookInput
	DataSourceInput
}

// SessionInput is what a tool reaches of the session it runs in.
type SessionInput interface {
	// OrganizationID should return the organisation the session acts in.
	OrganizationID() string

	// UserID should return the user the session acts for.
	UserID() string

	// Warn should log a problem the tool worked around, naming the tool.
	Warn(msg string, attrs ...slog.Attr)

	// ReadToolOutput should return the tool result stored at the path.
	ReadToolOutput(path string) (string, error)
}

// OffloadReader is the retrieval half of the store holding tool results
// that were too large to keep in the conversation. The persist
// package's Offload satisfies it.
type OffloadReader interface {
	// Read should return the payload stored at the path, or an error
	// explaining that it is gone.
	Read(ctx context.Context, path string) (string, error)
}
