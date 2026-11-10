package tools

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_gridOps(t *testing.T) {
	t.Parallel()

	// the stored grid shows a, b, c in one row.
	stored := testGrid(
		testGridBlock("a", document.Attributes{document.AttrWidePosX: 0.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
		testGridBlock("b", document.Attributes{document.AttrWidePosX: 8.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
		testGridBlock("c", document.Attributes{document.AttrWidePosX: 16.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
	)
	block := func(uid string) document.Block {
		b, _ := stored.FindByUID(uid)

		return b
	}
	added := testGridBlock("n", document.Attributes{})

	cc := map[string]struct {
		Built  document.Block
		Result []edit.Operation
	}{
		"Grid as it was": {
			Built: stored,
		},
		"Block removed": {
			Built:  testGrid(block("a"), block("c")),
			Result: []edit.Operation{edit.Delete("b")},
		},
		"Block changed": {
			Built: testGrid(block("a"), testGridBlock("b", document.Attributes{
				document.AttrWidePosX: 8.0,
				document.AttrWidePosY: 0.0,
				document.AttrWidth:    8.0,
				document.AttrTitle:    "B",
			}), block("c")),
			Result: []edit.Operation{edit.UpdateAttrs("b", map[string]any{document.AttrTitle: "B"})},
		},
		"Block moved to the end": {
			Built:  testGrid(block("b"), block("c"), block("a")),
			Result: []edit.Operation{edit.Move("a", "after", "c")},
		},
		"Block moved to the start": {
			Built:  testGrid(block("c"), block("a"), block("b")),
			Result: []edit.Operation{edit.Move("c", "before", "a")},
		},
		"Block added between two": {
			Built:  testGrid(block("a"), added, block("b"), block("c")),
			Result: []edit.Operation{edit.Insert("after", "a", added)},
		},
		"Stacked order changed": {
			Built: testGrid(
				testGridBlock("a", document.Attributes{document.AttrWidePosX: 0.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0, document.AttrStackPos: 1}),
				block("b"),
				testGridBlock("c", document.Attributes{document.AttrWidePosX: 16.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0, document.AttrStackPos: 0}),
			),
			Result: []edit.Operation{edit.SetStackOrder("g", []string{"c", "a"})},
		},
		"No block kept": {
			Built:  testGrid(added),
			Result: []edit.Operation{edit.Replace("g", testGrid(added))},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, gridOps(stored, c.Built))
		})
	}
}

func Test_stayingUIDs(t *testing.T) {
	t.Parallel()

	stored := testGrid(
		testGridBlock("a", document.Attributes{document.AttrWidePosX: 0.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
		testGridBlock("b", document.Attributes{document.AttrWidePosX: 8.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
		testGridBlock("c", document.Attributes{document.AttrWidePosX: 16.0, document.AttrWidePosY: 0.0, document.AttrWidth: 8.0}),
	)

	cc := map[string]struct {
		Built  []string
		Result map[string]bool
	}{
		"Same order":       {Built: []string{"a", "b", "c"}, Result: map[string]bool{"a": true, "b": true, "c": true}},
		"One block moved":  {Built: []string{"c", "a", "b"}, Result: map[string]bool{"a": true, "b": true}},
		"New block added":  {Built: []string{"a", "n", "c"}, Result: map[string]bool{"a": true, "c": true}},
		"No block kept":    {Built: []string{"n"}, Result: map[string]bool{}},
		"Every block gone": {Result: map[string]bool{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var blocks []document.Block

			for _, uid := range c.Built {
				blocks = append(blocks, testGridBlock(uid, document.Attributes{}))
			}

			assert.Equal(t, c.Result, stayingUIDs(stored, testGrid(blocks...)))
		})
	}
}

func Test_changedAttrs(t *testing.T) {
	t.Parallel()

	stored := testGridBlock("a", document.Attributes{
		document.AttrTitle:        "A",
		document.AttrWidth:        "standard",
		document.AttrWidePosX:     0.0,
		document.AttrStackPos:     1.0,
		document.AttrDataSourceID: nil,
	})
	built := testGridBlock("a", document.Attributes{
		document.AttrTitle:    "B",
		document.AttrWidePosX: 12.0,
		document.AttrStackPos: 0,
	})

	assert.Equal(t, map[string]any{document.AttrTitle: "B", document.AttrWidth: nil}, changedAttrs(stored, built))
}

func Test_stackOrderChange(t *testing.T) {
	t.Parallel()

	stored := testGrid(
		testGridBlock("a", document.Attributes{document.AttrStackPos: 3.0}),
		testGridBlock("b", document.Attributes{}),
		testGridBlock("c", document.Attributes{document.AttrStackPos: -1.0}),
		testGridBlock("d", document.Attributes{document.AttrStackPos: 1.5}),
	)

	cc := map[string]struct {
		Built   document.Block
		Result  []string
		Changed bool
	}{
		"Same places": {
			Built:  testGrid(testGridBlock("a", document.Attributes{document.AttrStackPos: 3}), testGridBlock("b", document.Attributes{})),
			Result: []string{"a"},
		},
		"Malformed places are no places": {
			Built: testGrid(
				testGridBlock("a", document.Attributes{document.AttrStackPos: 3.0}),
				testGridBlock("b", document.Attributes{}),
				testGridBlock("c", document.Attributes{document.AttrStackPos: -1.0}),
				testGridBlock("d", document.Attributes{document.AttrStackPos: 1.5}),
			),
			Result: []string{"a"},
		},
		"Same order numbered anew": {
			Built:  testGrid(testGridBlock("a", document.Attributes{document.AttrStackPos: 0}), testGridBlock("b", document.Attributes{document.AttrStackPos: 1})),
			Result: []string{"a", "b"},
		},
		"Another order": {
			Built:   testGrid(testGridBlock("a", document.Attributes{document.AttrStackPos: 1}), testGridBlock("b", document.Attributes{document.AttrStackPos: 0})),
			Result:  []string{"b", "a"},
			Changed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			order, changed := stackOrderChange(stored, c.Built)
			assert.Equal(t, c.Result, order)
			assert.Equal(t, c.Changed, changed)
		})
	}
}

func Test_isGrid(t *testing.T) {
	t.Parallel()

	assert.True(t, isGrid(document.Block{Type: document.BlockNodeBlockGrid}))
	assert.True(t, isGrid(document.Block{Type: document.BlockNodeMetricGrid}))
	assert.False(t, isGrid(document.Block{Type: document.BlockNodeMetricBlock}))
}

func Test_sameUID(t *testing.T) {
	t.Parallel()

	assert.True(t, sameUID(testGridBlock("a", document.Attributes{}), testGridBlock("a", document.Attributes{})))
	assert.False(t, sameUID(testGridBlock("a", document.Attributes{}), testGridBlock("b", document.Attributes{})))
}

func Test_hasKey(t *testing.T) {
	t.Parallel()

	assert.True(t, hasKey(map[string]int{"a": 0}, "a"))
	assert.False(t, hasKey(map[string]int{"a": 0}, "b"))
}

// testGrid is a grid with uid g holding blocks.
func testGrid(blocks ...document.Block) document.Block {
	return document.Block{Type: document.BlockNodeBlockGrid, Attrs: document.Attributes{document.AttrUID: "g"}, Content: blocks}
}

// testGridBlock is a metric block with uid and attrs.
func testGridBlock(uid string, attrs document.Attributes) document.Block {
	attrs[document.AttrUID] = uid

	return document.Block{Type: document.BlockNodeMetricBlock, Attrs: attrs}
}
