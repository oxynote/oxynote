// the named widths of a grid block
export enum GridItemWidth {
	Compact = "compact",
	Standard = "standard",
	Wide = "wide",
}

// the named heights of a grid block
export enum GridItemHeight {
	Standard = "standard",
	Tall = "tall",
}

// a block's cells in the widest grid, counted from zero
export interface GridRect {
	x: number
	y: number
	w: number
	h: number
}

// a block's place as its attributes store it, in cells. A block saved
// before blocks stored their place has no x and y.
export interface GridItem {
	x: number | null
	y: number | null
	w: number
	h: number
}

// where a drop puts a block, next to the block at index. Beside puts it at
// that block's left or right side, in the row of the block given by row
// (see placeAtLine). Free puts it into the free cells on that side, in the
// same row (see fillRun). Above and below put it over or under the block
// and push down what it covers. Start puts it at the grid's first cell.
export type GridDropTarget =
	| {
			kind: "beside" | "free"
			index: number
			side: "left" | "right"
			row: number
	  }
	| { kind: "above"; index: number }
	| { kind: "below"; index: number }
	| { kind: "start" }

// a block of the grid while an operation moves it. Its id is its index
// among the grid's children.
interface Box extends GridRect {
	id: number
}

// the columns of the widest grid
export const GRID_COLUMNS = 24

// the columns of each named width, widest first
export const GRID_WIDTH_COLUMNS: [GridItemWidth, number][] = [
	[GridItemWidth.Wide, 24],
	[GridItemWidth.Standard, 12],
	[GridItemWidth.Compact, 8],
]

// the rows of each named height
export const GRID_HEIGHT_ROWS: [GridItemHeight, number][] = [
	[GridItemHeight.Standard, 1],
	[GridItemHeight.Tall, 2],
]

// the most rows a block spans: the rows of the tallest named height. A
// block that stores more, such as a pasted one, gets the default height.
const GRID_MAX_ROWS = Math.max(...GRID_HEIGHT_ROWS.map(([, rows]) => rows))

// the size of a block that stores none: a standard block
export const GRID_DEFAULT_SIZE = { w: 12, h: 1 }

// the attributes that place a block. A size stays with the block, but a
// place belongs to its grid.
export const GRID_PLACE_ATTRS = ["widePosX", "widePosY", "stackPos"] as const

// the attributes that the layout reads from a block: its size and places
export const GRID_LAYOUT_ATTRS = [
	"width",
	"height",
	...GRID_PLACE_ATTRS,
] as const

// gridItemOf reads a block's place and size from its attributes. widePosX
// and widePosY are its column and row in the wide layout. width and height
// are the columns and rows it spans. A block saved before blocks stored
// them has only a named width, such as "standard".
export function gridItemOf(attrs: Record<string, unknown>): GridItem {
	const named = GRID_WIDTH_COLUMNS.find(([name]) => name === attrs.width)

	return {
		x: isWholeAtLeast(attrs.widePosX, 0) ? attrs.widePosX : null,
		y: isWholeAtLeast(attrs.widePosY, 0) ? attrs.widePosY : null,
		w:
			isWholeAtLeast(attrs.width, 1) && attrs.width <= GRID_COLUMNS
				? attrs.width
				: (named?.[1] ?? GRID_DEFAULT_SIZE.w),
		h:
			isWholeAtLeast(attrs.height, 1) && attrs.height <= GRID_MAX_ROWS
				? attrs.height
				: GRID_DEFAULT_SIZE.h,
	}
}

// the named width of a column count, or null when the count has no name
export function widthName(w: number): GridItemWidth | null {
	return GRID_WIDTH_COLUMNS.find(([, columns]) => columns === w)?.[0] ?? null
}

// the named height of a row count, or null when the count has no name
export function heightName(h: number): GridItemHeight | null {
	return GRID_HEIGHT_ROWS.find(([, rows]) => rows === h)?.[0] ?? null
}

// cellChanges returns the wide layout attributes whose values differ from
// rect. Writing only those keeps a collaborator's edits to the same block.
export function cellChanges(
	attrs: Record<string, unknown>,
	rect: GridRect,
): Record<string, number> {
	const cells = {
		widePosX: rect.x,
		widePosY: rect.y,
		width: rect.w,
		height: rect.h,
	}

	return Object.fromEntries(
		Object.entries(cells).filter(([key, value]) => attrs[key] !== value),
	)
}

