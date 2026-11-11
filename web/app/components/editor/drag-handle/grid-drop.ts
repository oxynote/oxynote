import type { JSONContent } from "@tiptap/core"
import type { Node, Slice } from "@tiptap/pm/model"
import type { EditorView } from "@tiptap/pm/view"
import {
	dropInGrid,
	GRID_COLUMNS,
	type GridDropTarget,
	gridItemOf,
	type GridRect,
	restack,
	sameLayout,
} from "@oxynote/common/grid"
import {
	gridLayoutOf,
	gridStackOf,
	writeGridPlaces,
} from "../blocks/grid/layout"
import { BLOCK_GRID_NAMES, GRID_ITEM_NAMES } from "../blocks/grid/utils"
import { deleteEmptyBlockGrids } from "../blocks/grid"
import { stripNode } from "../tiptap-utils/node"

// a drop into a grid. It holds the grid, where its blocks end up, and the
// line that shows where the dragged block goes, in viewport pixels. A drop
// in the wide layout changes the cells (layout). A drop in the stacked
// layout changes the stacked order (stack). A block from outside the grid
// takes the index after the last child. A stacked drop gives it cells too.
export interface GridDrop {
	doc: Node
	gridPos: number
	draggedIndex: number | null
	layout: GridRect[] | null
	stack: number[] | null
	line: GridDropLine
}

export type GridDropLine =
	| { vertical: true; x: number; top: number; bottom: number }
	| { vertical: false; y: number; left: number; right: number }

// a grid block with its cells in the layout and its box on the screen
interface GridCell {
	index: number
	pos: number
	rect: GridRect
	box: DOMRect
}

// the columns and rows the widest grid draws, in viewport pixels
interface GridGeometry {
	lefts: number[]
	rights: number[]
	top: number
	rowHeight: number
	rowGap: number
	columnGap: number
}

type GridDropPlan = Pick<GridDrop, "layout" | "stack" | "line">

// a target that a point over the widest grid can pick. It always names a
// block of the grid.
type WideTarget = Exclude<GridDropTarget, { kind: "start" }>

// the dragged block and the columns and rows it spans. Its index is null
// when the block comes from outside the grid.
interface DraggedBlock {
	index: number | null
	w: number
	h: number
}

// how far outside a box a point may be and still count as inside it
const EDGE_PX = 1

// how thick the drop cursor is
const LINE_REM = 0.1875

// findGridDrop finds where a grid block dragged to the point lands in the
// grid under it. A block that is copied, not moved, comes into the grid as
// a new one. It returns undefined when the slice is not one grid block or
// no grid is under the point. It returns null when the drop would change nothing
// or has no room.
export function findGridDrop(
	view: EditorView,
	x: number,
	y: number,
	slice: Slice | null,
	moving: boolean,
): GridDrop | null | undefined {
	const dragged = slice?.content.firstChild
	if (
		!dragged ||
		slice.content.childCount > 1 ||
		!GRID_ITEM_NAMES.has(dragged.type.name)
	) {
		return undefined
	}

	const grid = gridAt(view, x, y)
	if (!grid) {
		return undefined
	}

	const rects = gridLayoutOf(grid.node)
	const cells = gridCells(view, grid.node, grid.pos, rects)
	const content = cells[0] ? view.nodeDOM(cells[0].pos)?.parentElement : null
	const geometry = content instanceof HTMLElement ? gridGeometry(content) : null
	const index = moving ? draggedIndexIn(view, grid.node, grid.pos) : null
	const size =
		(index === null ? null : rects[index]) ?? gridItemOf(dragged.attrs)
	const block = { index: index, w: size.w, h: size.h }
	const point = { x: x, y: y }
	const plan =
		cells.length === 0
			? emptyGridDrop(view, grid.pos, block)
			: geometry
				? wideDrop(cells, geometry, rects, block, point)
				: stackedDrop(grid.node, cells, rects, block, point, content)

	return (
		plan && {
			doc: view.state.doc,
			gridPos: grid.pos,
			draggedIndex: index,
			...plan,
		}
	)
}

