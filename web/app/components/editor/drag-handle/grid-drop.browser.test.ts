import { Editor, type JSONContent } from "@tiptap/core"
import { ContentDocument } from "../blocks/document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Fragment, Slice } from "@tiptap/pm/model"
import { NodeSelection } from "@tiptap/pm/state"
import { afterEach, describe, it } from "vitest"
import { GridItemWidth } from "@oxynote/common/grid"
import {
	executeGridDrop,
	findGridDrop,
	type GridDrop,
	gridDropLineStyle,
} from "./grid-drop"
import { isWideGrid } from "../blocks/grid/screen-order"
import {
	blockElement,
	blockGrid,
	GRID_STYLES,
	GridLayout,
	legacyGrid,
	metricBlock,
	pointIn,
	posOfMetricBlock,
	RenderedBlockGrid,
	RenderedLegacyMetricGrid,
	storedCells,
	storedStackPlaces,
	TitledMetricBlock,
} from "./test-helpers"
import { paragraph } from "~/components/editor/test-helpers"

// the breakpoint grid switches to its columns at 56rem. GridView.vue's
// @4xl: classes switch at the same width.
// The tests cannot load the Tailwind styles, so this mirrors them.
const style = document.createElement("style")
style.textContent = `
	body { margin: 0; }
	${GRID_STYLES}
	.breakpoint-grid { container-type: inline-size; }
	.breakpoint-grid [data-type="block-grid"] {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		grid-auto-rows: 40px;
		gap: 4px;
	}
	@container (width >= 56rem) {
		.breakpoint-grid [data-type="block-grid"] {
			grid-template-columns: repeat(24, minmax(0, 1fr));
		}
	}
`
document.head.appendChild(style)

const cleanups: (() => void)[] = []

