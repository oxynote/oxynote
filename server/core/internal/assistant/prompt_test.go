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
	assert.Equal(t, _basePrompt, buildSystemPrompt(""))

	// an active document appends the current-context section
	got := buildSystemPrompt("doc-123")
	assert.Contains(t, got, _basePrompt)
	assert.Contains(t, got, "## Current context")
	assert.Contains(t, got, "`doc-123`")
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