// withoutPlaces returns a block's attributes without its places. Use it
// for a block that lands somewhere new. The block then takes the first
// free cells there.
export function withoutPlaces(
	attrs: Record<string, unknown>,
): Record<string, unknown> {
	return {
		...attrs,
		...Object.fromEntries(GRID_PLACE_ATTRS.map((key) => [key, null])),
	}
}

// stackPosOf reads a block's place in the stacked layout. It returns null
// when the block has none, so the block follows the wide layout.
export function stackPosOf(attrs: Record<string, unknown>): number | null {
	return isWholeAtLeast(attrs.stackPos, 0) ? attrs.stackPos : null
}

// layoutGrid places every block. A block with a stored place keeps it,
// but it moves down past any block placed before it. A block without one
// takes the first free cells, row by row. But when no block in the grid
// has a place yet, each block goes after the one before it. So a grid
// saved before blocks stored a place keeps its look. Then every block
// floats up and slides left as far as it can (see compact). The
// assistant reads grids in this order, so
// server/core/internal/assistant/markup/grid.go repeats it. A change here
// has to be made there too.
export function layoutGrid(items: GridItem[]): GridRect[] {
	const boxes: Box[] = []
	const placed = items
		.map((item, id) => ({ ...item, id: id }))
		.filter((item) => item.x !== null && item.y !== null)
		.sort((a, b) => (a.y ?? 0) - (b.y ?? 0) || (a.x ?? 0) - (b.x ?? 0))

	for (const item of placed) {
		const box = {
			x: Math.min(item.x ?? 0, GRID_COLUMNS - item.w),
			y: item.y ?? 0,
			w: item.w,
			h: item.h,
			id: item.id,
		}

		moveDownPast(box, boxes)
		boxes.push(box)
	}

	flowItems(items, boxes)
	compact(boxes)

	return toRects(boxes, items.length)
}

// dropInGrid returns the layout after a drop. It returns null when the
// target has no room, or names a block that is not in the grid. A dragged
// block that comes from this grid leaves its place first.
export function dropInGrid(
	rects: readonly GridRect[],
	dragged: { index: number | null; w: number; h: number },
	target: GridDropTarget,
): GridRect[] | null {
	const boxes = rects
		.map((rect, id) => ({ ...rect, id: id }))
		.filter((box) => box.id !== dragged.index)
	const box: Box = {
		x: 0,
		y: 0,
		w: dragged.w,
		h: dragged.h,
		id: dragged.index ?? rects.length,
	}

	// the dragged block leaves first, so the others settle into its cells.
	// Then the target block's new place decides where the block goes.
	compact(boxes)

	if (!placeAtTarget(boxes, box, target)) {
		return null
	}

	const all = [...boxes, box]

	compact(all)

	return toRects(all, Math.max(rects.length, box.id + 1))
}

// readingOrder lists the indexes of a layout's blocks row by row, left to
// right
export function readingOrder(rects: readonly GridRect[]): number[] {
	return rects
		.map((rect, i) => ({ rect: rect, i: i }))
		.sort((a, b) => a.rect.y - b.rect.y || a.rect.x - b.rect.x || a.i - b.i)
		.map(({ i }) => i)
}

// stackOrder lists the indexes of a grid's blocks in the stacked layout.
// Blocks sort by their place in it, and blocks with the same place keep
// the wide layout's reading order. A block without a place goes right after
// the block it follows in the wide layout. The assistant reads grids in
// this order too, so server/core/internal/assistant/markup/grid.go repeats
// it. A change here has to be made there too.
export function stackOrder(
	rects: readonly GridRect[],
	places: readonly (number | null)[],
): number[] {
	const wide = readingOrder(rects)
	const order = wide
		.flatMap((i) => {
			const place = places[i]

			return place == null ? [] : [{ i: i, place: place }]
		})
		.sort((a, b) => a.place - b.place)
		.map(({ i }) => i)

	wide.forEach((i, at) => {
		if (places[i] != null) {
			return
		}

		const previous = wide[at - 1]

		order.splice(previous === undefined ? 0 : order.indexOf(previous) + 1, 0, i)
	})

	return order
}

// restack returns the stacked order after a block moves. The block at
// index goes right before the block at before, or last when before is
// null. An index that is not in the order yet is a block that joins the
// grid.
export function restack(
	order: readonly number[],
	index: number,
	before: number | null,
): number[] {
	const next = order.filter((i) => i !== index)
	const at = before === null ? -1 : next.indexOf(before)

	next.splice(at < 0 ? next.length : at, 0, index)

	return next
}

