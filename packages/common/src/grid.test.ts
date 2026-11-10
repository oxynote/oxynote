import { describe, it } from "vitest"
import cases from "../testdata/grid-cases.json"
import {
	GRID_WIDTH_COLUMNS,
	GRID_HEIGHT_ROWS,
	GRID_DEFAULT_SIZE,
	cellChanges,
	dropInGrid,
	type GridDropTarget,
	type GridItem,
	gridItemOf,
	GridItemHeight,
	GridItemWidth,
	type GridRect,
	heightName,
	layoutGrid,
	readingOrder,
	restack,
	resizeInGrid,
	sameLayout,
	stackOrder,
	stackPlaces,
	stackPosOf,
	widthName,
	withoutPlaces,
} from "./grid"

describe("gridItemOf", () => {
	it.for([
		{
			name: "reads a block saved before blocks stored their place by its named width",
			input: { width: GridItemWidth.Compact },
			expected: { x: null, y: null, w: 8, h: 1 },
		},
		{
			name: "reads a block with no width at all as standard",
			input: {},
			expected: { x: null, y: null, w: 12, h: 1 },
		},
		{
			name: "reads its place and size",
			input: { widePosX: 2, widePosY: 4, width: 10, height: 2 },
			expected: { x: 2, y: 4, w: 10, h: 2 },
		},
		{
			name: "falls back to one row for more rows than the tallest height",
			input: { height: 3 },
			expected: { x: null, y: null, w: 12, h: 1 },
		},
		{
			name: "falls back to standard for columns out of range",
			input: { width: 30, height: 0, widePosX: -1, widePosY: 1.5 },
			expected: { x: null, y: null, w: 12, h: 1 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(gridItemOf(input)).toEqual(expected)
	})
})

describe("widthName", () => {
	it.for([
		{ input: 8, expected: GridItemWidth.Compact },
		{ input: 12, expected: GridItemWidth.Standard },
		{ input: 24, expected: GridItemWidth.Wide },
		{ input: 10, expected: null },
	])("names $input columns $expected", ({ input, expected }, { expect }) => {
		expect(widthName(input)).toBe(expected)
	})
})

describe("heightName", () => {
	it.for([
		{ input: 1, expected: GridItemHeight.Standard },
		{ input: 2, expected: GridItemHeight.Tall },
		{ input: 3, expected: null },
	])("names $input rows $expected", ({ input, expected }, { expect }) => {
		expect(heightName(input)).toBe(expected)
	})
})

describe("cellChanges", () => {
	it.for([
		{
			name: "every cell of a block that stores none",
			input: { width: GridItemWidth.Wide },
			expected: { widePosX: 12, widePosY: 3, width: 10, height: 2 },
		},
		{
			name: "only the row of a block pushed down",
			input: { widePosX: 12, widePosY: 1, width: 10, height: 2 },
			expected: { widePosY: 3 },
		},
		{
			name: "nothing for a block already in its cells",
			input: { widePosX: 12, widePosY: 3, width: 10, height: 2 },
			expected: {},
		},
	])("returns $name", ({ input, expected }, { expect }) => {
		expect(cellChanges(input, rect(12, 3, 10, 2))).toEqual(expected)
	})
})

describe("withoutPlaces", () => {
	it("drops the places and keeps the size", ({ expect }) => {
		expect(
			withoutPlaces({ widePosX: 1, widePosY: 2, stackPos: 3, width: 8 }),
		).toEqual({ widePosX: null, widePosY: null, stackPos: null, width: 8 })
	})
})

describe("stackPosOf", () => {
	it.for([
		{ input: { stackPos: 2 }, expected: 2 },
		{ input: { stackPos: 0 }, expected: 0 },
		{ input: {}, expected: null },
		{ input: { stackPos: -1 }, expected: null },
		{ input: { stackPos: 1.5 }, expected: null },
	])("reads $input as $expected", ({ input, expected }, { expect }) => {
		expect(stackPosOf(input)).toBe(expected)
	})
})

// the Go copy of the layout in server/core/internal/assistant/markup/grid.go
// runs the same cases from testdata/grid-cases.json. So both copies have to
// lay out a grid the same way.
describe("the shared grid cases", () => {
	it("hold the sizes the engine names", ({ expect }) => {
		expect(cases.sizes).toEqual({
			width: Object.fromEntries(GRID_WIDTH_COLUMNS),
			height: Object.fromEntries(GRID_HEIGHT_ROWS),
			default: GRID_DEFAULT_SIZE,
		})
	})

	it.for(cases.grids)("lay out and order $name", (grid, { expect }) => {
		const rects = layoutGrid(grid.blocks.map((attrs) => gridItemOf(attrs)))

		expect({
			rects: rects,
			readingOrder: readingOrder(rects),
			stackOrder: stackOrder(
				rects,
				grid.blocks.map((attrs) => stackPosOf(attrs)),
			),
		}).toEqual({
			rects: grid.rects,
			readingOrder: grid.readingOrder,
			stackOrder: grid.stackOrder,
		})
	})
})

describe("layoutGrid", () => {
	it.for([
		{
			name: "flows blocks without a place row by row",
			input: [legacy(12), legacy(12), legacy(8), legacy(24)],
			expected: [
				rect(0, 0, 12),
				rect(12, 0, 12),
				rect(0, 1, 8),
				rect(0, 2, 24),
			],
		},
		{
			name: "keeps the stored places",
			input: [item(12, 0, 12), item(0, 0, 12, 2), item(12, 1, 12)],
			expected: [rect(12, 0, 12), rect(0, 0, 12, 2), rect(12, 1, 12)],
		},
		{
			name: "moves a block down past one that covers its place",
			input: [item(0, 0, 12), item(0, 0, 24)],
			expected: [rect(0, 0, 12), rect(0, 1, 24)],
		},
		{
			name: "floats blocks up into free cells",
			input: [item(0, 3, 12), item(12, 5, 12)],
			expected: [rect(0, 0, 12), rect(12, 0, 12)],
		},
		{
			name: "flows a block without a place after the placed ones and floats it up",
			input: [item(0, 0, 12, 2), item(12, 0, 12), legacy(12)],
			expected: [rect(0, 0, 12, 2), rect(12, 0, 12), rect(12, 1, 12)],
		},
		{
			name: "flows a block without a place into a gap between placed ones",
			input: [item(0, 0, 8), item(16, 0, 8), legacy(8)],
			expected: [rect(0, 0, 8), rect(16, 0, 8), rect(8, 0, 8)],
		},
		{
			name: "flows a tall block past cells taken only in its second row",
			input: [item(0, 0, 12), item(12, 1, 12), legacy(12, 2)],
			expected: [rect(0, 0, 12), rect(12, 0, 12), rect(0, 1, 12, 2)],
		},
		{
			name: "floats a block stored far down up at once",
			input: [item(0, 1_000_000_000, 12)],
			expected: [rect(0, 0, 12)],
		},
		{
			name: "keeps a block inside the grid",
			input: [item(20, 0, 12)],
			expected: [rect(12, 0, 12)],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(layoutGrid(input)).toEqual(expected)
	})

	// blocks saved before blocks stored a place sat on a grid of six
	// columns. They have to stay where that grid put them.
	it("lays out every grid saved before blocks stored a place as the old grid did", ({
		expect,
	}) => {
		for (let run = 0; run < 300; run++) {
			const widths = randomWidths(run)
			const items = widths.map((w) => legacy(w))

			expect(layoutGrid(items)).toEqual(oldGridLayout(widths))
		}
	})
})

describe("dropInGrid", () => {
	it.for([
		{
			name: "puts a block at the line in free cells of the row",
			input: {
				rects: [rect(0, 0, 12)],
				dragged: outside(12),
				target: line(12, 0),
			},
			expected: [rect(0, 0, 12), rect(12, 0, 12)],
		},
		{
			name: "ends a block at the line when the free cells are before it",
			input: {
				rects: [rect(12, 0, 12)],
				dragged: outside(8),
				target: line(12, 0),
			},
			expected: [rect(12, 0, 12), rect(4, 0, 8)],
		},
		{
			name: "slides the blocks after the line right into free columns",
			input: {
				rects: [rect(0, 0, 8), rect(8, 0, 8)],
				dragged: outside(8),
				target: line(8, 0),
			},
			expected: [rect(0, 0, 8), rect(16, 0, 8), rect(8, 0, 8)],
		},
		{
			name: "moves a block after the line with no room down in its columns",
			input: {
				rects: [rect(0, 0, 8), rect(8, 0, 8), rect(16, 0, 8)],
				dragged: outside(8),
				target: line(16, 0),
			},
			expected: [rect(0, 0, 8), rect(8, 0, 8), rect(16, 1, 8), rect(16, 0, 8)],
		},
		{
			name: "moves the dropped block under the line when the row has no room after it",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				dragged: outside(12),
				target: line(24, 0),
			},
			expected: [rect(0, 0, 12), rect(12, 0, 12), rect(12, 1, 12)],
		},
		{
			name: "puts a block from the row under the line on the other side",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				dragged: inside(0, 12),
				target: line(24, 0),
			},
			expected: [rect(12, 1, 12), rect(12, 0, 12)],
		},
		{
			name: "swaps two blocks of a row from the left side of the other",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				dragged: inside(1, 12),
				target: line(0, 0),
			},
			expected: [rect(12, 0, 12), rect(0, 0, 12)],
		},
		{
			name: "puts a block above another and pushes it down",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				dragged: outside(12),
				target: at(12, 0),
			},
			expected: [rect(0, 0, 12), rect(12, 1, 12), rect(12, 0, 12)],
		},
		{
			name: "puts a wide block above a row and pushes the whole row down",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				dragged: outside(24),
				target: at(12, 0),
			},
			expected: [rect(0, 1, 12), rect(12, 1, 12), rect(0, 0, 24)],
		},
		{
			name: "fills free cells beside a tall block",
			input: {
				rects: [rect(0, 0, 12, 2), rect(12, 0, 12)],
				dragged: outside(12),
				target: run(12, 1, 12),
			},
			expected: [rect(0, 0, 12, 2), rect(12, 0, 12), rect(12, 1, 12)],
		},
		{
			name: "shrinks a wide block to the widest width that fits free cells",
			input: {
				rects: [rect(0, 0, 12, 2), rect(12, 0, 12), rect(0, 2, 24)],
				dragged: inside(2, 24),
				target: run(12, 1, 12),
			},
			expected: [rect(0, 0, 12, 2), rect(12, 0, 12), rect(12, 1, 12)],
		},
		{
			name: "shrinks the block beside free cells too narrow for the narrowest width",
			input: {
				rects: [rect(0, 0, 8), rect(8, 0, 12), rect(0, 1, 8)],
				dragged: inside(2, 8),
				target: run(20, 0, 4),
			},
			expected: [rect(0, 0, 8), rect(8, 0, 8), rect(16, 0, 8)],
		},
		{
			name: "puts a block under the line when a tall block covers the cells after it",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12, 2), rect(0, 1, 12)],
				dragged: outside(12),
				target: line(12, 1),
			},
			expected: [
				rect(0, 0, 12),
				rect(12, 0, 12, 2),
				rect(0, 1, 12),
				rect(12, 2, 12),
			],
		},
		{
			name: "leaves the blocks after free columns where they are",
			input: {
				rects: [rect(0, 0, 8), rect(8, 0, 4), rect(20, 0, 4)],
				dragged: outside(8),
				target: line(8, 0),
			},
			expected: [rect(0, 0, 8), rect(16, 0, 4), rect(20, 0, 4), rect(8, 0, 8)],
		},
		{
			name: "moves a block down when a block from above stops its slide",
			input: {
				rects: [
					rect(0, 0, 8),
					rect(8, 0, 8),
					rect(16, 0, 8, 2),
					rect(0, 1, 8),
					rect(8, 1, 8),
				],
				dragged: outside(8),
				target: line(0, 1),
			},
			expected: [
				rect(0, 0, 8),
				rect(8, 0, 8),
				rect(16, 0, 8, 2),
				rect(8, 1, 8),
				rect(8, 2, 8),
				rect(0, 1, 8),
			],
		},
		{
			name: "shrinks a neighbour only as far as a block narrower than any named width needs",
			input: {
				rects: [rect(0, 0, 12), rect(14, 0, 10)],
				dragged: outside(4),
				target: run(12, 0, 2),
			},
			expected: [rect(0, 0, 12), rect(16, 0, 8), rect(12, 0, 4)],
		},
		{
			name: "shrinks the block after free cells too narrow for the narrowest width",
			input: {
				rects: [rect(4, 0, 12)],
				dragged: outside(8),
				target: run(0, 0, 4),
			},
			expected: [rect(8, 0, 8), rect(0, 0, 8)],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(dropInGrid(input.rects, input.dragged, input.target)).toEqual(
			expected,
		)
	})

	it("fills free cells from the right when the line runs along their right side", ({
		expect,
	}) => {
		const rects = [rect(16, 0, 8), rect(0, 1, 24)]

		expect(dropInGrid(rects, outside(12), run(0, 0, 16, "right"))).toEqual([
			rect(16, 0, 8),
			rect(0, 1, 24),
			rect(4, 0, 12),
		])
	})

	it("refuses free cells nothing can widen enough", ({ expect }) => {
		const rects = [rect(0, 0, 8), rect(8, 0, 8), rect(16, 0, 4)]

		expect(dropInGrid(rects, outside(8), run(20, 0, 4))).toBeNull()
	})

	it("keeps every block inside the grid, apart and floated up after any drop", ({
		expect,
	}) => {
		const problems: string[] = []

		for (let seed = 0; seed < 400; seed++) {
			const rects = randomLayout(seed)
			const target = randomTarget(seed, rects)
			const index = seed % 3 === 0 ? null : seed % rects.length
			const w =
				index === null ? pick(seed, [8, 12, 24]) : (rects[index]?.w ?? 12)
			const h = index === null ? pick(seed + 1, [1, 2]) : (rects[index]?.h ?? 1)
			const result = dropInGrid(rects, { index: index, w: w, h: h }, target)

			// only the blocks of the line's row may slide sideways
			problems.push(
				...layoutProblems(result ?? rects).map((p) => `seed ${seed}: ${p}`),
				...movedSideways(
					rects,
					result ?? rects,
					(rect, i) =>
						i === index ||
						target.kind === "run" ||
						(target.kind === "line" && rect.y === target.y),
				).map((p) => `seed ${seed}: ${p}`),
			)
		}

		expect(problems).toEqual([])
	})
})

