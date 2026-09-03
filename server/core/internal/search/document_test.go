package search

import (
	"encoding/json"
	"testing"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BlocksDiff(t *testing.T) {
	t.Parallel()

	// empty inputs
	empty := BlocksDiff(nil, nil)

	assert.Empty(t, empty.Added)
	assert.Empty(t, empty.Updated)
	assert.Empty(t, empty.Removed)

	// added, updated and removed blocks
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

// stubBlocksDifference builds a difference carrying one block of each kind.
func stubBlocksDifference() BlocksDifference {
	return BlocksDifference{
		Added:   []Block{{ID: "a", Text: "added"}},
		Updated: []Block{{ID: "u", Text: "updated"}},
		Removed: []Block{{ID: "r", Text: "removed"}},
	}
}

func Test_BlocksDifference_Value(t *testing.T) {
	t.Parallel()

	orig := stubBlocksDifference()

	val, err := orig.Value()
	require.NoError(t, err)

	data, ok := val.([]byte)
	require.True(t, ok)

	exp, err := json.Marshal(orig)
	require.NoError(t, err)
	assert.JSONEq(t, string(exp), string(data))
}

func Test_BlocksDifference_Scan(t *testing.T) {
	t.Parallel()

	orig := stubBlocksDifference()

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

func Test_Scope_Block(t *testing.T) {
	t.Parallel()

	scope := Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "draft",
		BranchDefault:  false,
	}

	assert.Equal(t, Block{
		ID:             scope.BranchID.String() + "-p1",
		OrganizationID: "org-1",
		DocumentID:     scope.DocumentID,
		BranchID:       scope.BranchID,
		BranchName:     "draft",
		BranchDefault:  false,
		Type:           "paragraph",
		Text:           "hello",
	}, scope.Block("p1", "paragraph", "hello"))
}
