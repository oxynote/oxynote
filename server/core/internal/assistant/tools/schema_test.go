package tools

import (
	"slices"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_blockSchema(t *testing.T) {
	t.Parallel()

	variants, ok := _blockSchema["anyOf"].([]map[string]any)
	require.True(t, ok)

	// one variant per canonical type in documentation order, less file,
	// which the assistant never writes.
	want := slices.DeleteFunc(block.Types(), func(t string) bool { return t == string(block.BlockFile) })
	require.Len(t, variants, len(want))

	for i, v := range variants {
		tp := want[i]

		assert.Equal(t, tp, v["title"])
		assert.Equal(t, false, v["additionalProperties"])

		props, pok := v["properties"].(map[string]any)
		require.True(t, pok, tp)
		assert.Equal(t, map[string]any{"const": tp}, props["type"], tp)
		assert.Contains(t, props, "uid", tp)

		// everything required is a field the variant publishes.
		required, rok := v["required"].([]string)
		require.True(t, rok, tp)
		assert.Equal(t, "type", required[0], tp)

		for _, key := range required {
			assert.Contains(t, props, key, tp)
		}
	}
}

func Test_blockVariant(t *testing.T) {
	t.Parallel()

	_, ok := blockVariant(block.BlockFile)
	assert.False(t, ok)

	// heading level is the one non-metric enum, and the only integer
	// one: a client sending "2" rather than 2 is sending the wrong type.
	heading, ok := blockVariant(block.BlockHeading)
	require.True(t, ok)
	assert.Equal(t, []string{"type", "attrs"}, heading["required"])

	attrs := heading["properties"].(map[string]any)["attrs"].(map[string]any)
	assert.Equal(t, []string{document.AttrLevel}, attrs["required"])

	level := attrs["properties"].(map[string]any)[document.AttrLevel].(map[string]any)
	assert.Equal(t, "integer", level["type"])
	assert.Equal(t, []int{1, 2, 3}, level["enum"])

	// the atoms that point somewhere require their src.
	for _, tp := range []block.Type{block.BlockImage, block.BlockFigma} {
		v, vok := blockVariant(tp)
		require.True(t, vok)
		assert.Equal(t, []string{"type", "attrs"}, v["required"], tp)
		assert.Equal(t, []string{document.AttrSrc}, v["properties"].(map[string]any)["attrs"].(map[string]any)["required"], tp)
	}

	// the metric enums are reachable from the metric variant, which is
	// what a client actually reads.
	metric, ok := blockVariant(block.BlockMetric)
	require.True(t, ok)
	assert.Equal(t, metricAttrProps(), metric["properties"].(map[string]any)["attrs"].(map[string]any)["properties"])
}

func Test_metricAttrProps(t *testing.T) {
	t.Parallel()

	got := metricAttrProps()

	// every metric enum is published, width included: the metric attrs
	// are their own schema, so the name no longer has to mean the same
	// thing as an image's pixel width.
	enums := block.MetricEnums()
	require.Len(t, got, len(enums))

	for attr, values := range enums {
		prop, pok := got[attr].(map[string]any)
		require.True(t, pok, "attr %q is missing from the schema", attr)

		assert.Equal(t, "string", prop["type"])
		assert.Equal(t, values, prop["enum"], "attr %q publishes the wrong values", attr)
		assert.NotEmpty(t, prop["description"], "attr %q has no description", attr)
	}
}