// stackPlaces inverts a stacked order. It maps each block's index to its
// place.
export function stackPlaces(order: readonly number[]): number[] {
	const places: number[] = []

	order.forEach((i, place) => {
		places[i] = place
	})

	return places
}

// resizeInGrid returns the layout after a block changes its columns or
// rows. The block keeps its left edge, as if it were dropped at the line
// there (see placeAtLine). Then every block floats up and slides left.
export function resizeInGrid(
	rects: readonly GridRect[],
	index: number,
	w: number,
	h: number,
): readonly GridRect[] {
	const current = rects[index]
	if (!current) {
		return rects
	}

	const boxes = rects
		.map((rect, id) => ({ ...rect, id: id }))
		.filter((box) => box.id !== index)
	const box: Box = {
		...current,
		w: w,
		h: h,
		id: index,
	}

	placeAtLine(boxes, box, current.x)

	const all = [...boxes, box]

	compact(all)

	return toRects(all, rects.length)
}

// sameLayout reports whether two layouts put every block in the same cells
export function sameLayout(
	a: readonly GridRect[],
	b: readonly GridRect[],
): boolean {
	return (
		a.length === b.length &&
		a.every((rect, i) => {
			const other = b[i]

			return (
				!!other &&
				rect.x === other.x &&
				rect.y === other.y &&
				rect.w === other.w &&
				rect.h === other.h
			)
		})
	)
}

// overlaps reports whether two rects share a cell
function overlaps(a: GridRect, b: GridRect): boolean {
	return (
		a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
	)
}

// placeAtLine puts the box at the line at x in its row. When the free
// cells around the line are wide enough, the box goes there. Otherwise it
// starts at the line, and the blocks after it in the row slide right into
// free columns. A block that still does not fit keeps its columns and
// moves down. But when the box itself does not fit between the line and
// the right edge, the box moves down instead.
function placeAtLine(boxes: Box[], box: Box, x: number): void {
	const row = box.y
	const inRow = boxes.filter((b) => b.y <= row && row < b.y + b.h)
	const leftEdge = Math.max(
		0,
		...inRow.filter((b) => b.x + b.w <= x).map((b) => b.x + b.w),
	)
	const rightEdge = Math.min(
		GRID_COLUMNS,
		...inRow.filter((b) => b.x >= x).map((b) => b.x),
	)

	if (rightEdge - leftEdge >= box.w) {
		box.x = Math.min(x, rightEdge - box.w)
		pushDown([...boxes, box], [box])

		return
	}

	// blocks reaching into the row from above stay where they are
	const above = inRow.filter((b) => b.y < row)

	box.x = x

	if (box.x + box.w > GRID_COLUMNS || above.some((b) => overlaps(b, box))) {
		box.x = Math.min(x, GRID_COLUMNS - box.w)
		box.y = Math.max(
			row,
			...inRow.filter((b) => overlaps(b, box)).map((b) => b.y + b.h),
		)
		pushDown([...boxes, box], [box])

		return
	}

	const fixed = [box]
	let cursor = box.x + box.w

	for (const b of inRow
		.filter((b) => b.y === row && b.x >= x)
		.sort((a, b) => a.x - b.x)) {
		if (b.x >= cursor) {
			break
		}

		const slid = { ...b, x: cursor }

		if (
			slid.x + slid.w <= GRID_COLUMNS &&
			!above.some((other) => overlaps(other, slid))
		) {
			b.x = slid.x
			cursor = slid.x + slid.w
			fixed.push(b)
		}
	}

	pushDown([...boxes, box], fixed)
}

// placeAtTarget puts the box where the target says, next to the target
// block. It reports false when the target has no room or names no block.
function placeAtTarget(
	boxes: Box[],
	box: Box,
	target: GridDropTarget,
): boolean {
	if (target.kind === "start") {
		placeAtLine(boxes, box, 0)

		return true
	}

	const anchor = boxes.find((b) => b.id === target.index)
	if (!anchor) {
		return false
	}

	if (target.kind === "above" || target.kind === "below") {
		box.x = Math.min(anchor.x, GRID_COLUMNS - box.w)
		box.y = target.kind === "above" ? anchor.y : anchor.y + anchor.h
		pushDown([...boxes, box], [box])

		return true
	}

	box.y = anchor.y + target.row

	if (target.kind === "beside") {
		placeAtLine(
			boxes,
			box,
			target.side === "left" ? anchor.x : anchor.x + anchor.w,
		)

		return true
	}

	const run = freeCellsBeside(boxes, anchor, target.side, box.y)

	return fillRun(boxes, box, run.x, run.w)
}