describe("readingOrder", () => {
	it("reads blocks row by row, left to right", ({ expect }) => {
		expect(
			readingOrder([rect(12, 0, 12), rect(0, 1, 12), rect(0, 0, 12, 2)]),
		).toEqual([2, 0, 1])
	})
})

describe("stackOrder", () => {
	it.for([
		{
			name: "follows the wide layout when no block has a place",
			input: [null, null, null],
			expected: [2, 0, 1],
		},
		{
			name: "sorts blocks by their places",
			input: [0, 2, 1],
			expected: [0, 2, 1],
		},
		{
			name: "keeps the wide order of blocks on the same place",
			input: [0, 0, 0],
			expected: [2, 0, 1],
		},
		{
			name: "puts a block without a place after the block it follows in the wide layout",
			input: [1, null, 0],
			expected: [2, 0, 1],
		},
		{
			name: "puts a block without a place first when it comes first in the wide layout",
			input: [0, 1, null],
			expected: [2, 0, 1],
		},
	])("$name", ({ input, expected }, { expect }) => {
		// the wide layout reads 2, 0, 1
		const rects = [rect(12, 0, 12), rect(0, 1, 12), rect(0, 0, 12, 2)]

		expect(stackOrder(rects, input)).toEqual(expected)
	})
})

describe("restack", () => {
	it.for([
		{
			name: "moves a block right before another",
			input: { index: 2, before: 0 },
			expected: [2, 0, 1],
		},
		{
			name: "moves a block last",
			input: { index: 0, before: null },
			expected: [1, 2, 0],
		},
		{
			name: "adds a block that joins the grid",
			input: { index: 3, before: 1 },
			expected: [0, 3, 1, 2],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(restack([0, 1, 2], input.index, input.before)).toEqual(expected)
	})
})

describe("stackPlaces", () => {
	it("gives each block its place in the order", ({ expect }) => {
		expect(stackPlaces([2, 0, 1])).toEqual([1, 2, 0])
	})
})

describe("resizeInGrid", () => {
	it.for([
		{
			name: "slides the block on the right right when a block widens",
			input: {
				rects: [rect(0, 0, 8), rect(8, 0, 8)],
				index: 0,
				w: 12,
				h: 1,
			},
			expected: [rect(0, 0, 12), rect(12, 0, 8)],
		},
		{
			name: "moves the block on the right down when it has no room",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				index: 0,
				w: 24,
				h: 1,
			},
			expected: [rect(0, 0, 24), rect(12, 1, 12)],
		},
		{
			name: "moves a block that widens past the right edge down a row",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12)],
				index: 1,
				w: 24,
				h: 1,
			},
			expected: [rect(0, 0, 12), rect(0, 1, 24)],
		},
		{
			name: "pushes the block below a block that grows taller down",
			input: {
				rects: [rect(0, 0, 12), rect(0, 1, 12)],
				index: 0,
				w: 12,
				h: 2,
			},
			expected: [rect(0, 0, 12, 2), rect(0, 2, 12)],
		},
		{
			name: "floats the blocks below a block that shrinks up",
			input: {
				rects: [rect(0, 0, 12, 2), rect(0, 2, 12)],
				index: 0,
				w: 12,
				h: 1,
			},
			expected: [rect(0, 0, 12), rect(0, 1, 12)],
		},
		{
			// the top left block turns compact beside a tall block. The block
			// under it stays under it.
			name: "moves no other block sideways when a block shrinks beside a tall one",
			input: {
				rects: [rect(0, 0, 12), rect(12, 0, 12, 2), rect(0, 1, 12)],
				index: 0,
				w: 8,
				h: 1,
			},
			expected: [rect(0, 0, 8), rect(12, 0, 12, 2), rect(0, 1, 12)],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(resizeInGrid(input.rects, input.index, input.w, input.h)).toEqual(
			expected,
		)
	})

	it("returns the layout as it is for a block it does not hold", ({
		expect,
	}) => {
		const rects = [rect(0, 0, 12)]

		expect(resizeInGrid(rects, 3, 8, 1)).toBe(rects)
	})

	it("keeps every block inside the grid, apart and floated up after any resize", ({
		expect,
	}) => {
		const problems: string[] = []

		for (let seed = 0; seed < 400; seed++) {
			const rects = randomLayout(seed)
			const index = seed % rects.length
			const result = resizeInGrid(
				rects,
				index,
				pick(seed, [8, 12, 24]),
				pick(seed + 2, [1, 2]),
			)
			const row = rects[index]?.y

			// only the blocks in the resized block's row may slide sideways
			problems.push(
				...layoutProblems(result).map((p) => `seed ${seed}: ${p}`),
				...movedSideways(
					rects,
					result,
					(rect, i) => i === index || rect.y === row,
				).map((p) => `seed ${seed}: ${p}`),
			)
		}

		expect(problems).toEqual([])
	})
})

