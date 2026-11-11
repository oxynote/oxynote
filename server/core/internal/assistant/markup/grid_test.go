package markup

import (
	"encoding/json"
	"maps"
	"os"
	"strconv"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GridReadingOrder(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Blocks []document.Block
		Result []string
	}

	cc := map[string]tcase{
		"Empty grid": {
			Result: []string{},
		},
		"Legacy grid reads in document order": {
			Blocks: []document.Block{
				gridBlock("a", document.Attributes{document.AttrWidth: "wide"}),
				gridBlock("b", document.Attributes{document.AttrWidth: "compact"}),
				gridBlock("c", document.Attributes{document.AttrWidth: "standard"}),
			},
			Result: []string{"a", "b", "c"},
		},
		"Placed grid reads row by row, left to right": {
			Blocks: []document.Block{
				gridBlock("a", cells(0, 1, 12, 1)),
				gridBlock("b", cells(12, 0, 12, 1)),
				gridBlock("c", cells(0, 0, 12, 1)),
			},
			Result: []string{"c", "b", "a"},
		},
		"Block floating up passes a block beside it": {
			Blocks: []document.Block{
				gridBlock("a", cells(12, 1, 12, 1)),
				gridBlock("b", cells(0, 3, 12, 1)),
			},
			Result: []string{"b", "a"},
		},
		"Blocks on one cell read in document order": {
			Blocks: []document.Block{
				gridBlock("a", cells(0, 0, 12, 1)),
				gridBlock("b", cells(0, 0, 12, 1)),
			},
			Result: []string{"a", "b"},
		},
	}

	for _, g := range loadGridCases(t).Grids {
		cc[g.Name] = tcase{Blocks: g.blocks(), Result: indexUIDs(g.ReadingOrder)}
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := []string{}

			for _, b := range GridReadingOrder(c.Blocks) {
				uid, _ := b.UID()
				got = append(got, uid)
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_GridStackOrder(t *testing.T) {
	t.Parallel()

	// the wide layout reads c, a, b.
	wide := func(places ...any) []document.Block {
		blocks := []document.Block{
			gridBlock("a", cells(12, 0, 12, 1)),
			gridBlock("b", cells(0, 1, 12, 1)),
			gridBlock("c", cells(0, 0, 12, 2)),
		}

		for i, place := range places {
			if place != nil {
				blocks[i].Attrs[document.AttrStackPos] = place
			}
		}

		return blocks
	}

	type tcase struct {
		Blocks []document.Block
		Result []string
	}

	cc := map[string]tcase{
		"Empty grid":                          {Result: []string{}},
		"No block has a place":                {Blocks: wide(), Result: []string{"c", "a", "b"}},
		"Blocks sort by their places":         {Blocks: wide(0.0, 2.0, 1.0), Result: []string{"a", "c", "b"}},
		"Blocks on one place keep wide order": {Blocks: wide(0.0, 0.0, 0.0), Result: []string{"c", "a", "b"}},
		"Block without a place follows its wide predecessor": {
			Blocks: wide(1.0, nil, 0.0),
			Result: []string{"c", "a", "b"},
		},
		"Block without a place first in the wide layout comes first": {
			Blocks: wide(0.0, 1.0, nil),
			Result: []string{"c", "a", "b"},
		},
	}

	for _, g := range loadGridCases(t).Grids {
		cc[g.Name] = tcase{Blocks: g.blocks(), Result: indexUIDs(g.StackOrder)}
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, uidsOf(GridStackOrder(c.Blocks)))
		})
	}
}

func Test_IsGridPlace(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Key    string
		Result bool
	}{
		"Column":        {Key: document.AttrWidePosX, Result: true},
		"Row":           {Key: document.AttrWidePosY, Result: true},
		"Stacked place": {Key: document.AttrStackPos, Result: true},
		"Width":         {Key: document.AttrWidth},
		"Height":        {Key: document.AttrHeight},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, IsGridPlace(c.Key))
		})
	}
}

