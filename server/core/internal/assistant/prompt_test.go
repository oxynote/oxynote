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

	// no active document returns the base prompt verbatim
	assert.Equal(t, _basePrompt, buildSystemPrompt("", ""))

	// an active document appends the current-context section
	got := buildSystemPrompt("doc-123", "")
	assert.Contains(t, got, _basePrompt)
	assert.Contains(t, got, "## Current context")
	assert.Contains(t, got, "`doc-123`")
	assert.NotContains(t, got, "on branch")

	// a branch other than the default one is named, and the model is
	// told to read and write with it.
	branched := buildSystemPrompt("doc-123", "draft")
	assert.Contains(t, branched, "`doc-123` on branch `draft`")
	assert.Contains(t, branched, "write with branch_id `draft`")
	assertHouseStyle(t, branched)

	// the model reads and writes content as markup, keeping ids.
	assert.Contains(t, got, "replace_blocks, keeping the id of every element that stays")
	assert.Contains(t, got, "branch_id")

	// the rubber duck stance needs its counterweight: an explicit
	// request for text is written, not interviewed about.
	assert.Contains(t, got, "When someone asks for text, write it")

	assertHouseStyle(t, got)
}

// assertHouseStyle checks the rules every model-facing text follows:
// the model copies the prompt's own punctuation, so the text carries
// no em dash, and it spells organisation the way the product does.
func assertHouseStyle(t *testing.T, text string) {
	t.Helper()

	assert.NotContains(t, text, "\u2014")
	assert.NotContains(t, text, "organization")
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

	// the persona and its confirmation flow are the chat surface's;
	// telling an MCP client about either would be a lie.
	assert.NotContains(t, got, "Rubber Duck")
	assert.NotContains(t, got, "confirmation")

	assertHouseStyle(t, got)
}

func Test_genModelInput(t *testing.T) {
	t.Parallel()

	msgs, err := genModelInput(context.Background(), "", &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	// the prompt is prepended to every run rather than stored in the
	// conversation, so it cannot be summarised or cleared away.
	assert.Equal(t, schema.System, msgs[0].Role)
	assert.NotEmpty(t, msgs[0].Content)
	assert.Equal(t, "hello", msgs[1].Content)

	// a restored conversation carries the system prompts previous runs
	// prepended; only the fresh one may reach the model, or the prompt
	// stacks up a copy per turn.
	msgs, err = genModelInput(context.Background(), "", &adk.AgentInput{
		Messages: []*schema.Message{
			schema.SystemMessage("stale prompt"),
			schema.UserMessage("hello"),
			schema.AssistantMessage("hi", nil),
			schema.SystemMessage("another stale prompt"),
		},
	})
	require.NoError(t, err)
	require.Len(t, msgs, 3)

	assert.Equal(t, schema.System, msgs[0].Role)
	assert.NotEqual(t, "stale prompt", msgs[0].Content)
	assert.Equal(t, "hello", msgs[1].Content)
	assert.Equal(t, "hi", msgs[2].Content)
}

func Test_sessionString(t *testing.T) {
	t.Parallel()

	// outside a run there is no session, so nothing is stored.
	assert.Empty(t, sessionString(context.Background(), _sessionKeyActiveDocument))
}
