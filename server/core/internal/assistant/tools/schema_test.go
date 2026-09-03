package tools

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_stringProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{"type": "string", "description": "a doc"}, stringProp("a doc"))
}

func Test_documentIDProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		map[string]any{"document_id": map[string]any{"type": "string", "description": "a doc"}},
		documentIDProp("a doc"))
}

func Test_attrProps(t *testing.T) {
	t.Parallel()

	got := attrProps()

	// heading level is the one non-metric enum, and the only integer
	// one: a client sending "2" rather than 2 is sending the wrong type.
	level, ok := got[document.AttrLevel].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, _typeInteger, level[_keyType])
	assert.Equal(t, []int{1, 2, 3}, level[_keyEnum])

	// every metric enum is published except width, whose name means a
	// size on an image and so cannot carry one set of values here.
	enums := block.MetricEnums()
	delete(enums, document.AttrWidth)

	require.Len(t, got, len(enums)+1)

	for attr, values := range enums {
		prop, pok := got[attr].(map[string]any)
		require.True(t, pok, "attr %q is missing from the schema", attr)

		assert.Equal(t, _typeString, prop[_keyType])
		assert.Equal(t, values, prop[_keyEnum], "attr %q publishes the wrong values", attr)
		assert.NotEmpty(t, prop[_keyDescription], "attr %q has no description", attr)
	}

	// the sub-schema is reachable from the block argument itself, which
	// is what a client actually reads.
	attrs, ok := _blockSchema["properties"].(map[string]any)["attrs"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, got, attrs["properties"])
}