func Test_GridStackPos(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Attrs  document.Attributes
		Result int
		Placed bool
	}{
		"Place":          {Attrs: document.Attributes{document.AttrStackPos: 2.0}, Result: 2, Placed: true},
		"No place":       {Attrs: document.Attributes{}},
		"Negative place": {Attrs: document.Attributes{document.AttrStackPos: -1.0}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, placed := GridStackPos(document.Block{Attrs: c.Attrs})
			assert.Equal(t, c.Result, got)
			assert.Equal(t, c.Placed, placed)
		})
	}
}

func Test_readingIndexes(t *testing.T) {
	t.Parallel()

	blocks := []document.Block{gridBlock("a", cells(12, 0, 12, 1)), gridBlock("b", cells(0, 0, 12, 1))}

	assert.Equal(t, []int{1, 0}, readingIndexes(blocks))
}

func Test_pickBlocks(t *testing.T) {
	t.Parallel()

	blocks := []document.Block{gridBlock("a", document.Attributes{}), gridBlock("b", document.Attributes{})}

	assert.Equal(t, []string{"b", "a"}, uidsOf(pickBlocks(blocks, []int{1, 0})))
}

func Test_layoutGrid(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Attrs  []document.Attributes
		Result []gridRect
	}

	cc := map[string]tcase{
		"Empty grid": {
			Result: []gridRect{},
		},
		"Legacy grid flows each block after the one before it": {
			Attrs: []document.Attributes{
				{document.AttrWidth: "standard"},
				{document.AttrWidth: "wide"},
				{document.AttrWidth: "compact"},
				{document.AttrWidth: "compact"},
			},
			Result: []gridRect{{0, 0, 12, 1}, {0, 1, 24, 1}, {0, 2, 8, 1}, {8, 2, 8, 1}},
		},
		"Legacy grid leaves a gap a later block would fit": {
			Attrs: []document.Attributes{
				{document.AttrWidth: "compact"},
				{document.AttrWidth: "wide"},
				{document.AttrWidth: "compact"},
			},
			Result: []gridRect{{0, 0, 8, 1}, {0, 1, 24, 1}, {0, 2, 8, 1}},
		},
		"Tall block without cells takes two rows": {
			Attrs: []document.Attributes{
				{document.AttrWidth: 12.0, document.AttrHeight: 2.0},
				{document.AttrWidth: 12.0},
				{document.AttrWidth: 12.0},
			},
			Result: []gridRect{{0, 0, 12, 2}, {12, 0, 12, 1}, {12, 1, 12, 1}},
		},
		"Block without cells takes the first free ones beside placed blocks": {
			Attrs: []document.Attributes{
				{document.AttrWidth: "standard"},
				cells(12, 0, 12, 1),
			},
			Result: []gridRect{{0, 0, 12, 1}, {12, 0, 12, 1}},
		},
		"Overlapping block moves down past the one placed first": {
			Attrs: []document.Attributes{
				cells(4, 0, 12, 1),
				cells(0, 0, 12, 1),
			},
			Result: []gridRect{{0, 1, 12, 1}, {0, 0, 12, 1}},
		},
		"Block past the last column is pulled in before it moves down": {
			Attrs: []document.Attributes{
				cells(0, 0, 16, 1),
				cells(20, 0, 12, 1),
			},
			Result: []gridRect{{0, 0, 16, 1}, {0, 1, 12, 1}},
		},
		"Blocks float up": {
			Attrs: []document.Attributes{
				cells(0, 5, 12, 1),
				cells(12, 2000000000, 12, 2),
			},
			Result: []gridRect{{0, 0, 12, 1}, {12, 0, 12, 2}},
		},
		"Block floats up to the one above it": {
			Attrs: []document.Attributes{
				cells(0, 0, 24, 1),
				cells(6, 4, 12, 1),
			},
			Result: []gridRect{{0, 0, 24, 1}, {0, 1, 12, 1}},
		},
	}

	for _, g := range loadGridCases(t).Grids {
		cc[g.Name] = tcase{Attrs: g.Blocks, Result: g.Rects}
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			blocks := make([]document.Block, 0, len(c.Attrs))

			for _, attrs := range c.Attrs {
				blocks = append(blocks, document.Block{Type: document.BlockNodeMetricBlock, Attrs: attrs})
			}

			got := []gridRect{}

			for _, box := range layoutGrid(blocks) {
				got = append(got, rectOf(box))
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_gridItemOf(t *testing.T) {
	t.Parallel()

	sizes := loadGridCases(t).Sizes

	// the editor's grid engine names the same sizes.
	assert.Equal(t, sizes.Width, _gridItemSizes[document.AttrWidth])
	assert.Equal(t, sizes.Height, _gridItemSizes[document.AttrHeight])

	cc := map[string]struct {
		Attrs  document.Attributes
		Result gridBox
	}{
		"Legacy block takes the columns of its named width": {
			Attrs:  document.Attributes{document.AttrWidth: "compact"},
			Result: gridBox{w: 8, h: 1},
		},
		"Block without a size takes the default one": {
			Attrs:  document.Attributes{},
			Result: gridBox{w: sizes.Default.W, h: sizes.Default.H},
		},
		"Block with its place and size": {
			Attrs: document.Attributes{
				document.AttrWidePosX: 2.0,
				document.AttrWidePosY: 3.0,
				document.AttrWidth:    10.0,
				document.AttrHeight:   2.0,
			},
			Result: gridBox{placed: true, x: 2, y: 3, w: 10, h: 2},
		},
		"Size out of range is standard": {
			Attrs:  document.Attributes{document.AttrWidth: 25.0, document.AttrHeight: 25.0},
			Result: gridBox{w: 12, h: 1},
		},
		"Unknown width name and any height name are standard": {
			Attrs:  document.Attributes{document.AttrWidth: "huge", document.AttrHeight: "tall"},
			Result: gridBox{w: 12, h: 1},
		},
		"Fractions and negatives are no cells": {
			Attrs: document.Attributes{
				document.AttrWidePosX: 1.5,
				document.AttrWidePosY: -1.0,
				document.AttrWidth:    0.0,
				document.AttrHeight:   "2",
			},
			Result: gridBox{w: 12, h: 1},
		},
		"Column without a row is no place": {
			Attrs:  document.Attributes{document.AttrWidePosX: 0.0, document.AttrWidePosY: nil},
			Result: gridBox{w: 12, h: 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Result.id = 7
			assert.Equal(t, &c.Result, gridItemOf(7, c.Attrs))
		})
	}
}

func Test_gridSizeName(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Attrs  document.Attributes
		Key    string
		Result string
	}{
		"Width with a name":                      {Attrs: document.Attributes{document.AttrWidth: 8.0}, Key: document.AttrWidth, Result: "compact"},
		"Width without a name takes the nearest": {Attrs: document.Attributes{document.AttrWidth: 20.0}, Key: document.AttrWidth, Result: "wide"},
		"Tie goes to the wider name":             {Attrs: document.Attributes{document.AttrWidth: 10.0}, Key: document.AttrWidth, Result: "standard"},
		"Legacy width name":                      {Attrs: document.Attributes{document.AttrWidth: "compact"}, Key: document.AttrWidth, Result: "compact"},
		"Missing width":                          {Attrs: document.Attributes{}, Key: document.AttrWidth, Result: "standard"},
		"Height with a name":                     {Attrs: document.Attributes{document.AttrHeight: 2.0}, Key: document.AttrHeight, Result: "tall"},
		"Height past the tallest name":           {Attrs: document.Attributes{document.AttrHeight: 3.0}, Key: document.AttrHeight, Result: "standard"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, gridSizeName(c.Attrs, c.Key))
		})
	}
}

