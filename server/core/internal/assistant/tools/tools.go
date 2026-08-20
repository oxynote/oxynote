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

	// ordered lists the tools in registration order. Tools returns it so
	// every session presents the model the same tool order, which keeps
	// provider prompt caching effective.
	ordered []tool.InvokableTool

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
		s.ordered = append(s.ordered, t)
	}

	return s
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
