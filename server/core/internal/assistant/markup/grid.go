package markup

import (
	"cmp"
	"maps"
	"math"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _gridColumns is how many columns the widest grid has.
const _gridColumns = 24

// _gridMaxRows is the most rows a grid block spans.
const _gridMaxRows = 24

// _gridDefaultSize names the width and height a block takes when it
// stores no size the grid knows.
const _gridDefaultSize = "standard"

// gridBox holds a grid block's cells while the layout places it.
// Columns and rows count from zero.
type gridBox struct {
	// id is the block's index among the grid's children.
	id int

	// placed indicates the block stores its column and row.
	placed bool

	// x is the first column.
	x int

	// y is the first row.
	y int

	// w is how many columns it spans.
	w int

	// h is how many rows it spans.
	h int
}

// GridReadingOrder returns a grid's blocks in the order its wide layout
// shows them: row by row, left to right.
func GridReadingOrder(blocks []document.Block) []document.Block {
	return pickBlocks(blocks, readingIndexes(blocks))
}

// GridStackOrder returns a grid's blocks in the order its stacked layout
// shows them. Blocks sort by their place in that layout. Blocks with the
// same place keep the wide layout's reading order. A block without a
// place goes right after the block it follows in the wide layout. It
// mirrors stackOrder in packages/common/src/grid.ts. A change there has
// to be made here, or the assistant reads a grid in another order than
// people see it.
func GridStackOrder(blocks []document.Block) []document.Block {
	wide := readingIndexes(blocks)
	order := slices.DeleteFunc(slices.Clone(wide), func(i int) bool {
		_, placed := GridStackPos(blocks[i])

		return !placed
	})

	slices.SortStableFunc(order, func(a, b int) int {
		pa, _ := GridStackPos(blocks[a])
		pb, _ := GridStackPos(blocks[b])

		return pa - pb
	})

	for at, i := range wide {
		if _, placed := GridStackPos(blocks[i]); placed {
			continue
		}

		next := 0
		if at > 0 {
			next = slices.Index(order, wide[at-1]) + 1
		}

		order = slices.Insert(order, next, i)
	}

	return pickBlocks(blocks, order)
}

// IsGridPlace reports whether the attribute says where a grid block
// sits.
func IsGridPlace(key string) bool {
	return slices.Contains(_gridPlaceAttrs, key)
}

// GridStackPos returns a grid block's place in the stacked layout, and
// whether it has one. Only a whole number from zero up is a place.
func GridStackPos(b document.Block) (int, bool) {
	return wholeAtLeast(b.Attrs, document.AttrStackPos, 0)
}

// readingIndexes returns the indexes of a grid's blocks in the wide
// layout's reading order.
func readingIndexes(blocks []document.Block) []int {
	boxes := layoutGrid(blocks)

	slices.SortStableFunc(boxes, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x, a.id-b.id)
	})

	indexes := make([]int, 0, len(boxes))

	for _, box := range boxes {
		indexes = append(indexes, box.id)
	}

	return indexes
}

// pickBlocks returns the blocks at the indexes, in the indexes' order.
func pickBlocks(blocks []document.Block, indexes []int) []document.Block {
	picked := make([]document.Block, 0, len(indexes))

	for _, i := range indexes {
		picked = append(picked, blocks[i])
	}

	return picked
}

// layoutGrid places a grid's blocks the way the editor does. A block
// with a stored place keeps it, but it moves down until it covers no
// block placed before it. A block without one takes the first free
// cells, row by row. When no block has a place, the search starts after
// the block before it. Then every block floats up as far as it can. It
// mirrors layoutGrid in packages/common/src/grid.ts. A change there has
// to be made here, or the assistant reads a grid in another order than
// people see it.
func layoutGrid(blocks []document.Block) []*gridBox {
	items := make([]*gridBox, 0, len(blocks))

	for i, b := range blocks {
		items = append(items, gridItemOf(i, b.Attrs))
	}

	placed := slices.DeleteFunc(slices.Clone(items), func(box *gridBox) bool { return !box.placed })

	slices.SortStableFunc(placed, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x)
	})

	var boxes []*gridBox

	for _, box := range placed {
		box.x = min(box.x, _gridColumns-box.w)
		moveDownPast(box, boxes)
		boxes = append(boxes, box)
	}

	dense := len(boxes) > 0
	row, column := 0, 0

	for _, box := range items {
		if box.placed {
			continue
		}

		box.x, box.y = column, row

		for box.x+box.w > _gridColumns || slices.ContainsFunc(boxes, box.overlaps) {
			box.x++

			if box.x+box.w > _gridColumns {
				box.x = 0
				box.y++
			}
		}

		boxes = append(boxes, box)

		if !dense {
			row, column = box.y, box.x+box.w
		}
	}

	compactGrid(boxes)

	return items
}