func Test_sizeNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"compact", "standard", "wide"}, sizeNames(map[string]int{"wide": 24, "compact": 8, "standard": 12}))
}

func Test_wholeAtLeast(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Floor  int
		Result int
		Count  bool
	}{
		"Missing":            {Value: nil},
		"String":             {Value: "3"},
		"Float":              {Value: 3.0, Result: 3, Count: true},
		"Int":                {Value: 3, Result: 3, Count: true},
		"Int64":              {Value: int64(3), Result: 3, Count: true},
		"Fraction":           {Value: 2.5},
		"Below the floor":    {Value: 0.0, Floor: 1},
		"At the floor":       {Value: 1.0, Floor: 1, Result: 1, Count: true},
		"Huge value is kept": {Value: 1e300, Result: 2147483647, Count: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, ok := wholeAtLeast(document.Attributes{"n": c.Value}, "n", c.Floor)
			assert.Equal(t, c.Result, got)
			assert.Equal(t, c.Count, ok)
		})
	}
}

func Test_compactGrid(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rects  []gridRect
		Result []gridRect
	}{
		"Block floats up before it slides left": {
			Rects:  []gridRect{{0, 0, 8, 1}, {16, 1, 8, 1}},
			Result: []gridRect{{0, 0, 8, 1}, {8, 0, 8, 1}},
		},
		"Block that slides left lets the block under it float up": {
			Rects:  []gridRect{{0, 0, 8, 1}, {16, 0, 8, 1}, {16, 1, 8, 1}, {0, 1, 16, 1}},
			Result: []gridRect{{0, 0, 8, 1}, {8, 0, 8, 1}, {16, 0, 8, 1}, {0, 1, 16, 1}},
		},
		"Block that floats up slides left again": {
			Rects:  []gridRect{{0, 0, 8, 1}, {12, 0, 4, 1}, {14, 1, 8, 1}, {0, 1, 14, 1}},
			Result: []gridRect{{0, 0, 8, 1}, {8, 0, 4, 1}, {12, 0, 8, 1}, {0, 1, 14, 1}},
		},
		"Compact grid stays": {
			Rects:  []gridRect{{0, 0, 12, 2}, {12, 0, 12, 1}, {12, 1, 8, 1}},
			Result: []gridRect{{0, 0, 12, 2}, {12, 0, 12, 1}, {12, 1, 8, 1}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			boxes := boxesOf(c.Rects)

			compactGrid(boxes)

			assert.Equal(t, c.Result, rectsOf(boxes))
		})
	}
}

