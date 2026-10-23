package assistant

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_buildSystemPrompt(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DocumentID  string
		BranchID    string
		Result      string
		Contains    []string
		NotContains []string
	}{
		"No active document returns the base prompt verbatim": {
			Result: _basePrompt,
		},
		"Active document appends the current context section": {
			DocumentID: "doc-123",
			Contains: []string{
				_basePrompt,
				"## Current context",
				"`doc-123`",

				// the model reads and writes content as markup, keeping
				// ids.
				"replace_blocks, keeping the id of every element that stays",
				"branch_id",

				// the rubber duck stance needs its counterweight: an
				// explicit request for text is written, not interviewed
				// about.
				"When someone asks for text, write it",
			},
			NotContains: []string{"on branch"},
		},
		"Active branch is named": {
			// a branch other than the default one is named, and the
			// model is told to read and write with it.
			DocumentID: "doc-123",
			BranchID:   "draft",
			Contains: []string{
				"`doc-123` on branch `draft`",
				"write with branch_id `draft`",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := buildSystemPrompt(c.DocumentID, c.BranchID)

			if c.Result != "" {
				assert.Equal(t, c.Result, got)
			}

			for _, want := range c.Contains {
				assert.Contains(t, got, want)
			}

			for _, unwanted := range c.NotContains {
				assert.NotContains(t, got, unwanted)
			}

			// the model copies the prompt's own punctuation, so the text
			// carries no em dash, and it spells organisation the way the
			// product does.
			assert.NotContains(t, got, "\u2014")
			assert.NotContains(t, got, "organization")
		})
	}
}

func Test_MCPInstructions(t *testing.T) {
	t.Parallel()

	got := MCPInstructions()

	// the shared sections ship verbatim, so the two surfaces describe
	// the same workflow, content model, etiquette and style.
	assert.Contains(t, got, _workflowSection)
	assert.Contains(t, got, _blockModelSection)
	assert.Contains(t, got, _etiquetteSection)
	assert.Contains(t, got, _aestheticsSection)

	// Claude Code shows the first 2 KB of a server's instructions and
	// drops the rest.
	assert.Less(t, len(got), 2000)

	// an MCP client learns about branches and the protected rule here.
	assert.Contains(t, got, "names a branch by branch_id")
	assert.Contains(t, got, "protected branch is read-only")

	// metrics follow the built-in variables on both surfaces.
	assert.Contains(t, got, "Metrics follow the built-in metric_range and metric_refresh variables")

	// the persona and its confirmation flow are the chat surface's;
	// telling an MCP client about either would be a lie.
	assert.NotContains(t, got, "Rubber Duck")
	assert.NotContains(t, got, "confirmation")

	// the house style holds here as in the chat prompt.
	assert.NotContains(t, got, "—")
	assert.NotContains(t, got, "organization")
}

func Test_genModelInput(t *testing.T) {
	t.Parallel()

	// Result lists the content of every message after the system
	// prompt.
	cc := map[string]struct {
		Messages []*schema.Message
		Result   []string
	}{
		"Prompt is prepended to the conversation": {
			// the prompt is prepended to every run rather than stored in
			// the conversation, so it cannot be summarised or cleared
			// away.
			Messages: []*schema.Message{schema.UserMessage("hello")},
			Result:   []string{"hello"},
		},
		"Stale system prompts are dropped": {
			// a restored conversation carries the system prompts
			// previous runs prepended; only the fresh one may reach the
			// model, or the prompt stacks up a copy per turn.
			Messages: []*schema.Message{
				schema.SystemMessage("stale prompt"),
				schema.UserMessage("hello"),
				schema.AssistantMessage("hi", nil),
				schema.SystemMessage("another stale prompt"),
			},
			Result: []string{"hello", "hi"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			msgs, err := genModelInput(context.Background(), "", &adk.AgentInput{
				Messages: c.Messages,
			})
			require.NoError(t, err)
			require.Len(t, msgs, len(c.Result)+1)

			assert.Equal(t, schema.System, msgs[0].Role)
			assert.NotEmpty(t, msgs[0].Content)
			assert.NotEqual(t, "stale prompt", msgs[0].Content)

			for i, want := range c.Result {
				assert.Equal(t, want, msgs[i+1].Content)
			}
		})
	}
}

func Test_sessionString(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		InRun  bool
		Values map[string]any
		Result string
	}{
		"Outside a run nothing is stored": {},
		"Run without the key stores nothing": {
			InRun: true,
		},
		"Stored string is returned": {
			InRun:  true,
			Values: map[string]any{_sessionKeyActiveDocument: "doc-1"},
			Result: "doc-1",
		},
		"Stored value of another type is ignored": {
			InRun:  true,
			Values: map[string]any{_sessionKeyActiveDocument: 42},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if !c.InRun {
				assert.Equal(t, c.Result, sessionString(context.Background(), _sessionKeyActiveDocument))

				return
			}

			// adk keeps session values on a context only a run builds,
			// so the value is read from inside one.
			var got string

			agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
				Name:        _agentName,
				Description: _agentDescription,
				Model:       stubChatModel(),
				GenModelInput: func(ctx context.Context, _ string, input *adk.AgentInput) ([]*schema.Message, error) {
					got = sessionString(ctx, _sessionKeyActiveDocument)

					return input.Messages, nil
				},
			})
			require.NoError(t, err)

			iter := adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: agent}).Run(
				context.Background(),
				[]*schema.Message{schema.UserMessage("hi")},
				adk.WithSessionValues(c.Values),
			)

			for {
				if _, ok := iter.Next(); !ok {
					break
				}
			}

			assert.Equal(t, c.Result, got)
		})
	}
}