describe("sameLayout", () => {
	it.for([
		{
			name: "the same cells",
			input: [rect(0, 0, 12)],
			expected: true,
		},
		{
			name: "another column",
			input: [rect(1, 0, 12)],
			expected: false,
		},
		{
			name: "another row count",
			input: [rect(0, 0, 12, 2)],
			expected: false,
		},
		{ name: "another block count", input: [], expected: false },
	])("tells $name apart", ({ input, expected }, { expect }) => {
		expect(sameLayout([rect(0, 0, 12)], input)).toBe(expected)
	})
})

function rect(x: number, y: number, w: number, h = 1): GridRect {
	return { x: x, y: y, w: w, h: h }
}

function item(x: number, y: number, w: number, h = 1): GridItem {
	return { x: x, y: y, w: w, h: h }
}

function legacy(w: number, h = 1): GridItem {
	return { x: null, y: null, w: w, h: h }
}

function outside(w: number, h = 1) {
	return { index: null, w: w, h: h }
}

function inside(index: number, w: number, h = 1) {
	return { index: index, w: w, h: h }
}

function line(x: number, y: number): GridDropTarget {
	return { kind: "line", x: x, y: y }
}

function at(x: number, y: number): GridDropTarget {
	return { kind: "at", x: x, y: y }
}

