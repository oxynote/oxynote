package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

// _loadToolsName is the tool the model calls to load a group.
const _loadToolsName = "load_tools"

// _groupOrder is the order load_tools names the groups in, which keeps
// its description the same in every conversation.
var _groupOrder = []tools.Group{
	tools.GroupTags,
	tools.GroupVariables,
	tools.GroupHooks,
	tools.GroupDataSources,
}

// _groupSummaries says what each group is for, so the model knows which
// one a request needs before it sees any of their tools.
var _groupSummaries = map[tools.Group]string{
	tools.GroupTags:        "list, create, update, delete and assign tags",
	tools.GroupVariables:   "variable groups, variables and their values, metric_range and metric_refresh included",
	tools.GroupHooks:       "freshness hooks and review reminders on a document",
	tools.GroupDataSources: "list data sources, their metrics and tables, and run queries before adding a chart",
}

// Loader keeps the grouped tools out of the model's tool list until the
// model loads their group with load_tools. Most conversations never use
// them, and their definitions are over half of the list.
//
// A load answers with the group's definitions as tool references. The
// Anthropic API expands them where they stand, outside the cached
// prefix, so the tool list never changes and the prompt cache holds.
// Only a model that expands references can use it.
type Loader struct {
	*adk.BaseChatModelAgentMiddleware

	// grouped maps each grouped tool's name to its definition.
	grouped map[string]*schema.ToolInfo

	// load is the load_tools tool the agent is given.
	load *loadTools
}

// NewLoader creates a fresh instance of Loader over the tools of each
// group.
func NewLoader(ctx context.Context, groups map[tools.Group][]tool.BaseTool) (*Loader, error) {
	l := &Loader{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		grouped:                      make(map[string]*schema.ToolInfo),
		load:                         &loadTools{groups: make(map[tools.Group][]*schema.ToolInfo, len(groups))},
	}

	for _, g := range slices.Sorted(maps.Keys(groups)) {
		if _, ok := _groupSummaries[g]; !ok {
			return nil, fmt.Errorf("group %q has no summary", g)
		}

		for _, t := range groups[g] {
			info, err := t.Info(ctx)
			if err != nil {
				return nil, fmt.Errorf("describing a tool of group %q: %w", g, err)
			}

			l.grouped[info.Name] = info
			l.load.groups[g] = append(l.load.groups[g], info)
		}
	}

	return l, nil
}

// BeforeAgent gives the agent load_tools. The grouped tools stay in the
// agent's tool set, so a loaded one runs like any other.
func (l *Loader) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	next := *runCtx
	next.Tools = append(slices.Clone(runCtx.Tools), l.load)

	return ctx, &next, nil
}

// BeforeModelRewriteState hides the grouped tools from the model, and
// rewrites the results of earlier loads: a stored conversation loses the
// definitions' schemas, so each reference gets its definition back, and
// a group loaded twice keeps its references only in the first load,
// because the API refuses a tool referenced twice.
func (l *Loader) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	state.ToolInfos = slices.DeleteFunc(slices.Clone(state.ToolInfos), func(info *schema.ToolInfo) bool {
		_, ok := l.grouped[info.Name]

		return ok
	})

	loaded := make(map[string]bool)
	msgs := slices.Clone(state.Messages)

	for i, msg := range msgs {
		if msg.Role != schema.Tool || msg.ToolName != _loadToolsName {
			continue
		}

		msgs[i] = l.restore(msg, loaded)
	}

	state.Messages = msgs

	return ctx, state, nil
}

// restore returns a load's result with its references pointing at the
// current definitions, leaving out the tools loaded already. A load that
// adds nothing becomes plain text.
func (l *Loader) restore(msg *schema.Message, loaded map[string]bool) *schema.Message {
	var fresh, repeat []*schema.ToolInfo

	for _, part := range msg.UserInputMultiContent {
		if part.Type != schema.ChatMessagePartTypeToolSearchResult || part.ToolSearchResult == nil {
			continue
		}

		for _, t := range part.ToolSearchResult.Tools {
			info, ok := l.grouped[t.Name]

			switch {
			case !ok:
				continue
			case loaded[t.Name]:
				repeat = append(repeat, info)
			default:
				loaded[t.Name] = true

				fresh = append(fresh, info)
			}
		}
	}

	if len(fresh) == 0 && len(repeat) == 0 {
		return msg
	}

	out := *msg

	if len(fresh) == 0 {
		out.UserInputMultiContent = nil
		out.Content = "Already loaded: " + toolNames(repeat) + "."

		return &out
	}

	out.UserInputMultiContent = []schema.MessageInputPart{{
		Type:             schema.ChatMessagePartTypeToolSearchResult,
		ToolSearchResult: &schema.ToolSearchResult{Tools: fresh},
	}}

	return &out
}

// loadTools is the tool the model calls to load a group. It answers
// with the group's definitions, which the model's provider turns into
// tool references.
type loadTools struct {
	// groups maps each group to its tools' definitions.
	groups map[tools.Group][]*schema.ToolInfo
}

// Info describes load_tools: the groups it offers and what each is for.
func (lt *loadTools) Info(_ context.Context) (*schema.ToolInfo, error) {
	names := make([]string, 0, len(_groupOrder))
	summaries := make([]string, 0, len(_groupOrder))

	for _, g := range _groupOrder {
		if _, ok := lt.groups[g]; !ok {
			continue
		}

		names = append(names, string(g))
		summaries = append(summaries, fmt.Sprintf("%s (%s)", g, _groupSummaries[g]))
	}

	return &schema.ToolInfo{
		Name: _loadToolsName,
		Desc: "Load a group of tools so you can call them. Document and block tools are always loaded. Groups: " + strings.Join(summaries, ", ") + ".",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"group": {
				Type:     schema.String,
				Desc:     "The group to load.",
				Enum:     names,
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun answers with the definitions of the group the model
// named. A group it does not offer is answered in text naming the ones
// it does, so the model can try again rather than the turn ending.
func (lt *loadTools) InvokableRun(_ context.Context, arg *schema.ToolArgument, _ ...tool.Option) (*schema.ToolResult, error) {
	var in struct {
		Group tools.Group `json:"group"`
	}

	if err := json.Unmarshal([]byte(arg.Text), &in); err != nil {
		return textResult(fmt.Sprintf("load_tools: invalid input: %s; pass {\"group\": one of %s}", err.Error(), lt.offered())), nil
	}

	infos, ok := lt.groups[in.Group]
	if !ok {
		return textResult(fmt.Sprintf("load_tools: no group %q; pass one of %s", in.Group, lt.offered())), nil
	}

	return &schema.ToolResult{Parts: []schema.ToolOutputPart{{
		Type:             schema.ToolPartTypeToolSearchResult,
		ToolSearchResult: &schema.ToolSearchResult{Tools: infos},
	}}}, nil
}

// offered lists the groups load_tools offers, in its own order.
func (lt *loadTools) offered() string {
	names := make([]string, 0, len(lt.groups))

	for _, g := range _groupOrder {
		if _, ok := lt.groups[g]; ok {
			names = append(names, string(g))
		}
	}

	return strings.Join(names, ", ")
}

// textResult wraps a message in a tool result.
func textResult(text string) *schema.ToolResult {
	return &schema.ToolResult{Parts: []schema.ToolOutputPart{{
		Type: schema.ToolPartTypeText,
		Text: text,
	}}}
}

// toolNames joins the definitions' names for a sentence.
func toolNames(infos []*schema.ToolInfo) string {
	names := make([]string, 0, len(infos))

	for _, info := range infos {
		names = append(names, info.Name)
	}

	return strings.Join(names, ", ")
}