// executeGridDrop writes a grid drop. Every block whose cells or stacked
// place change gets the new ones. A block from outside the grid is added
// to it, and a moved block is deleted from its old place. A grid that the
// move leaves empty is deleted too. It returns false when the document
// changed since the drop was found.
export function executeGridDrop(
	view: EditorView,
	drop: GridDrop,
	slice: Slice,
	moved: boolean,
): boolean {
	const { state } = view
	const grid = state.doc.nodeAt(drop.gridPos)
	const dragged = slice.content.firstChild

	if (state.doc !== drop.doc || !grid || !dragged) {
		return false
	}

	const tr = state.tr
	const { from, to, empty } = state.selection
	const joins = drop.draggedIndex === null

	if (joins) {
		// a copy is a new block, so it shares no uid or comment with the
		// block it copies
		const block = moved
			? dragged
			: state.schema.nodeFromJSON(stripNode(dragged.toJSON() as JSONContent))

		// a block from another grid does not keep its old stacked place
		tr.insert(
			drop.gridPos + grid.nodeSize - 1,
			block.type.create(
				{ ...block.attrs, stackPos: null },
				block.content,
				block.marks,
			),
		)
	}

	writeGridPlaces(tr, drop.gridPos, drop.layout, drop.stack)

	if (joins && moved && !empty) {
		tr.delete(tr.mapping.map(from), tr.mapping.map(to))
	}

	view.dispatch(deleteEmptyBlockGrids(tr))

	return true
}

// gridDropLineStyle returns the drop cursor's style for a grid line,
// relative to the editor. The cursor is centred on the line.
export function gridDropLineStyle(
	line: GridDropLine,
	editorRect: DOMRect,
): Record<string, string> {
	const thickness = `${LINE_REM}rem`
	const centred = (px: number) => `calc(${px}px - ${LINE_REM / 2}rem)`

	return line.vertical
		? {
				top: `${line.top - editorRect.top}px`,
				left: centred(line.x - editorRect.left),
				width: thickness,
				height: `${line.bottom - line.top}px`,
				display: "block",
			}
		: {
				top: centred(line.y - editorRect.top),
				left: `${line.left - editorRect.left}px`,
				width: `${line.right - line.left}px`,
				height: thickness,
				display: "block",
			}
}

// wideDrop finds where a block lands in the widest grid
function wideDrop(
	cells: GridCell[],
	geometry: GridGeometry,
	rects: readonly GridRect[],
	dragged: DraggedBlock,
	point: { x: number; y: number },
): GridDropPlan | null {
	const target = wideTarget(cells, geometry, rects, dragged, point)
	const layout = target && dropInGrid(rects, dragged, target)
	const landed = layout?.[dragged.index ?? rects.length]

	if (
		!target ||
		!layout ||
		!landed ||
		(dragged.index !== null && sameLayout(layout, rects))
	) {
		return null
	}

	return {
		layout: layout,
		stack: null,
		line: dropLine(geometry, rects, dragged, target, layout, landed),
	}
}

// emptyGridDrop puts a block into a grid with no blocks, at its first
// cell. The line runs along the grid's top edge.
function emptyGridDrop(
	view: EditorView,
	gridPos: number,
	dragged: DraggedBlock,
): GridDropPlan | null {
	const dom = view.nodeDOM(gridPos)
	if (!(dom instanceof HTMLElement)) {
		return null
	}

	const box = dom.getBoundingClientRect()

	return {
		layout: dropInGrid([], dragged, { kind: "start" }),
		stack: null,
		line: horizontal(box.top, box),
	}
}

// stackedDrop finds where a block lands in a narrower grid, which stacks
// its blocks one per row in their stacked order
function stackedDrop(
	grid: Node,
	cells: GridCell[],
	rects: readonly GridRect[],
	dragged: DraggedBlock,
	point: { x: number; y: number },
	content: Element | null | undefined,
): GridDropPlan | null {
	const order = gridStackOf(grid)
	const found = stackedTarget(cells, order, dragged.index, point, content)

	if (!found) {
		return null
	}

	const id = dragged.index ?? rects.length
	const stack = restack(order, id, found.before)

	if (dragged.index !== null) {
		return { layout: null, stack: stack, line: found.line }
	}

	// a block from outside the grid also needs cells in the wide layout.
	// It goes right of the block it follows in the stack.
	const previous = stack[stack.indexOf(id) - 1]
	const layout = dropInGrid(
		rects,
		dragged,
		previous === undefined
			? { kind: "start" }
			: { kind: "beside", index: previous, side: "right", row: 0 },
	)

	return { layout: layout, stack: stack, line: found.line }
}