function run(
	x: number,
	y: number,
	w: number,
	side: "left" | "right" = "left",
): GridDropTarget {
	return { kind: "run", x: x, y: y, w: w, side: side }
}

// a seeded pick, so a failing run repeats
function pick<T>(seed: number, values: T[]): T {
	const index = Math.abs(Math.sin(seed * 9301 + 49297) * 233280) % values.length

	return values[Math.floor(index)] as T
}

function randomWidths(seed: number): number[] {
	return Array.from({ length: 1 + (seed % 7) }, (_, i) =>
		pick(seed * 31 + i, [8, 12, 24]),
	)
}

// a valid layout of up to seven blocks. Some of them are tall.
function randomLayout(seed: number): GridRect[] {
	const items = Array.from({ length: 1 + (seed % 7) }, (_, i) => ({
		x: pick(seed * 17 + i, [0, 8, 12, 16]),
		y: pick(seed * 13 + i, [0, 1, 2, 3]),
		w: pick(seed * 7 + i, [8, 12, 24]),
		h: pick(seed * 5 + i, [1, 1, 2]),
	}))

	return layoutGrid(items)
}

function randomTarget(seed: number, rects: GridRect[]): GridDropTarget {
	const anchor = rects[seed % rects.length] ?? rect(0, 0, 12)

	switch (seed % 4) {
		case 0:
			return line(anchor.x, anchor.y)
		case 1:
			return line(anchor.x + anchor.w, anchor.y)
		case 2:
			return at(anchor.x, anchor.y + (seed % 2 ? anchor.h : 0))
		default:
			return run(anchor.x, anchor.y + anchor.h, pick(seed, [4, 8, 12]))
	}
}

