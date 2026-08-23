// Package tools owns the AI assistant's tool surface. Tools are grouped
// by what they act on — documents, blocks, the search index — and each
// describes itself, names what it is about to do, and does its work in
// this package's own vocabulary; eino.go is the only place that speaks
// the agent framework's. All tools execute server-side — there is no
// client-side forwarding.
package tools

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/cloudwego/eino/components/tool"
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

// Data-source tools — reads against the organisation's outbound
// connections. They reach systems outside Oxynote, so a surface that
// gates by scope asks Traits.DataSource rather than lumping them in
// with the document reads.
const (
	NameListDataSources           Name = "list_data_sources"
	NameGetPrometheusMetadata     Name = "get_prometheus_metadata"
	NameListPrometheusLabelNames  Name = "list_prometheus_label_names"
	NameListPrometheusLabelValues Name = "list_prometheus_label_values"
	NameListPrometheusSeries      Name = "list_prometheus_series"
	NameQueryPrometheus           Name = "query_prometheus"
	NameGetSQLMetadata            Name = "get_sql_metadata"
	NameGetSQLQueryLabels         Name = "get_sql_query_labels"
	NameQuerySQL                  Name = "query_sql"
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
// built once from the session's wiring and handed straight to the agent.
//
// It is the only place that names every tool. What a tool is — whether
// it needs confirming, whether it survives an approve-all, whether it
// belongs on surfaces outside this process — the tool states in its own
// Traits, so there is no second list to keep in step with this one.
type Set struct {
	// tools maps each tool's name to the value that serves it, already
	// wrapped in its confirmation gate where one applies.
	tools map[Name]registryTool

	// ordered lists the tools in registration order. Tools returns it so
	// every session presents the model the same tool order, which keeps
	// provider prompt caching effective.
	ordered []registryTool

	// entries lists the tools in registration order without their
	// confirmation gates, for surfaces that own their own approval
	// story. Entries returns it.
	entries []Entry

	// writes lists the tools that mutate a document.
	writes []string
}

// New creates a fresh instance of Set.
func New(deps *Deps) *Set {
	registerConfirmTypes()

	all := []Tool{
		listDocuments{},
		getDocument{},
		readDocumentSummary{},
		readBlock{},
		searchDocuments{},

		listDataSources{},
		getPrometheusMetadata{},
		listPrometheusLabelNames{},
		listPrometheusLabelValues{},
		listPrometheusSeries{},
		queryPrometheus{},
		getSQLMetadata{},
		getSQLQueryLabels{},
		querySQL{},

		createDocument{},
		deleteDocument{},
		renameDocument{},
		setDocumentIcon{},
		moveDocument{},
		insertBlock{},
		appendBlock{},
		prependBlock{},
		replaceBlock{},
		updateBlockText{},
		updateBlockAttrs{},
		deleteBlock{},

		readToolOutput{},
	}

	s := &Set{tools: make(map[Name]registryTool, len(all))}

	for _, tl := range all {
		et := newEinoTool(tl, deps)
		tr := tl.Traits()

		s.entries = append(s.entries, Entry{
			Traits: tr,
			Name:   et.info.Name,
			Info:   et.info,
			Tool:   et,
		})

		var it registryTool = et

		// the gate goes on here rather than being something every write
		// has to remember to ask for, so a write cannot slip through by
		// forgetting.
		if tr.Write {
			it = &confirming{einoTool: et, destructive: tr.Destructive}

			s.writes = append(s.writes, string(et.info.Name))
		}

		s.tools[et.info.Name] = it
		s.ordered = append(s.ordered, it)
	}

	return s
}

// Entry describes one registered tool: what kind of tool it is, its
// name, how it describes itself, and its implementation with no
// confirmation gate applied.
type Entry struct {
	Traits

	// Name is the tool's canonical identifier.
	Name Name

	// Info is the tool's own description. A surface that has to
	// announce the tool builds its description from this, so no surface
	// has to restate a schema the tool already owns.
	Info Info

	// Tool is the tool without its confirmation gate. Running it
	// performs the write immediately, so it belongs only to surfaces
	// whose client owns the approval story (the MCP server); the
	// assistant's chat loop must use Tools.
	Tool Runner
}

// Entries returns every tool in registration order, ungated. The MCP
// surface builds its tool list from this, minus the internal ones; the
// assistant's chat loop uses Tools, which keeps the confirmation gates.
func (s *Set) Entries() []Entry {
	return slices.Clone(s.entries)
}

// Entry returns the named tool, ungated, reporting whether the set owns
// it. Callers that want one specific tool ask here rather than searching
// Entries, so the registry stays the only thing that knows how tools are
// addressed.
func (s *Set) Entry(name Name) (Entry, bool) {
	for _, e := range s.entries {
		if e.Name == name {
			return e, true
		}
	}

	return Entry{}, false
}

// Tools returns every tool in registration order, ready to hand to the
// agent.
func (s *Set) Tools() []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(s.ordered))
	for _, t := range s.ordered {
		out = append(out, t)
	}

	return out
}

// Label returns a short line describing what the named tool is about to
// do, or an empty string for tools too noisy or too generic to
// announce. An unknown name is not an error: the agent may run tools
// this set does not own.
//
// Arguments the tool cannot read are an empty label too. The caller
// announces a call it is about to make, and that call is about to fail
// on the same arguments — the failure is the tool's to report, and
// announcing a status line derived from nothing would only precede it
// with a lie.
func (s *Set) Label(ctx context.Context, name Name, args json.RawMessage) string {
	t, ok := s.tools[name]
	if !ok {
		return ""
	}

	label, err := t.Title(ctx, args)
	if err != nil {
		return ""
	}

	return label
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

// Result is one tool call's outcome: the payload the model reads, and
// the documents the call changed.
type Result struct {
	// Output is the tool's result, serialised for the caller.
	Output string

	// Documents lists the documents this call created or changed, in
	// the order it touched them.
	//
	// It is recorded by the Input as the call mutates rather than read
	// back out of the arguments, so it is right for a call that changes
	// several documents and empty for one that changes none — neither
	// of which an argument can be asked about. A delete records
	// nothing: the document it names no longer exists to point at.
	Documents []string
}

// Runner runs one tool call in this package's own vocabulary: the raw
// JSON arguments in, the outcome out. Surfaces outside this package
// reach a tool through it, so none of them has to speak the agent
// framework's interface to run one.
type Runner interface {
	// Run should perform the call and report what it produced and what
	// it changed.
	Run(ctx context.Context, args json.RawMessage) (Result, error)
}

// registryTool is what the registry stores: a tool the agent can invoke
// that can also describe what it is about to do. The adapter and the
// confirmation gate wrapping it both satisfy it — the gate embeds the
// adapter — so Label reaches Title without asking which one it holds.
type registryTool interface {
	tool.InvokableTool

	// Title should return a short line describing what the tool is
	// about to do, or an empty string for tools too noisy or too
	// generic to announce.
	Title(ctx context.Context, args json.RawMessage) (string, error)
}