// dropLine is the line that shows where a block lands. It sits at the
// target block, where the grid shows it during the drag. A drop beside a
// block, or into the free cells right of it, shows that side. A drop above
// or below a block shows the free cells there. Free cells at the grid's
// left edge show their top. A block that cannot land at its target, such
// as one with no room there, gets the line where it lands instead.
function dropLine(
	geometry: GridGeometry,
	rects: readonly GridRect[],
	dragged: DraggedBlock,
	target: WideTarget,
	layout: readonly GridRect[],
	landed: GridRect,
): GridDropLine {
	const shown = rects[target.index]
	const moved = layout[target.index]

	if (shown && moved && landsAtTarget(target, landed, moved)) {
		switch (target.kind) {
			case "above":
				return zoneLine(geometry, rects, dragged.index, shown.y, shown)
			case "below":
				return zoneLine(
					geometry,
					rects,
					dragged.index,
					shown.y + shown.h,
					shown,
				)
		}

		const row = shown.y + target.row

		if (target.side === "right") {
			return sideLine(geometry, shown.x + shown.w, row, 1)
		}

		// free cells on the left end at the block, so their line grows
		// from the cell right before it
		return target.kind === "beside"
			? sideLine(geometry, shown.x, row, 1)
			: zoneLine(geometry, rects, dragged.index, row, {
					x: shown.x - 1,
					w: 1,
				})
	}

	return landed.x > 0
		? sideLine(geometry, landed.x, landed.y, landed.h)
		: zoneLine(
				geometry,
				layout,
				dragged.index ?? rects.length,
				landed.y,
				landed,
			)
}

// landsAtTarget reports whether a block lands where its target says, next
// to the target block in that block's new place
function landsAtTarget(
	target: WideTarget,
	landed: GridRect,
	block: GridRect,
): boolean {
	const sharesRows =
		landed.y < block.y + block.h && block.y < landed.y + landed.h

	switch (target.kind) {
		case "above":
			return landed.y + landed.h === block.y
		case "below":
			return landed.y === block.y + block.h
	}

	if (target.side === "right") {
		return sharesRows && landed.x === block.x + block.w
	}

	return (
		sharesRows &&
		(target.kind === "beside"
			? landed.x + landed.w === block.x
			: landed.x + landed.w <= block.x)
	)
}

// wideTarget finds the target in the widest grid. Over a block, the point
// picks the nearest side, so the drop goes above, below, left or right of
// the block. A left or right drop takes the block's row at the point's
// height. Over free cells, the drop fills them. Anywhere else, such as in
// the gap between two blocks, the nearest block counts.
function wideTarget(
	cells: GridCell[],
	geometry: GridGeometry,
	rects: readonly GridRect[],
	dragged: DraggedBlock,
	point: { x: number; y: number },
): WideTarget | null {
	const under = cells.find((cell) => contains(cell.box, point))
	if (under?.index === dragged.index) {
		return null
	}

	if (!under) {
		const free = freeTarget(geometry, rects, dragged, point)
		if (free) {
			return free
		}
	}

	const cell = under ?? nearest(cells, dragged.index, point)
	if (!cell) {
		return null
	}

	const { index, rect, box } = cell
	const step = geometry.rowHeight + geometry.rowGap
	const row = Math.min(
		rect.h - 1,
		Math.max(0, Math.floor((point.y - box.top + geometry.rowGap / 2) / step)),
	)
	const side = sideOf(box, point)

	switch (side) {
		case "top":
			return { kind: "above", index: index }
		case "bottom":
			return { kind: "below", index: index }
		default:
			return { kind: "beside", index: index, side: side, row: row }
	}
}

// stackedTarget finds the target in a narrower grid, which stacks its
// blocks one per row in their stacked order. Over the upper half of a
// block, the drop goes right before it. Over the lower half, it goes right
// before the next block, or last when there is none. It returns null when
// the order would stay the same.
function stackedTarget(
	cells: GridCell[],
	stacked: readonly number[],
	draggedIndex: number | null,
	point: { x: number; y: number },
	content: Element | null | undefined,
): { before: number | null; line: GridDropLine } | null {
	const cell =
		cells.find((candidate) => contains(candidate.box, point)) ??
		nearest(cells, draggedIndex, point)

	if (!cell || cell.index === draggedIndex) {
		return null
	}

	const { box } = cell
	const rowGap = content ? parseFloat(getComputedStyle(content).rowGap) || 0 : 0
	const order = stacked.filter((index) => index !== draggedIndex)
	const upper = point.y < box.top + box.height / 2
	const at = order.indexOf(cell.index) + (upper ? 0 : 1)
	const before = order[at] ?? null
	const draggedAt = draggedIndex === null ? -1 : stacked.indexOf(draggedIndex)

	// the dragged block already sits right there
	if (draggedAt >= 0 && draggedAt === at) {
		return null
	}

	return {
		before: before,
		line: horizontal(
			upper ? box.top - rowGap / 2 : box.bottom + rowGap / 2,
			box,
		),
	}
}

