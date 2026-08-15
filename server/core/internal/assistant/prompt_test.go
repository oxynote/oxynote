package assistant

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