func Test_floatUp(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rects  []gridRect
		Result []gridRect
		Moved  bool
	}{
		"Block floats up to the first row": {
			Rects:  []gridRect{{0, 3, 12, 1}},
			Result: []gridRect{{0, 0, 12, 1}},
			Moved:  true,
		},
		"Block rises to the bottom of the block above it": {
			Rects:  []gridRect{{0, 0, 12, 2}, {6, 5, 12, 1}},
			Result: []gridRect{{0, 0, 12, 2}, {6, 2, 12, 1}},
			Moved:  true,
		},
		"Block rises past a block beside it": {
			Rects:  []gridRect{{0, 0, 12, 4}, {12, 3, 12, 1}},
			Result: []gridRect{{0, 0, 12, 4}, {12, 0, 12, 1}},
			Moved:  true,
		},
		"Block that cannot rise stays": {
			Rects:  []gridRect{{0, 0, 12, 1}, {6, 1, 12, 1}},
			Result: []gridRect{{0, 0, 12, 1}, {6, 1, 12, 1}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			boxes := boxesOf(c.Rects)

			moved := floatUp(boxes)

			assert.Equal(t, c.Result, rectsOf(boxes))
			assert.Equal(t, c.Moved, moved)
		})
	}
}

func Test_slideLeft(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rects  []gridRect
		Result []gridRect
		Moved  bool
	}{
		"Block slides to the first column": {
			Rects:  []gridRect{{6, 0, 12, 1}},
			Result: []gridRect{{0, 0, 12, 1}},
			Moved:  true,
		},
		"Block slides to the block before it in its row": {
			Rects:  []gridRect{{0, 0, 8, 1}, {16, 0, 8, 1}},
			Result: []gridRect{{0, 0, 8, 1}, {8, 0, 8, 1}},
			Moved:  true,
		},
		"Block passes a block in another row": {
			Rects:  []gridRect{{0, 0, 12, 1}, {16, 1, 8, 1}},
			Result: []gridRect{{0, 0, 12, 1}, {0, 1, 8, 1}},
			Moved:  true,
		},
		"Tall block stops at a block in its second row": {
			Rects:  []gridRect{{0, 1, 12, 1}, {16, 0, 8, 2}},
			Result: []gridRect{{0, 1, 12, 1}, {12, 0, 8, 2}},
			Moved:  true,
		},
		"Blocks that touch stay": {
			Rects:  []gridRect{{0, 0, 12, 1}, {12, 0, 12, 1}},
			Result: []gridRect{{0, 0, 12, 1}, {12, 0, 12, 1}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			boxes := boxesOf(c.Rects)

			moved := slideLeft(boxes)

			assert.Equal(t, c.Result, rectsOf(boxes))
			assert.Equal(t, c.Moved, moved)
		})
	}
}

