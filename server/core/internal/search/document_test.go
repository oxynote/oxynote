package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BlocksDiff(t *testing.T) {
	t.Parallel()

	kept := Block{ID: "kept", Text: "same"}
	changed := Block{ID: "changed", Text: "old"}
	changedNew := Block{ID: "changed", Text: "new"}
	removed := Block{ID: "removed", Text: "gone"}
	added := Block{ID: "added", Text: "fresh"}

	diff := BlocksDiff(
		map[string]Block{"kept": kept, "changed": changed, "removed": removed},
		map[string]Block{"kept": kept, "changed": changedNew, "added": added},
	)

	assert.Equal(t, []Block{added}, diff.Added)
	assert.Equal(t, []Block{changedNew}, diff.Updated)
	assert.Equal(t, []Block{removed}, diff.Removed)
}

func Test_BlocksDiff_Empty(t *testing.T) {
	t.Parallel()

	diff := BlocksDiff(nil, nil)

	assert.Empty(t, diff.Added)
	assert.Empty(t, diff.Updated)
	assert.Empty(t, diff.Removed)
}

func Test_BlocksDifference_Value_Scan(t *testing.T) {
	t.Parallel()

	orig := BlocksDifference{
		Added:   []Block{{ID: "a", Text: "added"}},
		Updated: []Block{{ID: "u", Text: "updated"}},
		Removed: []Block{{ID: "r", Text: "removed"}},
	}

	val, err := orig.Value()
	require.NoError(t, err)

	var decoded BlocksDifference

	require.NoError(t, decoded.Scan(val))
	assert.Equal(t, orig, decoded)

	require.NoError(t, decoded.Scan(nil))
	assert.Equal(t, orig, decoded, "nil scan should leave the value untouched")

	assert.Error(t, decoded.Scan("not bytes"))
	assert.Error(t, decoded.Scan([]byte(`{not json`)))
}
