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

	// the prompt states exactly the metric values Validate accepts. The
	// model is told the metric configuration here rather than in a tool
	// schema — a block's attrs depend on its type, which the shared
	// block schema cannot express — so this is what keeps the two from
	// drifting apart.
	for attr, values := range block.MetricEnums() {
		for _, v := range values {
			assert.Contains(t, got, v, "%s value %q is missing from the prompt", attr, v)
		}
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
	} {
		assert.Contains(t, got, attr, "attr %q is missing from the prompt", attr)
	}

	// the model authors metric blocks now, so the prompt must not still
	// tell it not to.
	assert.NotContains(t, got, "do not author")
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
}

func Test_activeDocumentID(t *testing.T) {
	t.Parallel()

	// outside a run there is no session, so no document is active.
	assert.Empty(t, activeDocumentID(context.Background()))
}
