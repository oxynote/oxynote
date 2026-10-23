package middleware

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTool is a tool that only describes itself.
type stubTool struct {
	info *schema.ToolInfo
	err  error
}

// Info returns the stub's description or error.
func (st stubTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return st.info, st.err
}

// stubInfo describes a tool with one string argument.
func stubInfo(name string) *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: name,
		Desc: "Does " + name + ".",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"id": {Type: schema.String, Desc: "The id.", Required: true},
		}),
	}
}

// testLoader builds a loader over two groups of stub tools.
func testLoader(t *testing.T) *Loader {
	t.Helper()

	l, err := NewLoader(context.Background(), map[tools.Group][]tool.BaseTool{
		tools.GroupTags:  {stubTool{info: stubInfo("list_tags")}, stubTool{info: stubInfo("create_tag")}},
		tools.GroupHooks: {stubTool{info: stubInfo("list_hooks")}},
	})
	require.NoError(t, err)

	return l
}

// loadMessage is the tool message a load of the named tools leaves in
// the conversation.
func loadMessage(infos ...*schema.ToolInfo) *schema.Message {
	return &schema.Message{
		Role:       schema.Tool,
		ToolName:   _loadToolsName,
		ToolCallID: "call",
		UserInputMultiContent: []schema.MessageInputPart{{
			Type:             schema.ChatMessagePartTypeToolSearchResult,
			ToolSearchResult: &schema.ToolSearchResult{Tools: infos},
		}},
	}
}

// referenced returns the names a load message references.
func referenced(t *testing.T, msg *schema.Message) []string {
	t.Helper()

	var out []string

	for _, part := range msg.UserInputMultiContent {
		require.NotNil(t, part.ToolSearchResult)

		for _, info := range part.ToolSearchResult.Tools {
			out = append(out, info.Name)
		}
	}

	return out
}

func Test_NewLoader(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Groups map[tools.Group][]tool.BaseTool
		Err    error
	}{
		"Group without a summary": {
			Groups: map[tools.Group][]tool.BaseTool{"unknown": {stubTool{info: stubInfo("x")}}},
			Err:    assert.AnError,
		},
		"Error returned by Info": {
			Groups: map[tools.Group][]tool.BaseTool{tools.GroupTags: {stubTool{err: assert.AnError}}},
			Err:    assert.AnError,
		},
		"Successful creation": {
			Groups: map[tools.Group][]tool.BaseTool{
				tools.GroupTags:  {stubTool{info: stubInfo("list_tags")}, stubTool{info: stubInfo("create_tag")}},
				tools.GroupHooks: {stubTool{info: stubInfo("list_hooks")}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			l, err := NewLoader(context.Background(), c.Groups)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, l)
			assert.NotNil(t, l.BaseChatModelAgentMiddleware)
			assert.ElementsMatch(t, []string{"list_tags", "create_tag", "list_hooks"}, func() []string {
				var names []string

				for n := range l.grouped {
					names = append(names, n)
				}

				return names
			}())
			require.Len(t, l.load.groups[tools.GroupTags], 2)
			assert.Equal(t, "list_tags", l.load.groups[tools.GroupTags][0].Name)
			assert.Equal(t, "create_tag", l.load.groups[tools.GroupTags][1].Name)
			require.Len(t, l.load.groups[tools.GroupHooks], 1)
		})
	}
}

func Test_Loader_BeforeAgent(t *testing.T) {
	t.Parallel()

	l := testLoader(t)
	own := stubTool{info: stubInfo("get_document")}
	in := &adk.ChatModelAgentContext{Tools: []tool.BaseTool{own}}

	_, out, err := l.BeforeAgent(context.Background(), in)
	require.NoError(t, err)

	require.Len(t, out.Tools, 2)
	assert.Equal(t, own, out.Tools[0])
	assert.Same(t, l.load, out.Tools[1])

	// the agent's own context is left as it was.
	assert.Len(t, in.Tools, 1)
}

func Test_Loader_BeforeModelRewriteState(t *testing.T) {
	t.Parallel()

	l := testLoader(t)
	user := schema.UserMessage("tag it")
	load := loadMessage(stubInfo("list_tags"), stubInfo("create_tag"))
	other := schema.ToolMessage("{}", "other", schema.WithToolName("get_document"))
	in := &adk.ChatModelAgentState{
		Messages:  []*schema.Message{user, load, other, loadMessage(stubInfo("create_tag"))},
		ToolInfos: []*schema.ToolInfo{stubInfo("get_document"), stubInfo("list_tags"), stubInfo("list_hooks")},
	}

	_, out, err := l.BeforeModelRewriteState(context.Background(), in, &adk.ModelContext{})
	require.NoError(t, err)

	// only the always-loaded tools reach the model up front.
	require.Len(t, out.ToolInfos, 1)
	assert.Equal(t, "get_document", out.ToolInfos[0].Name)

	require.Len(t, out.Messages, 4)
	assert.Same(t, user, out.Messages[0])
	assert.Same(t, other, out.Messages[2])
	assert.Equal(t, []string{"list_tags", "create_tag"}, referenced(t, out.Messages[1]))
	assert.Equal(t, "Already loaded: create_tag.", out.Messages[3].Content)
	assert.Empty(t, out.Messages[3].UserInputMultiContent)
}

