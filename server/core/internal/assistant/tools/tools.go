// Package tools owns the AI assistant's tools. They are grouped by what
// they act on, such as documents, blocks or tags. Each describes itself
// and does its work in this package's own vocabulary, and only eino.go
// speaks the agent framework's. Every tool runs on the server.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/rs/xid"
)

// Read tools are never confirmed. The assistant runs them at once.
const (
	NameListDocuments   Name = "list_documents"
	NameGetDocument     Name = "get_document"
	NameSearchDocuments Name = "search_documents"
	NameListTags        Name = "list_tags"
	NameListHooks       Name = "list_hooks"
	NameListVariables   Name = "list_variables"
)

// Data-source tools read the organisation's outbound connections. They reach systems outside Oxynote, so a surface that
// gates by scope asks Traits.DataSource rather than lumping them in
// with the document reads.
const (
	NameListDataSources       Name = "list_data_sources"
	NameGetDataSourceMetadata Name = "get_data_source_metadata"
	NameListPrometheusLabels  Name = "list_prometheus_labels"
	NameListPrometheusSeries  Name = "list_prometheus_series"
	NameListLogFields         Name = "list_log_fields"
	NameQueryDataSource       Name = "query_data_source"
)

// Write tools are always confirmed, deletes included.
const (
	NameCreateDocument   Name = "create_document"
	NameDeleteDocument   Name = "delete_document"
	NameUpdateDocument   Name = "update_document"
	NameInsertBlocks     Name = "insert_blocks"
	NameReplaceBlocks    Name = "replace_blocks"
	NameDeleteBlock      Name = "delete_block"
	NameMoveBlock        Name = "move_block"
	NameCreateTag        Name = "create_tag"
	NameUpdateTag        Name = "update_tag"
	NameDeleteTag        Name = "delete_tag"
	NameSetTagAssignment Name = "set_tag_assignment"

	NameCreateVariableGroup Name = "create_variable_group"
	NameUpdateVariableGroup Name = "update_variable_group"
	NameDeleteVariableGroup Name = "delete_variable_group"
	NameCreateVariable      Name = "create_variable"
	NameUpdateVariable      Name = "update_variable"
	NameDeleteVariable      Name = "delete_variable"
	NameSetVariableValue    Name = "set_variable_value"
	NameCreateHook          Name = "create_hook"
	NameUpdateHook          Name = "update_hook"
	NameDeleteHook          Name = "delete_hook"
)

// The groups of tools the chat assistant loads on request.
const (
	GroupTags        Group = "tags"
	GroupVariables   Group = "variables"
	GroupHooks       Group = "hooks"
	GroupDataSources Group = "data_sources"
)

// Name is the canonical identifier of one tool the assistant
// exposes to the model. Names use snake_case; they double as the
// model-facing tool name and the protocol confirm-action's "tool"
// field.
type Name string

// Group is a set of tools the chat assistant loads on request. Most
// conversations never use them, and their definitions are over half of
// the tool list. Values are snake_case like tool names, because the
// model names a group to load it.
type Group string

