package assistant

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_buildSystemPrompt(t *testing.T) {
	t.Parallel()

	// no active document returns the base prompt verbatim
	assert.Equal(t, _basePrompt, buildSystemPrompt(""))

	// an active document appends the current-context section
	got := buildSystemPrompt("doc-123")
	assert.Contains(t, got, _basePrompt)
	assert.Contains(t, got, "## Current context")
	assert.Contains(t, got, "`doc-123`")

	// width is the one metric enum the prompt is still the only source
	// for: attrs is a single field shared by every block type, and
	// width means something else on an image, so the tool schema cannot
	// publish it. The rest moved there, where Test_attrProps pins them.
	for _, v := range block.MetricEnums()[document.AttrWidth] {
		assert.Contains(t, got, v, "width value %q is missing from the prompt", v)
	}

	for _, attr := range []string{
		document.AttrDataSourceID,
		document.AttrVisualizationType,
		document.AttrQueries,
		document.AttrTimeRange,
		document.AttrRefreshInterval,
		document.AttrThresholds,
		document.AttrBaseThresholdColor,
		document.AttrDecimals,
		document.AttrUnitType,
		document.AttrUnitCustom,
		document.AttrAxisBoundsMin,
		document.AttrAxisBoundsMax,
		document.AttrSimulationPreset,
	} {
		assert.Contains(t, got, attr, "attr %q is missing from the prompt", attr)
	}

	// the model authors metric blocks now, so the prompt must not still
	// tell it not to.
	assert.NotContains(t, got, "do not author")

	// the block model is the only schema the chat model sees, so every
	// type the tools accept has to be in it.
	for _, bt := range block.Types() {
		assert.Contains(t, _blockModelSection, "| "+bt+" |", "block type %q is missing from the prompt", bt)
	}

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
	// the same block model, etiquette, and aesthetics.
	assert.Contains(t, got, _blockModelSection)
	assert.Contains(t, got, _etiquetteSection)
	assert.Contains(t, got, _aestheticsSection)

	// reordering is the etiquette rule the surface exists to teach.
	assert.Contains(t, got, "move_block")

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

func Test_activeDocumentID(t *testing.T) {
	t.Parallel()

	// outside a run there is no session, so no document is active.
	assert.Empty(t, activeDocumentID(context.Background()))
}