// freeTarget finds the free cells under the point. From the point's row,
// the run moves up while the cell above in the point's column is free. So
// the block does not float away from the point. The run spans the free
// cells of that row. The block on the run's left anchors it, or else the
// block on its right. A run that touches no block gives no target.
function freeTarget(
	geometry: GridGeometry,
	rects: readonly GridRect[],
	dragged: DraggedBlock,
	point: { x: number; y: number },
): WideTarget | null {
	const step = geometry.rowHeight + geometry.rowGap
	const column = columnAt(geometry, point.x)
	let row = Math.floor((point.y - geometry.top + geometry.rowGap / 2) / step)
	// the dragged block leaves its cells, so they count as free
	const blockAt = (r: number, c: number) =>
		rects.findIndex((rect, i) => i !== dragged.index && covers(rect, r, c))

	if (row < 0 || blockAt(row, column) >= 0) {
		return null
	}

	while (row > 0 && blockAt(row - 1, column) < 0) {
		row--
	}

	let start = column
	let stop = column + 1

	while (start > 0 && blockAt(row, start - 1) < 0) {
		start--
	}

	while (stop < geometry.lefts.length && blockAt(row, stop) < 0) {
		stop++
	}

	const left = blockAt(row, start - 1)
	const index = left >= 0 ? left : blockAt(row, stop)
	const anchor = rects[index]

	if (!anchor) {
		return null
	}

	return {
		kind: "free",
		index: index,
		side: left >= 0 ? "right" : "left",
		row: row - anchor.y,
	}
}

// sideLine is the line along the left edge of a column, as tall as rows.
// Past the last column, it runs along the grid's right edge.
function sideLine(
	geometry: GridGeometry,
	column: number,
	row: number,
	rows: number,
): GridDropLine {
	const step = geometry.rowHeight + geometry.rowGap
	const top = geometry.top + row * step
	const x =
		column < geometry.lefts.length
			? (geometry.lefts[column] ?? 0) - geometry.columnGap / 2
			: (geometry.rights.at(-1) ?? 0) + geometry.columnGap / 2

	return vertical(x, top, top + rows * step - geometry.rowGap)
}

// zoneLine is the line along the top of row. It spans the columns of span
// and the open cells on either side of them. A cell is open when it is
// free and a block or the grid's top edge is right above it. The block at
// skip counts as free in row, since a dragged block leaves its cells. But
// above row it still shows, so there it counts as a block.
function zoneLine(
	geometry: GridGeometry,
	rects: readonly GridRect[],
	skip: number | null,
	row: number,
	span: { x: number; w: number },
): GridDropLine {
	const taken = (r: number, c: number, skipped: boolean) =>
		rects.some((rect, i) => (!skipped || i !== skip) && covers(rect, r, c))
	const open = (c: number) =>
		!taken(row, c, true) && (row === 0 || taken(row - 1, c, false))
	const step = geometry.rowHeight + geometry.rowGap
	let start = span.x
	let stop = span.x + span.w

	while (start > 0 && open(start - 1)) {
		start--
	}

	while (stop < geometry.lefts.length && open(stop)) {
		stop++
	}

	return {
		vertical: false,
		y: geometry.top + row * step - geometry.rowGap / 2,
		left: geometry.lefts[start] ?? 0,
		right: geometry.rights[stop - 1] ?? 0,
	}
}

// sideOf names the side of a box nearest to a point. The box's
// diagonals are the borders between sides, so a block of any size has all
// four sides.
function sideOf(
	box: DOMRect,
	point: { x: number; y: number },
): "top" | "bottom" | "left" | "right" {
	// how far the point sits across the box and down it, from 0 to 1
	const fromLeft = clamp((point.x - box.left) / box.width)
	const fromTop = clamp((point.y - box.top) / box.height)

	if (fromTop < fromLeft && fromTop < 1 - fromLeft) {
		return "top"
	}

	if (fromTop > fromLeft && fromTop > 1 - fromLeft) {
		return "bottom"
	}

	return fromLeft <= 0.5 ? "left" : "right"
}