// the suites drive real editors in the one shared page, so they cannot
// interleave
describe("findGridDrop", { concurrent: false }, () => {
	afterEach(cleanUp)

	it("finds nothing when no grid block is dragged", ({ expect }) => {
		const editor = mountEditor([blockGrid(metricBlock("a")), paragraph("p")])
		const point = pointIn(blockElement(editor, "a"), 0.5, 0.5)

		expect(
			findGridDrop(
				editor.view,
				point.x,
				point.y,
				new Slice(Fragment.from(editor.state.doc.child(1)), 0, 0),
				true,
			),
		).toBeUndefined()
	})

	it("finds nothing when a grid block is dragged with more content", ({
		expect,
	}) => {
		const editor = mountEditor([blockGrid(metricBlock("a")), paragraph("p")])
		const point = pointIn(blockElement(editor, "a"), 0.5, 0.5)
		const slice = new Slice(
			Fragment.from([
				editor.state.doc.child(0).child(0),
				editor.state.doc.child(1),
			]),
			0,
			0,
		)

		expect(
			findGridDrop(editor.view, point.x, point.y, slice, false),
		).toBeUndefined()
	})

	it("takes a moved block for one from outside the grid while nothing is selected", ({
		expect,
	}) => {
		const editor = mountEditor(
			[paragraph("p"), blockGrid(metricBlock("a"), metricBlock("b"))],
			"wide-grid",
		)
		const point = pointIn(blockElement(editor, "b"), 0.5, 0.9)

		const drop = findGridDrop(
			editor.view,
			point.x,
			point.y,
			sliceOf(editor, "a"),
			true,
		)

		expect.soft(drop?.draggedIndex).toBeNull()
		expect
			.soft(drop?.layout)
			.toEqual([rect(0, 0, 12), rect(12, 0, 12), rect(0, 1, 12)])
	})

	it("finds no grid beside the editor at a grid's height", ({ expect }) => {
		const editor = mountEditor([
			blockGrid(metricBlock("a")),
			blockGrid(metricBlock("x")),
		])
		const box = blockElement(editor, "a").getBoundingClientRect()
		const editorRight = editor.view.dom.getBoundingClientRect().right

		expect(
			dropAt(editor, "x", { x: editorRight + 50, y: box.top + 10 }),
		).toBeUndefined()
	})

	describe("in the widest grid", () => {
		it.for([
			{
				name: "at the line on a block's left side",
				input: { u: 0.1, v: 0.5 },
				expected: {
					line: "left",
					layout: [rect(0, 0, 12), rect(0, 1, 12), rect(12, 0, 12)],
				},
			},
			{
				name: "at the start of the next row from the line on a full row's right end",
				input: { u: 0.9, v: 0.5 },
				expected: {
					line: "next row",
					layout: [rect(0, 0, 12), rect(12, 0, 12), rect(0, 1, 12)],
				},
			},
			{
				name: "above a block from its top side",
				input: { u: 0.5, v: 0.1 },
				expected: {
					line: "top",
					layout: [rect(0, 0, 12), rect(0, 1, 12), rect(12, 0, 12)],
				},
			},
			{
				name: "at the start of the next row from a block's bottom side",
				input: { u: 0.5, v: 0.9 },
				expected: {
					line: "next row",
					layout: [rect(0, 0, 12), rect(12, 0, 12), rect(0, 1, 12)],
				},
			},
		] as const)("puts a block $name", ({ input, expected }, { expect }) => {
			const editor = mountEditor(
				[
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				"wide-grid",
			)
			const first = blockElement(editor, "a").getBoundingClientRect()
			const box = blockElement(editor, "b").getBoundingClientRect()

			const drop = dropAt(
				editor,
				"x",
				pointIn(blockElement(editor, "b"), input.u, input.v),
			)

			expect.soft(drop?.layout).toEqual(expected.layout)
			expect.soft(drop?.stack).toBeNull()
			expect.soft(drop?.draggedIndex).toBeNull()
			// the line below the row spans the whole row, since the block
			// lands at its start
			expect.soft(drop?.line).toEqual(
				expected.line === "next row"
					? {
							vertical: false,
							y: box.bottom + 2,
							left: first.left,
							right: box.right,
						}
					: sideLine(box, expected.line),
			)
		})

		it.for([
			{
				name: "above the other block from that block's top side",
				input: { u: 0.5, v: 0.1 },
				expected: {
					layout: [rect(0, 0, 12), rect(0, 1, 12)],
					line: (_first: DOMRect, box: DOMRect) => ({
						vertical: false,
						y: box.top - 2,
						left: box.left,
						right: box.right,
					}),
				},
			},
			{
				name: "right of the other block from that block's right side, so the two swap",
				input: { u: 0.9, v: 0.5 },
				expected: {
					layout: [rect(12, 0, 12), rect(0, 0, 12)],
					line: (_first: DOMRect, box: DOMRect) => ({
						vertical: true,
						x: box.right + 2,
						top: box.top,
						bottom: box.bottom,
					}),
				},
			},
			{
				name: "under the other block from that block's bottom side",
				input: { u: 0.5, v: 0.9 },
				expected: {
					layout: [rect(0, 1, 12), rect(0, 0, 12)],
					line: (first: DOMRect, box: DOMRect) => ({
						vertical: false,
						y: box.bottom + 2,
						left: first.left,
						right: box.right,
					}),
				},
			},
		])("moves a block of a row $name", ({ input, expected }, { expect }) => {
			const editor = mountEditor(
				[blockGrid(metricBlock("a"), metricBlock("b"))],
				"wide-grid",
			)
			const first = blockElement(editor, "a").getBoundingClientRect()
			const box = blockElement(editor, "b").getBoundingClientRect()

			const drop = dropAt(
				editor,
				"a",
				pointIn(blockElement(editor, "b"), input.u, input.v),
			)

			// a leaves its cells first, so b slides left into them. The
			// line still sits at b, where it shows during the drag.
			expect.soft(drop?.layout).toEqual(expected.layout)
			expect.soft(drop?.line).toEqual(expected.line(first, box))
		})

		describe("in a grid of three columns with a tall block", () => {
			it.for([
				{ name: "the top of b, right under c", input: ["b", 0.5, 0.1] },
				{ name: "the right side of a, beside c", input: ["a", 0.95, 0.25] },
				{ name: "the left side of d, beside c", input: ["d", 0.05, 0.5] },
				{
					name: "the left side of b, where c would land in its own place",
					input: ["b", 0.05, 0.5],
				},
			] as const)("finds nothing for c at $name", ({ input }, { expect }) => {
				const editor = mountEditor(tallGrid(), "wide-grid")
				const [title, u, v] = input

				expect(
					dropAt(editor, "c", pointIn(blockElement(editor, title), u, v)),
				).toBeNull()
			})

			it.for([
				{
					// b rises into c's place and e slides into b's, so d drops
					// into e's place
					name: "c above d, which moves down",
					input: { dragged: "c", point: ["d", 0.5, 0.1] },
					expected: {
						layout: [
							rect(0, 0, 8, 2),
							rect(16, 0, 8),
							rect(16, 1, 8),
							rect(8, 0, 8),
							rect(8, 1, 8),
						],
						line: (box: BoxOf) => ({
							vertical: false,
							y: box("d").top - 2,
							left: box("d").left,
							right: box("d").right,
						}),
					},
				},
				{
					// c drops into the place b leaves, so e stays
					name: "b above c, which drops into b's place",
					input: { dragged: "b", point: ["c", 0.5, 0.1] },
					expected: {
						layout: [
							rect(0, 0, 8, 2),
							rect(8, 1, 8),
							rect(16, 0, 8),
							rect(8, 0, 8),
							rect(16, 1, 8),
						],
						line: (box: BoxOf) => ({
							vertical: false,
							y: box("c").top - 2,
							left: box("c").left,
							right: box("c").right,
						}),
					},
				},
				{
					// a takes its lower half along, so b slides right too, and e
					// ends under c
					name: "c left of the tall block from low on its side",
					input: { dragged: "c", point: ["a", 0.05, 0.75] },
					expected: {
						layout: [
							rect(8, 0, 8, 2),
							rect(0, 0, 8),
							rect(16, 0, 8),
							rect(16, 1, 8),
							rect(0, 1, 8),
						],
						line: (box: BoxOf) => ({
							vertical: true,
							x: box("a").left - 2,
							top: box("a").top,
							bottom: box("a").bottom,
						}),
					},
				},
				{
					name: "c right of e after the gap c leaves fills",
					input: { dragged: "c", point: ["e", 0.95, 0.5] },
					expected: {
						layout: [
							rect(0, 0, 8, 2),
							rect(16, 1, 8),
							rect(16, 0, 8),
							rect(8, 0, 8),
							rect(8, 1, 8),
						],
						line: (box: BoxOf) => ({
							vertical: true,
							x: box("e").right + 2,
							top: box("e").top,
							bottom: box("e").bottom,
						}),
					},
				},
				{
					// the line runs along the edge under c and d. It stops at a,
					// which crosses that edge.
					name: "e under c and moves b to the next row",
					input: { dragged: "e", point: ["c", 0.5, 0.9] },
					expected: {
						layout: [
							rect(0, 0, 8, 2),
							rect(8, 0, 8),
							rect(16, 0, 8),
							rect(0, 2, 8),
							rect(8, 1, 8),
						],
						line: (box: BoxOf) => ({
							vertical: false,
							y: box("c").bottom + 2,
							left: box("c").left,
							right: box("d").right,
						}),
					},
				},
			] as const)("puts $name", ({ input, expected }, { expect }) => {
				const editor = mountEditor(tallGrid(), "wide-grid")
				const box = (title: string) =>
					blockElement(editor, title).getBoundingClientRect()
				const [title, u, v] = input.point

				const drop = dropAt(
					editor,
					input.dragged,
					pointIn(blockElement(editor, title), u, v),
				)

				expect.soft(drop?.layout).toEqual(expected.layout)
				expect.soft(drop?.line).toEqual(expected.line(box))
			})

			it.for([
				{
					name: "b into the tall block's place from its top",
					input: { dragged: "b", point: ["a", 0.5, 0.05], line: "a" },
					expected: [rect(0, 1, 8, 2), rect(8, 0, 8), rect(0, 0, 8)],
				},
				{
					name: "b into c's place from c's top",
					input: { dragged: "b", point: ["c", 0.5, 0.1], line: "c" },
					expected: [rect(0, 0, 8, 2), rect(8, 1, 8), rect(8, 0, 8)],
				},
				{
					name: "the tall block under c from c's bottom",
					input: { dragged: "a", point: ["c", 0.5, 0.9], line: "c" },
					expected: [rect(0, 1, 8, 2), rect(0, 0, 8), rect(0, 3, 8)],
				},
				{
					name: "the tall block above b from b's top",
					input: { dragged: "a", point: ["b", 0.5, 0.1], line: "b" },
					expected: [rect(0, 1, 8, 2), rect(0, 0, 8), rect(0, 3, 8)],
				},
			] as const)(
				"puts $name and draws the line along that block only",
				({ input, expected }, { expect }) => {
					const editor = mountEditor(tallBesidePair(), "wide-grid")
					const [title, u, v] = input.point
					const lined = blockElement(editor, input.line).getBoundingClientRect()
					const top = v < 0.5

					const drop = dropAt(
						editor,
						input.dragged,
						pointIn(blockElement(editor, title), u, v),
					)

					// the line never crosses the tall block while it is dragged
					expect.soft(drop?.layout).toEqual(expected)
					expect.soft(drop?.line).toEqual({
						vertical: false,
						y: top ? lined.top - 2 : lined.bottom + 2,
						left: lined.left,
						right: lined.right,
					})
				},
			)

			it.for([
				{
					name: "c's top while b is dragged",
					input: {
						content: tallBesidePair,
						dragged: "b",
						target: "c",
						points: [0.4, 0.5, 0.6].flatMap((u) =>
							[0.02, 0.08, 0.14, 0.2, 0.26].map((v) => [u, v] as const),
						),
					},
				},
				{
					name: "the tall block's left side while c is dragged",
					input: {
						content: tallGrid,
						dragged: "c",
						target: "a",
						points: [0.02, 0.06].flatMap((u) =>
							[0.15, 0.3, 0.5, 0.7, 0.85].map((v) => [u, v] as const),
						),
					},
				},
			])("draws one line anywhere over $name", ({ input }, { expect }) => {
				const editor = mountEditor(input.content(), "wide-grid")
				const lines = new Set(
					input.points.map(([u, v]) =>
						JSON.stringify(
							dropAt(
								editor,
								input.dragged,
								pointIn(blockElement(editor, input.target), u, v),
							)?.line,
						),
					),
				)

				expect([...lines]).toHaveLength(1)
				expect(lines.has(JSON.stringify(undefined))).toBe(false)
			})
		})

		it("draws the line where a block lands when a tall block is in its way", ({
			expect,
		}) => {
			// t is tall and on the right. a sits above c on the left.
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("t", { widePosX: 12, widePosY: 0, height: 2 }),
						metricBlock("c", { widePosX: 0, widePosY: 1 }),
					),
					blockGrid(metricBlock("x")),
				],
				"wide-grid",
			)
			const tall = blockElement(editor, "t").getBoundingClientRect()
			const left = blockElement(editor, "c").getBoundingClientRect()

			const drop = dropAt(
				editor,
				"x",
				pointIn(blockElement(editor, "c"), 0.9, 0.5),
			)

			// the block has no room right of c, so it lands in the next row.
			// It slides to the row's start, under both c and t.
			expect
				.soft(drop?.layout)
				.toEqual([
					rect(0, 0, 12),
					rect(12, 0, 12, 2),
					rect(0, 1, 12),
					rect(0, 2, 12),
				])
			expect.soft(drop?.line).toEqual({
				vertical: false,
				y: left.bottom + 2,
				left: left.left,
				right: tall.right,
			})
		})

		it("fills the free cells beside a tall block and draws the line along that block", ({
			expect,
		}) => {
			// t is tall and on the left, and a is right of its first row. The
			// cells right of its second row are free.
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("t", { widePosX: 0, widePosY: 0, height: 2 }),
						metricBlock("a", { widePosX: 12, widePosY: 0 }),
					),
					blockGrid(metricBlock("n", { width: GridItemWidth.Compact })),
				],
				"wide-grid",
			)
			const tall = blockElement(editor, "t").getBoundingClientRect()

			// the point is right of t, in its second row
			const drop = dropAt(editor, "n", {
				x: tall.right + 40,
				y: tall.bottom - 20,
			})

			expect
				.soft(drop?.layout)
				.toEqual([rect(0, 0, 12, 2), rect(12, 0, 12), rect(12, 1, 8)])
			expect.soft(drop?.line).toEqual({
				vertical: true,
				x: tall.right + 2,
				top: tall.bottom - 40,
				bottom: tall.bottom,
			})
		})

		it("draws the line below a tall block only as wide as the block", ({
			expect,
		}) => {
			// t is tall and on the left, and a is right of its first row. The
			// cells right of its second row are free.
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("t", { widePosX: 0, widePosY: 0, height: 2 }),
						metricBlock("a", { widePosX: 12, widePosY: 0 }),
					),
					blockGrid(metricBlock("x")),
				],
				"wide-grid",
			)
			const tall = blockElement(editor, "t").getBoundingClientRect()

			const drop = dropAt(
				editor,
				"x",
				pointIn(blockElement(editor, "t"), 0.5, 0.95),
			)

			// the cells right of the line have no block right above them
			expect
				.soft(drop?.layout)
				.toEqual([rect(0, 0, 12, 2), rect(12, 0, 12), rect(0, 2, 12)])
			expect.soft(drop?.line).toEqual({
				vertical: false,
				y: tall.bottom + 2,
				left: tall.left,
				right: tall.right,
			})
		})

		it("puts a block into free cells at the grid's left edge and draws the line along the block above", ({
			expect,
		}) => {
			// a is on the left, and t is tall and right of it. The cells left
			// of t's second row are free.
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("t", { widePosX: 12, widePosY: 0, height: 2 }),
					),
					blockGrid(metricBlock("n", { width: GridItemWidth.Compact })),
				],
				"wide-grid",
			)
			const above = blockElement(editor, "a").getBoundingClientRect()

			const drop = dropAt(editor, "n", {
				x: above.left + 20,
				y: above.bottom + 24,
			})

			expect
				.soft(drop?.layout)
				.toEqual([rect(0, 0, 12), rect(12, 0, 12, 2), rect(0, 1, 8)])
			expect.soft(drop?.line).toEqual({
				vertical: false,
				y: above.bottom + 2,
				left: above.left,
				right: above.right,
			})
		})

		it("puts a block at a tall block's side into that block's first row", ({
			expect,
		}) => {
			// a is on the left, and t is tall and right of it
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("t", { widePosX: 12, widePosY: 0, height: 2 }),
					),
					blockGrid(metricBlock("n", { width: GridItemWidth.Compact })),
				],
				"wide-grid",
			)
			const left = blockElement(editor, "a").getBoundingClientRect()

			// the point is on t's left side, level with its second row
			const drop = dropAt(
				editor,
				"n",
				pointIn(blockElement(editor, "t"), 0.05, 0.75),
			)

			// t has no room right of n, so it moves down. The line follows n.
			expect
				.soft(drop?.layout)
				.toEqual([rect(0, 0, 12), rect(0, 1, 12, 2), rect(12, 0, 8)])
			expect.soft(drop?.line).toEqual({
				vertical: true,
				x: left.right + 2,
				top: left.top,
				bottom: left.bottom,
			})
		})

		it("counts the dragged block's own cells as free", ({ expect }) => {
			// d sits left of t's second row, and four free columns are
			// between them
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("t", { widePosX: 12, widePosY: 0, height: 2 }),
						metricBlock("d", { widePosX: 0, widePosY: 1, width: 8 }),
					),
				],
				"wide-grid",
			)
			const box = blockElement(editor, "d").getBoundingClientRect()

			// the point is in the free columns. With d's cells free, d fits
			// there again and nothing changes. Otherwise t would shrink.
			const drop = dropAt(editor, "d", {
				x: box.right + 20,
				y: box.top + 20,
			})

			expect(drop).toBeNull()
		})

		it.for([14, 22])(
			"finds nothing for a block alone in its row at column %i of that row",
			(column, { expect }) => {
				const editor = mountEditor(
					[
						blockGrid(
							metricBlock("a", { widePosX: 0, widePosY: 0, width: 8 }),
							metricBlock("b", { widePosX: 0, widePosY: 1, width: 24 }),
						),
					],
					"wide-grid",
				)
				const box = blockElement(editor, "a").getBoundingClientRect()

				// a column is 12px wide, with 4px between columns
				const drop = dropAt(editor, "a", {
					x: box.left + column * 16 + 6,
					y: box.top + 20,
				})

				expect(drop).toBeNull()
			},
		)

		it("picks the nearest block for a pointer in the gap between two", ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				"wide-grid",
			)
			const left = blockElement(editor, "a").getBoundingClientRect()
			const right = blockElement(editor, "b").getBoundingClientRect()

			// the point is slightly closer to a, at the height of its middle
			const drop = dropAt(editor, "x", {
				x: left.right + 1.5,
				y: left.top + 20,
			})

			expect(drop?.line).toEqual({
				vertical: true,
				x: right.left - 2,
				top: left.top,
				bottom: left.bottom,
			})
		})

		it("finds a drop in a grid saved under its former name", ({ expect }) => {
			const editor = mountEditor(
				[legacyGrid(metricBlock("a"), metricBlock("b"))],
				"wide-grid",
			)

			const drop = dropAt(
				editor,
				"b",
				pointIn(blockElement(editor, "a"), 0.1, 0.5),
			)

			expect(drop?.layout).toEqual([rect(12, 0, 12), rect(0, 0, 12)])
		})

		it.for([
			{
				name: "the dragged block itself",
				input: { target: "a", u: 0.5, v: 0.5 },
			},
			{
				name: "the left side of the block right of the dragged one",
				input: { target: "b", u: 0.1, v: 0.5 },
			},
		])("finds nothing over $name", ({ input }, { expect }) => {
			const editor = mountEditor(
				[blockGrid(metricBlock("a"), metricBlock("b"))],
				"wide-grid",
			)

			expect(
				dropAt(
					editor,
					"a",
					pointIn(blockElement(editor, input.target), input.u, input.v),
				),
			).toBeNull()
		})

		it("puts a block into a grid with no blocks at its first cell", ({
			expect,
		}) => {
			const editor = mountEditor(
				[blockGrid(), blockGrid(metricBlock("x"))],
				"wide-grid",
			)
			const box = editor.view.dom
				.querySelector('[data-type="block-grid"]')
				?.getBoundingClientRect()

			if (!box) {
				throw new Error("the editor renders the empty grid")
			}

			const drop = dropAt(editor, "x", {
				x: box.left + box.width / 2,
				y: box.top + box.height / 2,
			})

			expect.soft(drop?.layout).toEqual([rect(0, 0, 12)])
			expect.soft(drop?.line).toEqual({
				vertical: false,
				y: box.top,
				left: box.left,
				right: box.right,
			})
		})
	})

	describe("in a narrower grid", () => {
		it.for([
			{
				name: "right above a block from its upper half",
				input: { target: "b", v: 0.25 },
			},
			{
				name: "right below a block from its lower half",
				input: { target: "a", v: 0.75 },
			},
		])(
			"puts a block from outside the grid $name and gives it cells right of the block before it",
			({ input }, { expect }) => {
				const editor = mountEditor(
					[
						blockGrid(metricBlock("a"), metricBlock("b")),
						blockGrid(metricBlock("x")),
					],
					"narrow-grid",
				)
				const box = blockElement(editor, input.target).getBoundingClientRect()

				const drop = dropAt(
					editor,
					"x",
					pointIn(blockElement(editor, input.target), 0.5, input.v),
				)

				expect.soft(drop?.stack).toEqual([0, 2, 1])
				expect
					.soft(drop?.layout)
					.toEqual([rect(0, 0, 12), rect(0, 1, 12), rect(12, 0, 12)])
				expect.soft(drop?.line).toEqual({
					vertical: false,
					y: input.v < 0.5 ? box.top - 2 : box.bottom + 2,
					left: box.left,
					right: box.right,
				})
			},
		)

		it("moves a block of the grid in the stacked order and leaves its cells alone", ({
			expect,
		}) => {
			// a and b share the first row of the widest layout, and c is
			// below a
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("b", { widePosX: 12, widePosY: 0 }),
						metricBlock("c", { widePosX: 0, widePosY: 1 }),
					),
				],
				"narrow-grid",
			)

			const drop = dropAt(
				editor,
				"c",
				pointIn(blockElement(editor, "b"), 0.5, 0.25),
			)

			expect.soft(drop?.stack).toEqual([0, 2, 1])
			expect.soft(drop?.layout).toBeNull()
		})

		it.for([
			{
				name: "the lower half of the block above",
				input: { target: "a", v: 0.75 },
			},
			{
				name: "the upper half of the block below",
				input: { target: "c", v: 0.25 },
			},
			{
				name: "the dragged block itself",
				input: { target: "b", v: 0.5 },
			},
		])(
			"finds nothing next to the dragged block from $name",
			({ input }, { expect }) => {
				const editor = mountEditor(
					[blockGrid(metricBlock("a"), metricBlock("b"), metricBlock("c"))],
					"narrow-grid",
				)

				expect(
					dropAt(
						editor,
						"b",
						pointIn(blockElement(editor, input.target), 0.5, input.v),
					),
				).toBeNull()
			},
		)
	})

	it.for([
		{ input: 895, expected: false },
		{ input: 896, expected: true },
	])(
		"reads the columns of a grid $input pixels wide exactly when isWideGrid says $expected",
		({ input, expected }, { expect }) => {
			const editor = mountEditor(
				[
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				"breakpoint-grid",
				input,
			)
			const grid = editor.view.dom.querySelector('[data-type="block-grid"]')

			const drop = dropAt(
				editor,
				"x",
				pointIn(blockElement(editor, "a"), 0.5, 0.5),
			)

			expect
				.soft(isWideGrid(grid?.getBoundingClientRect().width ?? 0))
				.toBe(expected)
			expect.soft(drop?.stack === null).toBe(expected)
		},
	)
})