// oldGridLayout lays out blocks on a grid of six columns. A block spans 2,
// 3 or 6 of them. Each block goes after the one before it, row by row.
function oldGridLayout(widths: number[]): GridRect[] {
	let x = 0
	let y = 0

	return widths.map((w) => {
		const columns = w / 4

		if (x + columns > 6) {
			x = 0
			y++
		}

		const placed = rect(x * 4, y, w)
		x += columns

		return placed
	})
}

// layoutProblems lists what is wrong with a layout. A block may leave the
// grid, two blocks may share a cell, or a block may still float up.
function layoutProblems(rects: readonly GridRect[]): string[] {
	const problems: string[] = []

	rects.forEach((a, i) => {
		if (a.x < 0 || a.x + a.w > 24) {
			problems.push(`block ${i} leaves the grid`)
		}

		rects.forEach((b, j) => {
			if (i < j && overlaps(a, b)) {
				problems.push(`blocks ${i} and ${j} overlap`)
			}
		})

		const raised = { ...a, y: a.y - 1 }

		if (a.y > 0 && !rects.some((b, j) => j !== i && overlaps(b, raised))) {
			problems.push(`block ${i} could float up`)
		}
	})

	return problems
}

// movedSideways lists the blocks that changed their column. It skips the
// blocks that allowed accepts.
function movedSideways(
	before: readonly GridRect[],
	after: readonly GridRect[],
	allowed: (rect: GridRect, index: number) => boolean,
): string[] {
	return before.flatMap((rect, i) =>
		allowed(rect, i) || after[i]?.x === rect.x
			? []
			: [`block ${i} moved from column ${rect.x} to ${String(after[i]?.x)}`],
	)
}

function overlaps(a: GridRect, b: GridRect): boolean {
	return (
		a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
	)
}
