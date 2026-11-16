package markup

import (
	"cmp"
	"maps"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

const (
	// _gridColumns is how many columns the widest grid has.
	_gridColumns = 24

	// _gridDefaultSize names the width and height a block takes when it
	// stores no size the grid knows.
	_gridDefaultSize = "standard"
)

// _gridMaxRows is the most rows a grid block spans: the rows of the
// tallest named height. A block that stores more gets the default height.
var _gridMaxRows = slices.Max(slices.Collect(maps.Values(_gridItemSizes[document.AttrHeight])))

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

	// stack is the block's place in the stacked layout, once
	// setStackPlaces has set it.
	stack int
}

// newGridBox reads a grid block's place and size from its attributes, as
// the editor reads them. widePosX and widePosY hold its column and row in
// the wide layout. width and height hold the columns and rows it spans.
// An older block stores only a width, and that width is a size name.
func newGridBox(id int, attrs document.Attributes) *gridBox {
	x, hasX := attrs.Get(document.AttrWidePosX).WholeAtLeast(0)
	y, hasY := attrs.Get(document.AttrWidePosY).WholeAtLeast(0)
	w, hasW := attrs.Get(document.AttrWidth).WholeAtLeast(1)
	h, hasH := attrs.Get(document.AttrHeight).WholeAtLeast(1)

	widths := _gridItemSizes[document.AttrWidth]

	if !hasW || w > _gridColumns {
		w = cmp.Or(widths[attrs.Get(document.AttrWidth).String()], widths[_gridDefaultSize])
	}

	if !hasH || h > _gridMaxRows {
		h = _gridItemSizes[document.AttrHeight][_gridDefaultSize]
	}

	return &gridBox{id: id, placed: hasX && hasY, x: x, y: y, w: w, h: h}
}

// overlaps reports whether two boxes share a cell.
func (b *gridBox) overlaps(other *gridBox) bool {
	return b.x < other.x+other.w && other.x < b.x+b.w && b.y < other.y+other.h && other.y < b.y+b.h
}

// columnBits returns a number with one bit for each column the box spans.
// Bit 0 stands for the first column, bit 1 for the second, and so on. For
// example, a box in columns 4, 5 and 6 sets bits 4, 5 and 6. In binary
// that is 1110000, read from the right.
func (b *gridBox) columnBits() uint32 {
	return (uint32(1)<<b.w - 1) << b.x
}

// takenCells marks the cells that placed boxes cover. It holds one bit for
// each column of a row, so testing a cell costs the same in a grid of any
// size.
type takenCells map[int]uint32

// take marks the cells of a box as taken.
func (t takenCells) take(box *gridBox) {
	for y := box.y; y < box.y+box.h; y++ {
		t[y] |= box.columnBits()
	}
}

// fits reports whether a box stays inside the columns and covers no
// taken cell.
func (t takenCells) fits(box *gridBox) bool {
	if box.x+box.w > _gridColumns {
		return false
	}

	for y := box.y; y < box.y+box.h; y++ {
		if t[y]&box.columnBits() != 0 {
			return false
		}
	}

	return true
}

// readingBoxes lays out a grid's blocks and returns their boxes in the
// order the wide layout shows them: row by row, left to right.
func readingBoxes(blocks []document.Block) []*gridBox {
	boxes := layoutGrid(blocks)

	slices.SortStableFunc(boxes, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x, a.id-b.id)
	})

	return boxes
}

// setStackPlaces gives each box its place in the stacked layout, as the
// editor orders it. boxes are in the wide layout's reading order. Blocks
// sort by their stored place, and blocks with the same place keep the
// reading order. A block without one goes right after the block it
// follows in the wide layout.
func setStackPlaces(blocks []document.Block, boxes []*gridBox) {
	stored := map[*gridBox]int{}
	order := make([]*gridBox, 0, len(boxes))

	for _, box := range boxes {
		if place, placed := blocks[box.id].Attrs.Get(document.AttrStackPos).WholeAtLeast(0); placed {
			stored[box] = place
			order = append(order, box)
		}
	}

	slices.SortStableFunc(order, func(a, b *gridBox) int {
		return stored[a] - stored[b]
	})

	for at, box := range boxes {
		if _, placed := stored[box]; placed {
			continue
		}

		next := 0
		if at > 0 {
			next = slices.Index(order, boxes[at-1]) + 1
		}

		order = slices.Insert(order, next, box)
	}

	for place, box := range order {
		box.stack = place
	}
}