// Set is the tools one session offers the model, built once from the
// session's wiring. It is the only list of tools.
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
	all := []registration{
		register[listDocuments](),
		register[getDocument](),
		register[listTags](),
		register[listHooks](),
		register[listVariables](),
		register[searchDocuments](),
		register[listDataSources](),
		register[getDataSourceMetadata](),
		register[listPrometheusLabels](),
		register[listPrometheusSeries](),
		register[listLogFields](),
		register[queryDataSource](),

		register[createDocument](),
		register[deleteDocument](),
		register[updateDocument](),
		register[insertBlocks](),
		register[replaceBlocks](),
		register[deleteBlock](),
		register[moveBlock](),
		register[createTag](),
		register[updateTag](),
		register[deleteTag](),
		register[setTagAssignment](),
		register[createVariableGroup](),
		register[updateVariableGroup](),
		register[deleteVariableGroup](),
		register[createVariable](),
		register[updateVariable](),
		register[deleteVariable](),
		register[setVariableValue](),
		register[createHook](),
		register[updateHook](),
		register[deleteHook](),

		register[readToolOutput](),
	}

	s := &Set{tools: make(map[Name]registryTool, len(all))}

	for _, reg := range all {
		et := newEinoTool(reg, deps)
		tr := et.info.Traits

		s.entries = append(s.entries, Entry{
			Info: et.info,
			Tool: et,
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
		if e.Info.Name == name {
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

// Grouped returns the tools of each group in registration order, as
// Tools returns them.
func (s *Set) Grouped() map[Group][]tool.BaseTool {
	out := make(map[Group][]tool.BaseTool)

	for i, e := range s.entries {
		if g := e.Info.Traits.Group; g != "" {
			out[g] = append(out[g], s.ordered[i])
		}
	}

	return out
}

// Label returns the status line of the named tool's call, or an empty
// string for a tool this set does not own, a tool without a title, or
// arguments the tool cannot read.
func (s *Set) Label(ctx context.Context, name Name, args json.RawMessage) string {
	t, ok := s.tools[name]
	if !ok {
		return ""
	}

	return t.Title(ctx, args)
}

// WriteNames returns every tool that mutates a document, which are
// exactly the gated ones. The context middlewares never clear their
// results.
func (s *Set) WriteNames() []string {
	return slices.Clone(s.writes)
}

// Entry describes one registered tool: how it describes itself, and its
// implementation with no confirmation gate applied.
type Entry struct {
	// Info is the tool's own description and traits. A surface that has
	// to announce the tool builds its description from this, so no
	// surface has to restate a schema the tool already owns.
	Info Info

	// Tool is the tool without its confirmation gate. Running it
	// performs the write immediately, so it belongs only to surfaces
	// whose client owns the approval story (the MCP server); the
	// assistant's chat loop must use Tools.
	Tool Runner
}

// registration is one tool as the registry holds it: its description,
// and how a call's arguments become the tool.
type registration struct {
	// info is the tool's description. It never depends on a call.
	info Info

	// decode reads and validates a call's arguments into the tool.
	decode func(args json.RawMessage) (Tool, error)
}

// register returns the registration of the tool type T.
func register[T Tool]() registration {
	var zero T

	return registration{
		info:   zero.Info(),
		decode: decodeTool[T],
	}
}

// decodeTool decodes a call's arguments into a T and validates them.
// json/v2 names the argument that failed. An empty payload reads as an
// empty object, since a provider may send none to a tool without
// parameters.
func decodeTool[T Tool](args json.RawMessage) (Tool, error) {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}

	var tl T

	if err := jsonv2.Unmarshal(args, &tl); err != nil {
		// a malformed id is nearly always a miscopied one, so the
		// message says how to get it right rather than naming Go types.
		var serr *jsonv2.SemanticError
		if errors.As(err, &serr) && serr.GoType == reflect.TypeFor[xid.ID]() {
			return nil, fmt.Errorf(
				"%s is not an id; copy all 20 characters exactly from the result that gave it",
				strings.TrimPrefix(string(serr.JSONPointer), "/"),
			)
		}

		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if err := tl.Validate(); err != nil {
		return nil, err
	}

	return tl, nil
}

// Runner runs one tool call in this package's own vocabulary: the raw
// JSON arguments in, the outcome out. Surfaces outside this package
// reach a tool through it, so none of them has to speak the agent
// framework's interface to run one.
type Runner interface {
	// Run should perform the call and return its result serialised for
	// the caller.
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// registryTool is what the registry stores: a tool the agent can invoke
// that can also describe what it is about to do. The adapter and the
// gate that embeds it both satisfy it, so Label reaches Title without
// asking which one it holds.
type registryTool interface {
	tool.InvokableTool

	// Title should return a short line describing what the tool is
	// about to do, or an empty string for tools too noisy or too
	// generic to announce.
	Title(ctx context.Context, args json.RawMessage) string
}