// the grid under a point. Any x inside the editor counts, so the margins
// beside the grid count too. Grids sit only at the document root.
function gridAt(
	view: EditorView,
	x: number,
	y: number,
): { node: Node; pos: number } | null {
	const editor = view.dom.getBoundingClientRect()

	if (x < editor.left || x > editor.right) {
		return null
	}

	let pos = 0

	for (const node of view.state.doc.children) {
		const dom = BLOCK_GRID_NAMES.has(node.type.name) ? view.nodeDOM(pos) : null

		if (dom instanceof HTMLElement) {
			const box = dom.getBoundingClientRect()

			if (y >= box.top && y <= box.bottom) {
				return { node: node, pos: pos }
			}
		}

		pos += node.nodeSize
	}

	return null
}

function gridCells(
	view: EditorView,
	grid: Node,
	gridPos: number,
	rects: readonly GridRect[],
): GridCell[] {
	const cells: GridCell[] = []

	grid.forEach((_child, offset, i) => {
		const pos = gridPos + 1 + offset
		const dom = view.nodeDOM(pos)
		const rect = rects[i]

		if (dom instanceof HTMLElement && rect) {
			cells.push({
				index: i,
				pos: pos,
				rect: rect,
				box: dom.getBoundingClientRect(),
			})
		}
	})

	return cells
}

// the dragged block's index in the grid, or null when it comes from
// outside it. The drag handle selects the block it drags.
function draggedIndexIn(
	view: EditorView,
	grid: Node,
	gridPos: number,
): number | null {
	const { from, to, empty } = view.state.selection
	let index: number | null = null

	if (empty) {
		return null
	}

	grid.forEach((child, offset, i) => {
		const pos = gridPos + 1 + offset

		if (pos === from && pos + child.nodeSize === to) {
			index = i
		}
	})

	return index
}

// gridGeometry reads the widest grid's columns and rows from its computed
// style. It returns null for a narrower grid, which stacks its blocks.
function gridGeometry(content: HTMLElement): GridGeometry | null {
	const style = getComputedStyle(content)
	const widths = style.gridTemplateColumns
		.split(" ")
		.map((width) => parseFloat(width))
		.filter((width) => !isNaN(width))
	const rowHeight = parseFloat(style.gridAutoRows)

	if (widths.length !== GRID_COLUMNS || !(rowHeight > 0)) {
		return null
	}

	const box = content.getBoundingClientRect()
	const columnGap = parseFloat(style.columnGap) || 0
	const lefts: number[] = []
	const rights: number[] = []
	let x = box.left + (parseFloat(style.paddingLeft) || 0)

	for (const width of widths) {
		lefts.push(x)
		rights.push(x + width)
		x += width + columnGap
	}

	return {
		lefts: lefts,
		rights: rights,
		top: box.top + (parseFloat(style.paddingTop) || 0),
		rowHeight: rowHeight,
		rowGap: parseFloat(style.rowGap) || 0,
		columnGap: columnGap,
	}
}

// the column under an x, or the nearest one when x is outside the grid
function columnAt(geometry: GridGeometry, x: number): number {
	const index = geometry.rights.findIndex(
		(right) => x < right + geometry.columnGap / 2,
	)

	return index < 0 ? geometry.lefts.length - 1 : index
}

// the block nearest the point, other than the dragged one
function nearest(
	cells: GridCell[],
	draggedIndex: number | null,
	point: { x: number; y: number },
): GridCell | null {
	let best: GridCell | null = null
	let bestDistance = Infinity

	for (const cell of cells) {
		const distance = Math.hypot(
			Math.max(cell.box.left - point.x, 0, point.x - cell.box.right),
			Math.max(cell.box.top - point.y, 0, point.y - cell.box.bottom),
		)

		if (cell.index !== draggedIndex && distance < bestDistance) {
			best = cell
			bestDistance = distance
		}
	}

	return best
}

// covers reports whether a rect holds the cell in row and column
function covers(rect: GridRect, row: number, column: number): boolean {
	return (
		rect.x <= column &&
		column < rect.x + rect.w &&
		rect.y <= row &&
		row < rect.y + rect.h
	)
}

function contains(box: DOMRect, point: { x: number; y: number }): boolean {
	return (
		point.x >= box.left - EDGE_PX &&
		point.x <= box.right + EDGE_PX &&
		point.y >= box.top - EDGE_PX &&
		point.y <= box.bottom + EDGE_PX
	)
}

function vertical(x: number, top: number, bottom: number): GridDropLine {
	return { vertical: true, x: x, top: top, bottom: bottom }
}

function horizontal(y: number, box: DOMRect): GridDropLine {
	return { vertical: false, y: y, left: box.left, right: box.right }
}

function clamp(value: number): number {
	return Math.min(Math.max(value, 0), 1)
}