func Test_moveDownPast(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rect   gridRect
		Others []gridRect
		Result gridRect
	}{
		"Box covering nothing stays": {
			Rect:   gridRect{0, 0, 12, 1},
			Others: []gridRect{{12, 0, 12, 1}},
			Result: gridRect{0, 0, 12, 1},
		},
		"Box moves below the block it covers": {
			Rect:   gridRect{0, 0, 12, 1},
			Others: []gridRect{{6, 0, 12, 2}},
			Result: gridRect{0, 2, 12, 1},
		},
		"Box moves past each block it covers in turn": {
			Rect:   gridRect{0, 0, 12, 1},
			Others: []gridRect{{0, 0, 24, 1}, {6, 1, 12, 2}},
			Result: gridRect{0, 3, 12, 1},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			box := c.Rect.box(0)
			others := make([]*gridBox, 0, len(c.Others))

			for i, r := range c.Others {
				others = append(others, r.box(i+1))
			}

			moveDownPast(box, others)
			assert.Equal(t, c.Result, rectOf(box))
		})
	}
}

func Test_gridBox_overlaps(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rect   gridRect
		Other  gridRect
		Result bool
	}{
		"Boxes sharing a cell":    {Rect: gridRect{0, 0, 12, 2}, Other: gridRect{11, 1, 12, 1}, Result: true},
		"Boxes side by side":      {Rect: gridRect{0, 0, 12, 1}, Other: gridRect{12, 0, 12, 1}},
		"Box above another":       {Rect: gridRect{0, 0, 12, 1}, Other: gridRect{0, 1, 12, 1}},
		"Box inside a larger one": {Rect: gridRect{4, 1, 4, 1}, Other: gridRect{0, 0, 24, 3}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, c.Rect.box(0).overlaps(c.Other.box(1)))
		})
	}
}

func Test_take(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Taken  map[int]uint32
		Rect   gridRect
		Result map[int]uint32
	}{
		"Box marks each row it spans": {
			Taken:  map[int]uint32{},
			Rect:   gridRect{2, 1, 3, 2},
			Result: map[int]uint32{1: 0b11100, 2: 0b11100},
		},
		"Box keeps the cells already taken": {
			Taken:  map[int]uint32{0: 0b1},
			Rect:   gridRect{4, 0, 2, 1},
			Result: map[int]uint32{0: 0b110001},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			take(c.Taken, c.Rect.box(0))
			assert.Equal(t, c.Result, c.Taken)
		})
	}
}

func Test_fits(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Taken  map[int]uint32
		Rect   gridRect
		Result bool
	}{
		"Box on free cells":          {Taken: map[int]uint32{0: 0xfff}, Rect: gridRect{12, 0, 12, 1}, Result: true},
		"Box covering a taken cell":  {Taken: map[int]uint32{1: 1 << 5}, Rect: gridRect{0, 0, 12, 2}},
		"Box past the right edge":    {Taken: map[int]uint32{}, Rect: gridRect{20, 0, 8, 1}},
		"Box below the taken cells":  {Taken: map[int]uint32{0: 0xffffff}, Rect: gridRect{0, 1, 24, 1}, Result: true},
		"Box on the last free cells": {Taken: map[int]uint32{0: 0x7fffff}, Rect: gridRect{23, 0, 1, 1}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, fits(c.Taken, c.Rect.box(0)))
		})
	}
}

func Test_gridBox_columnBits(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Rect   gridRect
		Result uint32
	}{
		"First column":            {Rect: gridRect{0, 0, 1, 1}, Result: 0b1},
		"Columns from the middle": {Rect: gridRect{4, 2, 3, 1}, Result: 0b1110000},
		"Every column":            {Rect: gridRect{0, 0, 24, 1}, Result: 0xffffff},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, c.Rect.box(0).columnBits())
		})
	}
}

