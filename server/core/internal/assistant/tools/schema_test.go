package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AnthropicTools(t *testing.T) {
	t.Parallel()

	defs := AnthropicTools()

	// one definition per known tool, in the read-then-write order
	expected := append(append([]Name{}, _readNames...), _writeNames...)
	require.Len(t, defs, len(expected))

	seen := map[string]bool{}

	for i, def := range defs {
		tool := def.OfTool
		require.NotNil(t, tool, "definition %d must be a plain tool", i)

		name := tool.Name
		assert.True(t, IsValid(Name(name)), "unknown tool name %q", name)
		assert.False(t, seen[name], "duplicate tool name %q", name)
		assert.NotEmpty(t, tool.Description.Value, "tool %q needs a description", name)
		assert.NotNil(t, tool.InputSchema.Properties, "tool %q needs schema properties", name)

		seen[name] = true
	}

	// the declared order matches the canonical read/write grouping
	for i, def := range defs {
		assert.Equal(t, string(expected[i]), def.OfTool.Name)
	}
}