func Test_Loader_restore(t *testing.T) {
	t.Parallel()

	// a conversation read back from storage, as the history keeps it.
	stored := func(t *testing.T, msg *schema.Message) *schema.Message {
		t.Helper()

		raw, err := json.Marshal(msg)
		require.NoError(t, err)

		var out schema.Message
		require.NoError(t, json.Unmarshal(raw, &out))

		return &out
	}

	cc := map[string]struct {
		Msg      func(t *testing.T) *schema.Message
		Loaded   map[string]bool
		Same     bool
		Refs     []string
		Text     string
		Restored bool
	}{
		"Message without references is returned as is": {
			Msg: func(*testing.T) *schema.Message {
				return schema.ToolMessage("{}", "c", schema.WithToolName(_loadToolsName))
			},
			Same: true,
		},
		"Reference to a tool outside every group is dropped": {
			Msg:  func(*testing.T) *schema.Message { return loadMessage(stubInfo("get_document")) },
			Same: true,
		},
		"Stored reference gets its definition back": {
			Msg:      func(t *testing.T) *schema.Message { return stored(t, loadMessage(stubInfo("list_tags"))) },
			Refs:     []string{"list_tags"},
			Restored: true,
		},
		"Tools loaded earlier are left out": {
			Msg:    func(*testing.T) *schema.Message { return loadMessage(stubInfo("list_tags"), stubInfo("create_tag")) },
			Loaded: map[string]bool{"list_tags": true},
			Refs:   []string{"create_tag"},
		},
		"Load adding nothing becomes text": {
			Msg:    func(*testing.T) *schema.Message { return loadMessage(stubInfo("list_hooks")) },
			Loaded: map[string]bool{"list_hooks": true},
			Text:   "Already loaded: list_hooks.",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			l := testLoader(t)
			loaded := map[string]bool{}
			maps.Copy(loaded, c.Loaded)

			msg := c.Msg(t)
			got := l.restore(msg, loaded)

			if c.Same {
				assert.Same(t, msg, got)

				return
			}

			assert.Equal(t, c.Text, got.Content)

			if c.Text != "" {
				assert.Empty(t, got.UserInputMultiContent)

				return
			}

			assert.Equal(t, c.Refs, referenced(t, got))

			for _, name := range c.Refs {
				assert.True(t, loaded[name])
			}

			if c.Restored {
				assert.Same(t, l.grouped["list_tags"], got.UserInputMultiContent[0].ToolSearchResult.Tools[0])
			}
		})
	}
}

func Test_loadTools_Info(t *testing.T) {
	t.Parallel()

	l := testLoader(t)

	info, err := l.load.Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, _loadToolsName, info.Name)
	assert.Equal(t, "Load a group of tools so you can call them. Document and block tools are always loaded. Groups: tags (list, create, update, delete and assign tags), hooks (freshness hooks and review reminders on a document).", info.Desc)

	js, err := info.ToJSONSchema()
	require.NoError(t, err)

	raw, err := json.Marshal(js)
	require.NoError(t, err)

	assert.JSONEq(t, `{"type":"object","properties":{"group":{"type":"string","description":"The group to load.","enum":["tags","hooks"]}},"required":["group"]}`, string(raw))
}

func Test_loadTools_InvokableRun(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args  string
		Text  string
		Names []string
	}{
		"Invalid JSON is answered in text": {
			Args: "{",
			Text: `load_tools: invalid input: unexpected end of JSON input; pass {"group": one of tags, hooks}`,
		},
		"Unknown group is answered in text": {
			Args: `{"group":"documents"}`,
			Text: `load_tools: no group "documents"; pass one of tags, hooks`,
		},
		"Group loads its definitions": {
			Args:  `{"group":"tags"}`,
			Names: []string{"list_tags", "create_tag"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			l := testLoader(t)

			got, err := l.load.InvokableRun(context.Background(), &schema.ToolArgument{Text: c.Args})
			require.NoError(t, err)
			require.Len(t, got.Parts, 1)

			if c.Text != "" {
				assert.Equal(t, schema.ToolPartTypeText, got.Parts[0].Type)
				assert.Equal(t, c.Text, got.Parts[0].Text)

				return
			}

			assert.Equal(t, schema.ToolPartTypeToolSearchResult, got.Parts[0].Type)
			require.NotNil(t, got.Parts[0].ToolSearchResult)
			assert.Equal(t, l.load.groups[tools.GroupTags], got.Parts[0].ToolSearchResult.Tools)
			assert.Len(t, got.Parts[0].ToolSearchResult.Tools, len(c.Names))
		})
	}
}

func Test_loadTools_offered(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tags, hooks", testLoader(t).load.offered())
}

func Test_textResult(t *testing.T) {
	t.Parallel()

	assert.Equal(t, &schema.ToolResult{Parts: []schema.ToolOutputPart{{Type: schema.ToolPartTypeText, Text: "hi"}}}, textResult("hi"))
}

func Test_toolNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a, b", toolNames([]*schema.ToolInfo{{Name: "a"}, {Name: "b"}}))
}