// gridItemOf reads a grid block's place and size from its attributes.
// widePosX and widePosY hold its column and row in the wide layout.
// width and height hold the columns and rows it spans. An older block
// stores only a width, and that width is a size name. It mirrors
// gridItemOf in packages/common/src/grid.ts.
func gridItemOf(id int, attrs document.Attributes) *gridBox {
	x, hasX := wholeAtLeast(attrs, document.AttrWidePosX, 0)
	y, hasY := wholeAtLeast(attrs, document.AttrWidePosY, 0)
	w, hasW := wholeAtLeast(attrs, document.AttrWidth, 1)
	h, hasH := wholeAtLeast(attrs, document.AttrHeight, 1)

	widths := _gridItemSizes[document.AttrWidth]

	if !hasW || w > _gridColumns {
		w = cmp.Or(widths[attrs.Get(document.AttrWidth).String()], widths[_gridDefaultSize])
	}

	if !hasH || h > _gridMaxRows {
		h = _gridItemSizes[document.AttrHeight][_gridDefaultSize]
	}

	return &gridBox{id: id, placed: hasX && hasY, x: x, y: y, w: w, h: h}
}

// gridSizeName returns the size name the model sees for a grid block's
// width or height, as key says. It is the name of the block's span, or
// the nearest name when the span has none. A tie goes to the larger size.
func gridSizeName(attrs document.Attributes, key string) string {
	box := gridItemOf(0, attrs)
	span := box.w

	if key == document.AttrHeight {
		span = box.h
	}

	sizes := _gridItemSizes[key]
	distance := func(name string) int { return max(sizes[name]-span, span-sizes[name]) }

	return slices.MinFunc(sizeNames(sizes), func(a, b string) int {
		return cmp.Or(cmp.Compare(distance(a), distance(b)), cmp.Compare(sizes[b], sizes[a]))
	})
}

// sizeNames returns the names of sizes, smallest first.
func sizeNames(sizes map[string]int) []string {
	names := slices.Collect(maps.Keys(sizes))

	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(sizes[a], sizes[b]) })

	return names
}

// wholeAtLeast returns the named attribute when it is a whole number no
// smaller than floor. JSON numbers arrive as float64, so it refuses a
// fraction such as 2.5.
func wholeAtLeast(attrs document.Attributes, name string, floor int) (int, bool) {
	var v float64

	switch n := attrs[name].(type) {
	case int:
		v = float64(n)
	case int64:
		v = float64(n)
	case float64:
		v = n
	default:
		return 0, false
	}

	if v != math.Trunc(v) || v < float64(floor) {
		return 0, false
	}

	return int(min(v, math.MaxInt32)), true
}

// compactGrid floats every block up as far as it can, top to bottom. A
// block rises to the bottom of the lowest block above it in its
// columns, or to the first row.
func compactGrid(boxes []*gridBox) {
	sorted := slices.Clone(boxes)

	slices.SortStableFunc(sorted, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x, a.id-b.id)
	})

	settled := make([]*gridBox, 0, len(sorted))

	for _, box := range sorted {
		moveDownPast(box, settled)

		top := 0

		for _, other := range settled {
			if other.x < box.x+box.w && box.x < other.x+other.w && other.y+other.h <= box.y {
				top = max(top, other.y+other.h)
			}
		}

		box.y = top
		settled = append(settled, box)
	}
}

// moveDownPast moves a box down until it covers none of the others.
func moveDownPast(box *gridBox, others []*gridBox) {
	for {
		i := slices.IndexFunc(others, box.overlaps)
		if i < 0 {
			return
		}

		box.y = others[i].y + others[i].h
	}
}

// overlaps reports whether two boxes share a cell.
func (b *gridBox) overlaps(other *gridBox) bool {
	return b.x < other.x+other.w && other.x < b.x+b.w && b.y < other.y+other.h && other.y < b.y+b.h
}