// placedTestGrid returns a grid of three copies of the fixture's metric.
// Each copy has its own cells. People see them in the order pc, pa, pb,
// but the document holds them in another order.
func placedTestGrid(t *testing.T) document.Block {
	t.Helper()

	metric, ok := loadFixture(t, "all_blocks").FindByUID("mb1")
	require.True(t, ok)

	placed := func(uid string, attrs document.Attributes) document.Block {
		b := metric
		b.Attrs = maps.Clone(metric.Attrs)
		maps.Copy(b.Attrs, attrs)
		b.Attrs[document.AttrUID] = uid

		return b
	}

	return document.Block{
		Type:  document.BlockNodeBlockGrid,
		Attrs: document.Attributes{document.AttrUID: "pg"},
		Content: []document.Block{
			placed("pa", cells(12, 0, 12, 1)),
			placed("pb", cells(0, 1, 24, 1)),
			placed("pc", cells(0, 0, 12, 1)),
		},
	}
}

// gridBlock returns a metric block with the uid and attributes given.
func gridBlock(uid string, attrs document.Attributes) document.Block {
	attrs[document.AttrUID] = uid

	return document.Block{Type: document.BlockNodeMetricBlock, Attrs: attrs}
}

// cells returns the attributes that place and size a grid block, as JSON
// numbers.
func cells(x, y, w, h int) document.Attributes {
	return document.Attributes{
		document.AttrWidePosX: float64(x),
		document.AttrWidePosY: float64(y),
		document.AttrWidth:    float64(w),
		document.AttrHeight:   float64(h),
	}
}

// gridCases are the grid cases in packages/common/testdata/grid-cases.json.
// The editor's grid engine runs them too, so both lay a grid out the same
// way.
type gridCases struct {
	Sizes struct {
		Width   map[string]int `json:"width"`
		Height  map[string]int `json:"height"`
		Default struct {
			W int `json:"w"`
			H int `json:"h"`
		} `json:"default"`
	} `json:"sizes"`
	Grids []gridCase `json:"grids"`
}

// gridCase is one grid of the shared cases. It holds its blocks'
// attributes and the cells and orders they get.
type gridCase struct {
	Name         string                `json:"name"`
	Blocks       []document.Attributes `json:"blocks"`
	Rects        []gridRect            `json:"rects"`
	ReadingOrder []int                 `json:"readingOrder"`
	StackOrder   []int                 `json:"stackOrder"`
}

// gridRect is a grid block's cells: its first column and row, and the
// columns and rows it spans.
type gridRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// loadGridCases reads the shared grid cases.
func loadGridCases(t *testing.T) gridCases {
	t.Helper()

	data, err := os.ReadFile("../../../../../packages/common/testdata/grid-cases.json")
	require.NoError(t, err)

	var cases gridCases
	require.NoError(t, json.Unmarshal(data, &cases))

	return cases
}

// blocks returns the grid's blocks, each with its index as its uid.
func (g gridCase) blocks() []document.Block {
	blocks := make([]document.Block, 0, len(g.Blocks))

	for i, attrs := range g.Blocks {
		blocks = append(blocks, gridBlock(strconv.Itoa(i), attrs))
	}

	return blocks
}

// indexUIDs returns the uid that gridCase.blocks gives to each block at
// the indexes.
func indexUIDs(indexes []int) []string {
	uids := make([]string, 0, len(indexes))

	for _, i := range indexes {
		uids = append(uids, strconv.Itoa(i))
	}

	return uids
}

// box returns the rect as the box of the block at index id.
func (r gridRect) box(id int) *gridBox {
	return &gridBox{id: id, placed: true, x: r.X, y: r.Y, w: r.W, h: r.H}
}

// rectOf returns a box's cells.
func rectOf(box *gridBox) gridRect {
	return gridRect{box.x, box.y, box.w, box.h}
}

// boxesOf returns the rects as boxes, each with its index as its id.
func boxesOf(rects []gridRect) []*gridBox {
	boxes := make([]*gridBox, 0, len(rects))

	for i, r := range rects {
		boxes = append(boxes, r.box(i))
	}

	return boxes
}

// rectsOf returns the cells of each box.
func rectsOf(boxes []*gridBox) []gridRect {
	rects := make([]gridRect, 0, len(boxes))

	for _, box := range boxes {
		rects = append(rects, rectOf(box))
	}

	return rects
}
