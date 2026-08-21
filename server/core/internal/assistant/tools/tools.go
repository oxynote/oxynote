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
	tools map[Name]tool.InvokableTool

	// ordered lists the tools in registration order. Tools returns it so
	// every session presents the model the same tool order, which keeps
	// provider prompt caching effective.
	ordered []tool.InvokableTool

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

	all := []ReadTool{
		listDocuments{},
		getDocument{},
		readDocumentSummary{},
		readBlock{},
		searchDocuments{},

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

	s := &Set{tools: make(map[Name]tool.InvokableTool, len(all))}

	for _, tl := range all {
		et := newEinoTool(tl, deps)
		tr := tl.Traits()

		s.entries = append(s.entries, Entry{
			Traits: tr,
			Name:   et.info.Name,
			Tool:   et,
		})

		var it tool.InvokableTool = et

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
// name, and its implementation with no confirmation gate applied.
type Entry struct {
	Traits

	// Name is the tool's canonical identifier.
	Name Name

	// Tool is the tool without its confirmation gate. Running it
	// performs the write immediately, so it belongs only to surfaces
	// whose client owns the approval story (the MCP server); the
	// assistant's chat loop must use Tools.
	Tool tool.InvokableTool
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
func (s *Set) Label(ctx context.Context, name Name, args json.RawMessage) string {
	t, ok := s.tools[name]
	if !ok {
		return ""
	}

	return unwrap(t).Title(ctx, args)
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

// unwrap returns the adapter underneath a confirmation gate, or the
// adapter itself when it is not gated.
func unwrap(t tool.InvokableTool) *einoTool {
	if c, ok := t.(*confirming); ok {
		return c.einoTool
	}

	// NOCOV: the registry only ever holds adapters, gated or not.
	et, _ := t.(*einoTool)

	return et
}