// layoutGrid places a grid's blocks the way the editor does. A block with
// a stored place keeps it, but it moves down until it covers no block
// placed before it. A block without one takes the first free cells, row by
// row. When no block has a place, the search starts after the block before
// it. Then every block floats up and slides left as far as it can. The
// editor lays grids out with the same steps. A change to one has to be made
// to the other, or the assistant reads a grid in another order than people
// see it.
func layoutGrid(blocks []document.Block) []*gridBox {
	items := make([]*gridBox, 0, len(blocks))

	for i, b := range blocks {
		items = append(items, newGridBox(i, b.Attrs))
	}

	placed := slices.DeleteFunc(slices.Clone(items), func(box *gridBox) bool { return !box.placed })

	slices.SortStableFunc(placed, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x)
	})

	var boxes []*gridBox

	for _, box := range placed {
		box.x = min(box.x, _gridColumns-box.w)

		for i := slices.IndexFunc(boxes, box.overlaps); i >= 0; i = slices.IndexFunc(boxes, box.overlaps) {
			box.y = boxes[i].y + boxes[i].h
		}

		boxes = append(boxes, box)
	}

	dense := len(boxes) > 0
	row, column := 0, 0
	taken := takenCells{}

	for _, box := range boxes {
		taken.take(box)
	}

	for _, box := range items {
		if box.placed {
			continue
		}

		box.x, box.y = column, row

		for !taken.fits(box) {
			box.x++

			if box.x+box.w > _gridColumns {
				box.x = 0
				box.y++
			}
		}

		boxes = append(boxes, box)
		taken.take(box)

		if !dense {
			row, column = box.y, box.x+box.w
		}
	}

	compactGrid(boxes)

	return items
}

// gridSizeName returns the size name the model sees for a grid block's
// width or height, as key says. It is the name of the block's span, or
// the nearest name when the span has none. A tie goes to the larger size.
func gridSizeName(attrs document.Attributes, key string) string {
	box := newGridBox(0, attrs)
	span := box.w

	if key == document.AttrHeight {
		span = box.h
	}

	sizes := _gridItemSizes[key]

	return slices.MinFunc(sizeNames(sizes), func(a, b string) int {
		return cmp.Or(
			cmp.Compare(max(sizes[a]-span, span-sizes[a]), max(sizes[b]-span, span-sizes[b])),
			cmp.Compare(sizes[b], sizes[a]),
		)
	})
}

// sizeNames returns the names of sizes, smallest first.
func sizeNames(sizes map[string]int) []string {
	names := slices.Collect(maps.Keys(sizes))

	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(sizes[a], sizes[b]) })

	return names
}

// compactGrid floats every block up as far as it can. Then it slides every
// block left as far as it can. A block that slides left can find free
// cells above it, so the two steps take turns until no block moves. Both
// steps only move blocks up or left, so the turns always end.
func compactGrid(boxes []*gridBox) {
	floatUp(boxes)

	for slideLeft(boxes) {
		if !floatUp(boxes) {
			return
		}
	}
}

// floatUp floats every block up as far as it can, top to bottom. A block
// rises to the bottom of the lowest block above it in its columns, or to
// the first row. The blocks must not overlap. It reports whether a block
// moved.
func floatUp(boxes []*gridBox) bool {
	sorted := slices.Clone(boxes)

	slices.SortStableFunc(sorted, func(a, b *gridBox) int {
		return cmp.Or(a.y-b.y, a.x-b.x)
	})

	// bottoms holds the bottom edge of the blocks placed so far, for each
	// column.
	bottoms := make([]int, _gridColumns)
	moved := false

	for _, box := range sorted {
		top := 0

		for x := box.x; x < box.x+box.w; x++ {
			top = max(top, bottoms[x])
		}

		moved = moved || top != box.y
		box.y = top

		for x := box.x; x < box.x+box.w; x++ {
			bottoms[x] = top + box.h
		}
	}

	return moved
}

// slideLeft slides every block left as far as it can, left to right. A
// block stops at the right edge of the nearest block left of it in its
// rows, or at the first column. The blocks must not overlap. It reports
// whether a block moved.
func slideLeft(boxes []*gridBox) bool {
	sorted := slices.Clone(boxes)

	slices.SortStableFunc(sorted, func(a, b *gridBox) int {
		return cmp.Or(a.x-b.x, a.y-b.y)
	})

	// edges holds the right edge of the blocks placed so far, for each row.
	edges := map[int]int{}
	moved := false

	for _, box := range sorted {
		left := 0

		for y := box.y; y < box.y+box.h; y++ {
			left = max(left, edges[y])
		}

		moved = moved || left != box.x
		box.x = left

		for y := box.y; y < box.y+box.h; y++ {
			edges[y] = left + box.w
		}
	}

	return moved
}