// freeCellsBeside returns the free cells in row on the anchor's side, up
// to the next block or the grid's edge
function freeCellsBeside(
	boxes: Box[],
	anchor: Box,
	side: "left" | "right",
	row: number,
): { x: number; w: number } {
	const inRow = boxes.filter(
		(b) => b !== anchor && b.y <= row && row < b.y + b.h,
	)

	if (side === "right") {
		const x = anchor.x + anchor.w
		const end = Math.min(
			GRID_COLUMNS,
			...inRow.filter((b) => b.x >= x).map((b) => b.x),
		)

		return { x: x, w: end - x }
	}

	const start = Math.max(
		0,
		...inRow.filter((b) => b.x + b.w <= anchor.x).map((b) => b.x + b.w),
	)

	return { x: start, w: anchor.x - start }
}

// fillRun puts the box at the left end of the free cells from x, w wide.
// A box that is too wide takes the widest named width that fits. When even
// the narrowest width does not fit, the blocks on either side of the run
// shrink to make room. As few blocks as possible shrink, by as few steps
// as possible. It returns false when nothing makes room.
function fillRun(boxes: Box[], box: Box, x: number, w: number): boolean {
	const narrowest = GRID_WIDTH_COLUMNS.at(-1)?.[1] ?? 1
	const row = box.y
	let start = Math.min(Math.max(x, 0), GRID_COLUMNS - 1)
	let width = Math.min(w, GRID_COLUMNS - start)

	const needed = Math.min(box.w, narrowest)

	if (width < needed) {
		const inRow = boxes.filter((b) => b.y <= row && row < b.y + b.h)
		const end = start + width
		const left = inRow.find((b) => b.x + b.w === start)
		const right = inRow.find((b) => b.x === end)
		const fit = cheapestShrink(left, right, width, needed)

		if (!fit) {
			return false
		}

		if (left) {
			left.w = fit.left
		}

		if (right) {
			right.x += right.w - fit.right
			right.w = fit.right
		}

		start = left ? left.x + left.w : start
		width = (right ? right.x : end) - start
	}

	if (box.w > width) {
		box.w =
			GRID_WIDTH_COLUMNS.find(([, columns]) => columns <= width)?.[1] ?? width
	}

	box.x = start
	pushDown([...boxes, box], [box])

	return true
}

// cheapestShrink picks narrower named widths for the blocks on either side
// of a run. Together they must widen the run to at least min columns. It
// prefers the fewest blocks, then the fewest steps. It returns null when
// no widths are enough.
function cheapestShrink(
	left: Box | undefined,
	right: Box | undefined,
	run: number,
	min: number,
): { left: number; right: number } | null {
	const options = (box: Box | undefined) =>
		box
			? [
					box.w,
					...GRID_WIDTH_COLUMNS.map(([, c]) => c).filter((c) => c < box.w),
				]
			: [0]
	// a block that changes costs more than all the steps both sides can
	// take, so fewer blocks always win
	const blockCost = 2 * GRID_WIDTH_COLUMNS.length
	let best: { left: number; right: number; cost: number } | null = null

	for (const [li, l] of options(left).entries()) {
		for (const [ri, r] of options(right).entries()) {
			const gained = (left ? left.w - l : 0) + (right ? right.w - r : 0)
			const cost = (Math.sign(li) + Math.sign(ri)) * blockCost + li + ri

			if (cost > 0 && run + gained >= min && (!best || cost < best.cost)) {
				best = { left: l, right: r, cost: cost }
			}
		}
	}

	return best ? { left: best.left, right: best.right } : null
}

// pushDown moves every block that covers a fixed block down below it.
// Then it does the same for the blocks that those cover, from top to
// bottom. Blocks only move down, never sideways.
function pushDown(all: Box[], fixed: Box[]): void {
	const settled = [...fixed]
	const rest = all
		.filter((box) => !fixed.includes(box))
		.sort((a, b) => a.y - b.y || a.x - b.x)

	for (const box of rest) {
		moveDownPast(box, settled)
		settled.push(box)
	}
}

// compact floats every block up as far as it can. Then it slides every
// block left as far as it can. A block that slides left can find free
// cells above it, so the two steps take turns until no block moves. Both
// steps only move blocks up or left, so the turns always end.
function compact(boxes: Box[]): void {
	floatUp(boxes)

	while (slideLeft(boxes)) {
		if (!floatUp(boxes)) {
			return
		}
	}
}