describe("executeGridDrop", { concurrent: false }, () => {
	afterEach(cleanUp)

	it.for([
		{
			name: "a line",
			input: {
				content: () => [
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				dragged: "x",
				target: { title: "b", u: 0.1, v: 0.5 },
				className: "wide-grid",
			},
			expected: {
				cells: ["a 0,0 12x1", "b 0,1 12x1", "x 12,0 12x1"],
				stack: ["a null", "b null", "x null"],
			},
		},
		{
			name: "the right side of the other block of its row",
			input: {
				content: () => [blockGrid(metricBlock("a"), metricBlock("b"))],
				dragged: "a",
				target: { title: "b", u: 0.9, v: 0.5 },
				className: "wide-grid",
			},
			expected: {
				cells: ["a 12,0 12x1", "b 0,0 12x1"],
				stack: ["a null", "b null"],
			},
		},
		{
			name: "a block's top side",
			input: {
				content: () => [
					blockGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
				],
				dragged: "c",
				target: { title: "a", u: 0.5, v: 0.1 },
				className: "wide-grid",
			},
			expected: {
				cells: ["a 0,1 12x1", "b 12,0 12x1", "c 0,0 12x1"],
				stack: ["a null", "b null", "c null"],
			},
		},
		{
			name: "free cells",
			input: {
				content: () => [
					blockGrid(
						metricBlock("t", { widePosX: 0, widePosY: 0, height: 2 }),
						metricBlock("a", { widePosX: 12, widePosY: 0 }),
					),
					blockGrid(metricBlock("x")),
				],
				dragged: "x",
				target: { title: "t", u: 1.2, v: 0.75 },
				className: "wide-grid",
			},
			expected: {
				cells: ["t 0,0 12x2", "a 12,0 12x1", "x 12,1 12x1"],
				stack: ["t null", "a null", "x null"],
			},
		},
		{
			name: "a place in the stacked order",
			input: {
				content: () => [
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				dragged: "x",
				target: { title: "b", u: 0.5, v: 0.25 },
				className: "narrow-grid",
			},
			expected: {
				cells: ["a 0,0 12x1", "b 0,1 12x1", "x 12,0 12x1"],
				stack: ["a 0", "b 2", "x 1"],
			},
		},
	])("writes a drop at $name", ({ input, expected }, { expect }) => {
		const editor = mountEditor(input.content(), input.className)
		const point = pointIn(
			blockElement(editor, input.target.title),
			input.target.u,
			input.target.v,
		)
		const drop = dropAt(editor, input.dragged, point)

		if (!drop) {
			throw new Error("the point holds a grid drop")
		}

		const executed = executeGridDrop(
			editor.view,
			drop,
			sliceOf(editor, input.dragged),
			true,
		)

		// a block from another grid leaves that grid empty, so it is deleted
		expect.soft(executed).toBe(true)
		expect.soft(editor.state.doc.childCount).toBe(1)
		expect.soft(storedCells(editor)).toEqual(expected.cells)
		expect.soft(storedStackPlaces(editor)).toEqual(expected.stack)
	})

	it("leaves the blocks whose cells stay the same untouched", ({ expect }) => {
		// c moves from the left of d to its right, in the second row
		const editor = mountEditor(
			[
				blockGrid(
					metricBlock("a", { widePosX: 0, widePosY: 0 }),
					metricBlock("b", { widePosX: 12, widePosY: 0 }),
					metricBlock("c", { widePosX: 0, widePosY: 1, width: 8 }),
					metricBlock("d", { widePosX: 8, widePosY: 1, width: 8 }),
				),
			],
			"wide-grid",
		)
		const [a, b] = editor.state.doc.child(0).children
		const drop = dropAt(
			editor,
			"c",
			pointIn(blockElement(editor, "d"), 0.9, 0.5),
		)

		if (!drop) {
			throw new Error("the point holds a grid drop")
		}

		executeGridDrop(editor.view, drop, sliceOf(editor, "c"), true)

		// a node view only renders again for a node that changed
		const grid = editor.state.doc.child(0)
		expect
			.soft(storedCells(editor))
			.toEqual(["a 0,0 12x1", "b 12,0 12x1", "c 8,1 8x1", "d 0,1 8x1"])
		expect.soft(grid.child(0)).toBe(a)
		expect.soft(grid.child(1)).toBe(b)
	})

	it("moves a block in from another grid without its old stacked place", ({
		expect,
	}) => {
		const editor = mountEditor(
			[
				blockGrid(metricBlock("a")),
				blockGrid(metricBlock("x", { stackPos: 3 })),
			],
			"wide-grid",
		)
		const drop = dropAt(
			editor,
			"x",
			pointIn(blockElement(editor, "a"), 0.9, 0.5),
		)

		if (!drop) {
			throw new Error("the point holds a grid drop")
		}

		executeGridDrop(editor.view, drop, sliceOf(editor, "x"), true)

		expect.soft(editor.state.doc.childCount).toBe(1)
		expect.soft(storedStackPlaces(editor)).toEqual(["a null", "x null"])
	})

	it("copies a block as a new one without its uid and its comment", ({
		expect,
	}) => {
		const editor = mountEditor(
			[
				blockGrid(
					metricBlock("a", { uid: "a-uid", nodeCommentId: "a-comment" }),
					metricBlock("b", { uid: "b-uid" }),
				),
			],
			"wide-grid",
		)
		const point = pointIn(blockElement(editor, "b"), 0.5, 0.9)
		select(editor, "a")
		const slice = sliceOf(editor, "a")
		const drop = findGridDrop(editor.view, point.x, point.y, slice, false)

		if (!drop) {
			throw new Error("the point holds a grid drop")
		}

		executeGridDrop(editor.view, drop, slice, false)

		expect(
			editor.state.doc
				.child(0)
				.children.map((child): unknown[] => [
					child.attrs.title,
					child.attrs.uid,
					child.attrs.nodeCommentId,
				]),
		).toEqual([
			["a", "a-uid", "a-comment"],
			["b", "b-uid", null],
			["a", null, null],
		])
	})

	it("writes nothing once the document changed since the drop was found", ({
		expect,
	}) => {
		const editor = mountEditor(
			[blockGrid(metricBlock("a"), metricBlock("b"))],
			"wide-grid",
		)
		const drop = dropAt(
			editor,
			"b",
			pointIn(blockElement(editor, "a"), 0.1, 0.5),
		)

		if (!drop) {
			throw new Error("the point holds a grid drop")
		}

		editor.commands.insertContentAt(0, paragraph("new"))
		const doc = editor.state.doc

		expect
			.soft(executeGridDrop(editor.view, drop, sliceOf(editor, "b"), true))
			.toBe(false)
		expect.soft(editor.state.doc).toBe(doc)
	})
})

describe("gridDropLineStyle", () => {
	it.for([
		{
			name: "a vertical line",
			input: { vertical: true, x: 110, top: 70, bottom: 110 } as const,
			expected: {
				top: "50px",
				left: "calc(100px - 0.09375rem)",
				width: "0.1875rem",
				height: "40px",
				display: "block",
			},
		},
		{
			name: "a horizontal line",
			input: { vertical: false, y: 70, left: 110, right: 250 } as const,
			expected: {
				top: "calc(50px - 0.09375rem)",
				left: "100px",
				width: "140px",
				height: "0.1875rem",
				display: "block",
			},
		},
	])("centres the cursor on $name", ({ input, expected }, { expect }) => {
		expect(gridDropLineStyle(input, new DOMRect(10, 20, 500, 500))).toEqual(
			expected,
		)
	})
})

type BoxOf = (title: string) => DOMRect

// a grid of three columns: a is tall on the left, c and d sit right of it,
// and b and e sit under c and d
function tallGrid(): JSONContent[] {
	return [
		blockGrid(
			metricBlock("a", { widePosX: 0, widePosY: 0, width: 8, height: 2 }),
			metricBlock("c", { widePosX: 8, widePosY: 0, width: 8 }),
			metricBlock("d", { widePosX: 16, widePosY: 0, width: 8 }),
			metricBlock("b", { widePosX: 8, widePosY: 1, width: 8 }),
			metricBlock("e", { widePosX: 16, widePosY: 1, width: 8 }),
		),
	]
}

// a grid of three columns: a is tall on the left, c sits right of it and b
// under c. The third column is free.
function tallBesidePair(): JSONContent[] {
	return [
		blockGrid(
			metricBlock("a", { widePosX: 0, widePosY: 0, width: 8, height: 2 }),
			metricBlock("c", { widePosX: 8, widePosY: 0, width: 8 }),
			metricBlock("b", { widePosX: 8, widePosY: 1, width: 8 }),
		),
	]
}

// mounts an editor in a container of the given class and width, so the
// grid styles lay it out
function mountEditor(
	content: JSONContent[],
	className = "",
	width = 400,
): Editor {
	const container = document.createElement("div")
	container.className = className
	container.style.width = `${width}px`
	container.style.marginTop = "40px"
	container.style.position = "relative"
	document.body.appendChild(container)

	const editor = new Editor({
		element: container,
		extensions: [
			ContentDocument,
			Paragraph,
			Text,
			RenderedBlockGrid,
			RenderedLegacyMetricGrid,
			TitledMetricBlock,
			GridLayout,
		],
		content: { type: "doc", content },
	})

	cleanups.push(() => {
		editor.destroy()
		container.remove()
	})

	return editor
}

// selects the block, as the drag handle does for the block it drags
function select(editor: Editor, title: string) {
	const { view } = editor

	view.dispatch(
		view.state.tr.setSelection(
			NodeSelection.create(view.state.doc, posOfMetricBlock(editor, title)),
		),
	)
}

function sliceOf(editor: Editor, title: string): Slice {
	const node = editor.state.doc.nodeAt(posOfMetricBlock(editor, title))

	if (!node) {
		throw new Error(`no metric block titled "${title}"`)
	}

	return new Slice(Fragment.from(node), 0, 0)
}

// finds the drop at the point when the drag handle moves the block
function dropAt(
	editor: Editor,
	title: string,
	point: { x: number; y: number },
): GridDrop | null | undefined {
	select(editor, title)

	return findGridDrop(
		editor.view,
		point.x,
		point.y,
		sliceOf(editor, title),
		true,
	)
}

// the line that a drop at a side of the box draws. It sits half a gap away
// from the box.
function sideLine(box: DOMRect, side: "left" | "top") {
	return side === "left"
		? { vertical: true, x: box.left - 2, top: box.top, bottom: box.bottom }
		: { vertical: false, y: box.top - 2, left: box.left, right: box.right }
}

function rect(x: number, y: number, w: number, h = 1) {
	return { x: x, y: y, w: w, h: h }
}

function cleanUp() {
	for (const cleanup of cleanups.splice(0)) {
		cleanup()
	}
}