// floatUp floats every block up as far as it can, from top to bottom. A
// block rises to the bottom of the lowest block above it in its columns,
// or to the first row. The blocks must not overlap. It reports whether a
// block moved.
function floatUp(boxes: Box[]): boolean {
	// the bottom edge of the blocks placed so far, for each column
	const bottoms: number[] = []
	let moved = false

	for (const box of [...boxes].sort((a, b) => a.y - b.y || a.x - b.x)) {
		let y = 0

		for (let x = box.x; x < box.x + box.w; x++) {
			y = Math.max(y, bottoms[x] ?? 0)
		}

		moved ||= y !== box.y
		box.y = y

		for (let x = box.x; x < box.x + box.w; x++) {
			bottoms[x] = y + box.h
		}
	}

	return moved
}

// slideLeft slides every block left as far as it can, from left to right.
// A block stops at the right edge of the nearest block left of it in its
// rows, or at the first column. The blocks must not overlap. It reports
// whether a block moved.
function slideLeft(boxes: Box[]): boolean {
	// the right edge of the blocks placed so far, for each row
	const edges = new Map<number, number>()
	let moved = false

	for (const box of [...boxes].sort((a, b) => a.x - b.x || a.y - b.y)) {
		let x = 0

		for (let y = box.y; y < box.y + box.h; y++) {
			x = Math.max(x, edges.get(y) ?? 0)
		}

		moved ||= x !== box.x
		box.x = x

		for (let y = box.y; y < box.y + box.h; y++) {
			edges.set(y, x + box.w)
		}
	}

	return moved
}

// flowItems places the blocks without a stored place. When some blocks
// have a place, each block takes the first free cells. When none do, each
// block starts after the block before it and takes the first cells where
// it fits. CSS grid auto-placement works the same way. The taken cells are
// kept as one bit per column for each row. So testing a cell costs the
// same in a grid of any size.
function flowItems(items: GridItem[], boxes: Box[]): void {
	const dense = boxes.length > 0
	const taken = new Map<number, number>()
	let row = 0
	let column = 0

	for (const box of boxes) {
		take(taken, box)
	}

	items.forEach((item, id) => {
		if (item.x !== null && item.y !== null) {
			return
		}

		const box: Box = {
			x: column,
			y: row,
			w: item.w,
			h: item.h,
			id: id,
		}

		while (!fits(taken, box)) {
			box.x++

			if (box.x + box.w > GRID_COLUMNS) {
				box.x = 0
				box.y++
			}
		}

		boxes.push(box)
		take(taken, box)

		if (!dense) {
			row = box.y
			column = box.x + box.w
		}
	})
}

// take marks the cells of a box as taken
function take(taken: Map<number, number>, box: GridRect): void {
	for (let y = box.y; y < box.y + box.h; y++) {
		taken.set(y, (taken.get(y) ?? 0) | columnBits(box))
	}
}

// fits reports whether a box stays inside the columns and covers no taken
// cell
function fits(taken: Map<number, number>, box: GridRect): boolean {
	if (box.x + box.w > GRID_COLUMNS) {
		return false
	}

	for (let y = box.y; y < box.y + box.h; y++) {
		if (((taken.get(y) ?? 0) & columnBits(box)) !== 0) {
			return false
		}
	}

	return true
}

// columnBits returns a number with one bit for each column a box spans.
// Bit 0 stands for the first column, bit 1 for the second, and so on. For
// example, a box in columns 4, 5 and 6 sets bits 4, 5 and 6. In binary
// that is 1110000, read from the right.
function columnBits(box: GridRect): number {
	return ((1 << box.w) - 1) << box.x
}

// moveDownPast moves a box down until it covers none of the others
function moveDownPast(box: Box, others: Box[]): void {
	for (;;) {
		const hit = others.find((other) => overlaps(other, box))
		if (!hit) {
			return
		}

		box.y = hit.y + hit.h
	}
}

function toRects(boxes: Box[], length: number): GridRect[] {
	const rects: GridRect[] = Array.from({ length: length }, () => ({
		x: 0,
		y: 0,
		w: 0,
		h: 0,
	}))

	for (const box of boxes) {
		rects[box.id] = { x: box.x, y: box.y, w: box.w, h: box.h }
	}

	return rects
}

function isWholeAtLeast(value: unknown, min: number): value is number {
	return typeof value === "number" && Number.isInteger(value) && value >= min
}
