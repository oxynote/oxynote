import {
	combineTransactionSteps,
	Extension,
	getSchema,
	type JSONContent,
	Node,
} from "@tiptap/core"
import Document from "@tiptap/extension-document"
import { ContentDocument } from "../document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import {
	Fragment,
	type Node as PMNode,
	type Schema,
	Slice,
} from "@tiptap/pm/model"
import {
	EditorState,
	NodeSelection,
	type Plugin,
	type Transaction,
	TextSelection,
} from "@tiptap/pm/state"
import type { Decoration, DecorationSet } from "@tiptap/pm/view"
import { ySyncPluginKey } from "@tiptap/y-tiptap"
import { beforeEach, describe, it, vi } from "vitest"
import {
	BLOCK_GRID_NAME,
	GRID_ITEM_GROUP,
	LEGACY_METRIC_GRID_NAME,
	METRIC_BLOCK_NAME,
} from "../node-names"
import { runCommand } from "../../comments/test-helpers"
import { layoutGrid } from "@oxynote/common/grid"
import {
	GridMetricBlockStub,
	MetricBlockStub,
	nodeType,
} from "~/components/editor/test-helpers"
import {
	BlockGrid,
	deleteEmptyBlockGrids,
	insertGridItemCommand,
	LegacyMetricGrid,
	setUpGridItem,
	withoutGridPlaces,
} from "."

// the migration replays a transaction's steps only to find what it
// changed. A spy shows when it does.
vi.mock("@tiptap/core", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tiptap/core")>()

	return {
		...actual,
		combineTransactionSteps: vi.fn(actual.combineTransactionSteps),
	}
})

// a spy shows which grids the layout plugin lays out
vi.mock("@oxynote/common/grid", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@oxynote/common/grid")>()

	return {
		...actual,
		layoutGrid: vi.fn(actual.layoutGrid),
	}
})

// a nesting context standing in for the real containers that accept a
// bare grid item (e.g. the split documentation right side)
const Container = Node.create({
	name: "container",
	group: "block",
	content: `(paragraph | ${METRIC_BLOCK_NAME})+`,
})

// the uid every block carries in the editor, which a converted grid keeps
const Uid = Extension.create({
	name: "uid",
	addGlobalAttributes() {
		return [
			{
				types: [BLOCK_GRID_NAME, LEGACY_METRIC_GRID_NAME, METRIC_BLOCK_NAME],
				attributes: { uid: { default: null } },
			},
		]
	},
})

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	Container,
	BlockGrid,
	LegacyMetricGrid,
	MetricBlockStub,
	Uid,
])

const placedSchema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGrid,
	GridMetricBlockStub,
])

// a schema that knows paragraphs but no grid node type
const schemaWithoutGrids = getSchema([Document, Paragraph, Text])

describe("BlockGrid", () => {
	it("accepts zero grid items so emptying operations stay valid", ({
		expect,
	}) => {
		expect(BlockGrid.config.content).toBe(`${GRID_ITEM_GROUP}*`)
		expect(nodeType(schema, BLOCK_GRID_NAME).createAndFill()).not.toBeNull()
	})

	it("parses both the block grid and the legacy grid marker", ({ expect }) => {
		expect(BlockGrid.config.parseHTML?.call({} as never)).toEqual([
			{ tag: `div[data-type="block-grid"]` },
			{ tag: `div[data-type="metric-grid"]` },
		])
	})

	it("renders a div carrying the grid marker alongside the given attributes", ({
		expect,
	}) => {
		expect(
			BlockGrid.config.renderHTML?.call({} as never, {
				HTMLAttributes: { class: "custom" },
				node: {} as never,
			}),
		).toEqual(["div", { class: "custom", "data-type": "block-grid" }, 0])
	})
})

describe("gridLayout", () => {
	it("hands every block of a grid its cells and its place in reading order", ({
		expect,
	}) => {
		// the second block is tall and sits on the left in the wide layout,
		// so it reads first
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
			nodeType(placedSchema, BLOCK_GRID_NAME).create(null, [
				placed({ widePosX: 12, widePosY: 0, width: 12, height: 1 }),
				placed({ widePosX: 0, widePosY: 0, width: 12, height: 2 }),
				placed({ widePosX: 0, widePosY: 2, width: 24, height: 1 }),
			]),
		])

		expect(cellStyles(layoutDecorations(doc))).toEqual([
			{
				from: 3,
				style:
					"--grid-col:13;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:1",
			},
			{
				from: 4,
				style:
					"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:2;--grid-order:0",
			},
			{
				from: 5,
				style:
					"--grid-col:1;--grid-row:3;--grid-w:24;--grid-h:1;--grid-order:2",
			},
		])
	})

	it("lays out a block saved before blocks stored a place by its named width", ({
		expect,
	}) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, BLOCK_GRID_NAME).create(null, [
				placed({ width: "compact" }),
				placed({ width: "wide" }),
			]),
		])

		expect(cellStyles(layoutDecorations(doc))).toEqual([
			{
				from: 1,
				style: "--grid-col:1;--grid-row:1;--grid-w:8;--grid-h:1;--grid-order:0",
			},
			{
				from: 2,
				style:
					"--grid-col:1;--grid-row:2;--grid-w:24;--grid-h:1;--grid-order:1",
			},
		])
	})

	it("orders the stacked layout by the blocks' places", ({ expect }) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			placedGrid(placed({ stackPos: 1 }), placed({ stackPos: 0 })),
		])

		expect(
			cellStyles(layoutDecorations(doc)).map(({ style }) =>
				style.replace(/.*--grid-order:/, ""),
			),
		).toEqual(["1", "0"])
	})

	it("decorates nothing outside a grid", ({ expect }) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
		])

		expect(layoutDecorations(doc)).toEqual([])
	})

	it("moves the decorations along with an edit outside every grid", ({
		expect,
	}) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
			nodeType(placedSchema, BLOCK_GRID_NAME).create(null, [placed({})]),
		])
		const plugin = gridPlugins(true)[1]
		const state = EditorState.create({
			doc: doc,
			plugins: plugin ? [plugin] : [],
		})
		const typed = state.apply(state.tr.insertText("ab", 1))

		expect(
			cellStyles(
				(plugin?.getState(typed) as DecorationSet | undefined)?.find() ?? [],
			),
		).toEqual([
			{
				from: 5,
				style:
					"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
			},
		])
	})

	it("lays the grids out again after an edit and keeps them when nothing changed", ({
		expect,
	}) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, BLOCK_GRID_NAME).create(null, [placed({})]),
		])
		const plugin = gridPlugins(true)[1]
		const state = EditorState.create({
			doc: doc,
			plugins: plugin ? [plugin] : [],
		})
		const kept = state.apply(state.tr.setMeta("other", true))
		const edited = state.apply(
			state.tr.setNodeMarkup(1, null, { widePosX: 12, widePosY: 0 }),
		)

		expect(plugin?.getState(kept) as unknown).toBe(plugin?.getState(state))
		expect(
			cellStyles(
				(plugin?.getState(edited) as DecorationSet | undefined)?.find() ?? [],
			),
		).toEqual([
			{
				from: 1,
				style:
					"--grid-col:13;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
			},
		])
	})

	it("keeps the decorations of a grid deleted and inserted again", ({
		expect,
	}) => {
		const grid = placedGrid(placed({}), placed({ width: 8 }))
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
			grid,
		])

		const moved = applyLayoutEdit(doc, (tr) =>
			tr.delete(2, 2 + grid.nodeSize).insert(0, grid),
		)

		expect(cellStyles(moved.find())).toEqual([
			{
				from: 1,
				style:
					"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
			},
			{
				from: 2,
				style:
					"--grid-col:13;--grid-row:1;--grid-w:8;--grid-h:1;--grid-order:1",
			},
		])
	})

	it("moves the decorations of a grid after a change in the grid before it", ({
		expect,
	}) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			placedGrid(placed({})),
			placedGrid(placed({ width: 8 })),
		])

		const edited = applyLayoutEdit(doc, (tr) => tr.insert(2, placed({})))

		expect(cellStyles(edited.find())).toEqual([
			{
				from: 1,
				style:
					"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
			},
			{
				from: 2,
				style:
					"--grid-col:13;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:1",
			},
			{
				from: 5,
				style: "--grid-col:1;--grid-row:1;--grid-w:8;--grid-h:1;--grid-order:0",
			},
		])
	})

	it.for([
		{
			name: "inserted next to",
			input: {
				grids: 1,
				edit: (tr: Transaction) => tr.insert(3, oneBlockGrid()),
			},
			expected: [1, 4],
		},
		{
			name: "deleted next to",
			input: { grids: 2, edit: (tr: Transaction) => tr.delete(0, 3) },
			expected: [1],
		},
	])(
		"lays out every grid once a grid is $name one just like it",
		({ input, expected }, { expect }) => {
			const doc = nodeType(placedSchema, "doc").create(
				null,
				Array.from({ length: input.grids }, oneBlockGrid),
			)

			const edited = applyLayoutEdit(doc, input.edit)

			expect(cellStyles(edited.find())).toEqual(
				expected.map((from) => ({
					from: from,
					style:
						"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
				})),
			)
		},
	)

	describe(
		"when a remote change replaces every node",
		{ concurrent: false },
		() => {
			// these tests count the calls to the mocked layoutGrid, so they run
			// one at a time
			beforeEach(() => {
				vi.mocked(layoutGrid).mockClear()
			})

			it("keeps the decorations and lays out nothing for the same content", ({
				expect,
			}) => {
				const doc = twoGrids()
				const plugin = layoutPlugin()
				const state = EditorState.create({ doc: doc, plugins: [plugin] })
				vi.mocked(layoutGrid).mockClear()

				const next = state.apply(
					state.tr.replaceWith(
						0,
						doc.content.size,
						placedSchema.nodeFromJSON(doc.toJSON()).content,
					),
				)

				expect.soft(plugin.getState(next)).toBe(plugin.getState(state))
				expect.soft(vi.mocked(layoutGrid)).not.toHaveBeenCalled()
			})

			it("lays out only the grid it changes", ({ expect }) => {
				const doc = twoGrids()
				const plugin = layoutPlugin()
				const state = EditorState.create({ doc: doc, plugins: [plugin] })
				const json = doc.toJSON() as JSONContent
				const changed = json.content?.[2]?.content?.[0]

				if (!changed) {
					throw new Error("the second grid holds a block")
				}

				changed.attrs = { ...changed.attrs, width: 24 }
				vi.mocked(layoutGrid).mockClear()

				const next = state.apply(
					state.tr.replaceWith(
						0,
						doc.content.size,
						placedSchema.nodeFromJSON(json).content,
					),
				)

				expect.soft(vi.mocked(layoutGrid)).toHaveBeenCalledOnce()
				expect.soft(cellStyles(plugin.getState(next)?.find() ?? [])).toEqual([
					{
						from: 1,
						style:
							"--grid-col:1;--grid-row:1;--grid-w:12;--grid-h:1;--grid-order:0",
					},
					{
						from: 6,
						style:
							"--grid-col:1;--grid-row:1;--grid-w:24;--grid-h:1;--grid-order:0",
					},
				])
			})
		},
	)
})

describe("grid paste", () => {
	it.for([
		{
			name: "keeps the cells of a grid pasted outside a grid",
			input: (doc: PMNode) => TextSelection.create(doc, 1),
			expected: [{ widePosX: 12, widePosY: 3, stackPos: null }],
		},
		{
			name: "drops the cells of a grid pasted onto a block of another grid",
			input: (doc: PMNode) => NodeSelection.create(doc, 3),
			expected: [{ widePosX: null, widePosY: null, stackPos: null }],
		},
	])("$name", ({ input, expected }, { expect }) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
			placedGrid(placed({})),
		])
		const plugin = gridPlugins(true)[1]
		const state = EditorState.create({ doc: doc, selection: input(doc) })
		const slice = new Slice(
			Fragment.from(
				placedGrid(placed({ widePosX: 12, widePosY: 3, width: 8, height: 1 })),
			),
			0,
			0,
		)
		const pasted = plugin?.props.transformPasted?.call(
			plugin,
			slice,
			{ state: state } as never,
			false,
		)

		expect(pasted && cellsOf(pasted.content)).toEqual(expected)
	})

	it("adds the blocks of a whole grid pasted onto a block of a grid to that grid", ({
		expect,
	}) => {
		const doc = nodeType(placedSchema, "doc").create(null, [
			nodeType(placedSchema, "paragraph").create(),
			placedGrid(placed({}), placed({})),
		])
		const plugin = layoutPlugin()
		const state = EditorState.create({
			doc: doc,
			selection: NodeSelection.create(doc, 3),
		})
		const slice = new Slice(
			Fragment.from(
				placedGrid(placed({ widePosX: 12, widePosY: 3 }), placed({})),
			),
			0,
			0,
		)
		const pasted = plugin.props.transformPasted?.call(
			plugin,
			slice,
			{ state: state } as never,
			false,
		)

		if (!pasted) {
			throw new Error("the plugin transforms pasted content")
		}

		const next = state.apply(state.tr.replaceSelection(pasted))

		expect(shape(next.doc)).toEqual(["paragraph", `${BLOCK_GRID_NAME}(3)`])
		expect(cellsOf(next.doc.content)).toEqual([
			{ widePosX: null, widePosY: null, stackPos: null },
			{ widePosX: null, widePosY: null, stackPos: null },
			{ widePosX: null, widePosY: null, stackPos: null },
		])
	})
})

describe("LegacyMetricGrid", () => {
	it("keeps the stored legacy name and the grid's content", ({ expect }) => {
		expect(LegacyMetricGrid.name).toBe(LEGACY_METRIC_GRID_NAME)
		expect(nodeType(schema, LEGACY_METRIC_GRID_NAME).spec.content).toBe(
			`${GRID_ITEM_GROUP}*`,
		)
	})

	it("parses no HTML, so pasted grids become block grids", ({ expect }) => {
		expect(LegacyMetricGrid.config.parseHTML?.call({} as never)).toEqual([])
	})

	it("renders as a block grid", ({ expect }) => {
		expect(
			LegacyMetricGrid.config.renderHTML?.call({} as never, {
				HTMLAttributes: {},
				node: {} as never,
			}),
		).toEqual(["div", { "data-type": "block-grid" }, 0])
	})

	it("adds no plugin of its own", ({ expect }) => {
		expect(
			LegacyMetricGrid.config.addProseMirrorPlugins?.call({} as never),
		).toEqual([])
	})
})

describe("deleteEmptyBlockGrids", () => {
	it.for([BLOCK_GRID_NAME, LEGACY_METRIC_GRID_NAME])(
		"deletes an empty %s",
		(name, { expect }) => {
			const state = EditorState.create({
				doc: docOf([para("before"), node(name), para("after")]),
			})

			const tr = deleteEmptyBlockGrids(state.tr)

			expect(shape(tr.doc)).toEqual(["paragraph", "paragraph"])
		},
	)

	it("keeps a grid that still holds a block", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([grid(metricBlock())]),
		})

		const tr = deleteEmptyBlockGrids(state.tr)

		expect(shape(tr.doc)).toEqual([`${BLOCK_GRID_NAME}(1)`])
		expect(tr.steps).toHaveLength(0)
	})

	it("deletes every empty grid in one transaction, keeping the filled ones", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([
				grid(),
				para("a"),
				grid(metricBlock()),
				legacyGrid(),
				para("b"),
				grid(),
			]),
		})

		const tr = deleteEmptyBlockGrids(state.tr)

		expect(shape(tr.doc)).toEqual([
			"paragraph",
			`${BLOCK_GRID_NAME}(1)`,
			"paragraph",
		])
		expect(tr.steps).toHaveLength(3)
	})

	it("leaves a document without any grid untouched", ({ expect }) => {
		const state = EditorState.create({ doc: docOf([para("a")]) })

		const tr = deleteEmptyBlockGrids(state.tr)

		expect(tr.steps).toHaveLength(0)
	})
})

describe("setUpGridItem", () => {
	it("wraps the block in a new block grid at the document root", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([para("before"), para("here"), para("after")]),
		})
		const tr = state.tr

		const res = setUpGridItem(state, tr, 11, METRIC_BLOCK_NAME)

		expect(res).toBe(true)
		expect(shape(tr.doc)).toEqual([
			"paragraph",
			`${BLOCK_GRID_NAME}(1)`,
			"paragraph",
		])
	})

	it("inserts a bare block when the paragraph is nested", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([container(para("a"), para("b"))]),
		})
		const tr = state.tr

		const res = setUpGridItem(state, tr, 2, METRIC_BLOCK_NAME)

		expect(res).toBe(true)
		expect(shape(tr.doc.child(0))).toEqual([METRIC_BLOCK_NAME, "paragraph"])
	})

	it("replaces the paragraph when it is the only block of its parent", ({
		expect,
	}) => {
		const state = EditorState.create({ doc: docOf([para("a")]) })
		const tr = state.tr

		const res = setUpGridItem(state, tr, 1, METRIC_BLOCK_NAME)

		expect(res).toBe(true)
		expect(shape(tr.doc)).toEqual([`${BLOCK_GRID_NAME}(1)`])
	})

	it("replaces the only paragraph of a nested parent", ({ expect }) => {
		const state = EditorState.create({ doc: docOf([container(para("a"))]) })
		const tr = state.tr

		const res = setUpGridItem(state, tr, 2, METRIC_BLOCK_NAME)

		expect(res).toBe(true)
		expect(shape(tr.doc.child(0))).toEqual([METRIC_BLOCK_NAME])
	})

	it("returns false when the position is not inside a paragraph", ({
		expect,
	}) => {
		const state = EditorState.create({ doc: docOf([container(para("a"))]) })
		const tr = state.tr

		const res = setUpGridItem(state, tr, 1, METRIC_BLOCK_NAME)

		expect(res).toBe(false)
		expect(tr.steps).toHaveLength(0)
	})

	it("returns false when the schema does not declare the block", ({
		expect,
	}) => {
		const state = EditorState.create({ doc: docOf([para("a")]) })
		const tr = state.tr

		const res = setUpGridItem(state, tr, 1, "unknownBlock")

		expect(res).toBe(false)
		expect(tr.steps).toHaveLength(0)
	})

	it("returns false when the schema declares no grid", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([para("a", schemaWithoutGrids)], schemaWithoutGrids),
		})
		const tr = state.tr

		const res = setUpGridItem(state, tr, 1, METRIC_BLOCK_NAME)

		expect(res).toBe(false)
		expect(tr.steps).toHaveLength(0)
	})
})

describe("insertGridItemCommand", () => {
	it("replaces the paragraph with a grid holding one block", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([para("a"), para("b")]),
		})

		const res = runCommand(insertGridItemCommand(METRIC_BLOCK_NAME)(1), state)

		expect(res.result).toBe(true)
		expect(shape(res.state.doc)).toEqual([`${BLOCK_GRID_NAME}(1)`, "paragraph"])
	})

	it("returns false when the position is not inside a paragraph", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([container(para("a"))]),
		})

		const res = runCommand(insertGridItemCommand(METRIC_BLOCK_NAME)(1), state)

		expect(res.result).toBe(false)
		expect(res.tr.steps).toHaveLength(0)
	})

	it("reports success without dispatching when only asked whether it can run", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([para("a"), para("b")]),
		})

		const res = runCommand(insertGridItemCommand(METRIC_BLOCK_NAME)(1), state, {
			dispatch: false,
		})

		expect(res.result).toBe(true)
		expect(res.tr.steps).toHaveLength(0)
		expect(shape(res.state.doc)).toEqual(["paragraph", "paragraph"])
	})
})

describe("legacy grid migration", () => {
	it("turns a legacy grid into a block grid when a local edit changes a block in it", ({
		expect,
	}) => {
		// para("a") spans 0-3, the grid starts at 3, its blocks at 4 and 5
		const doc = docOf([
			para("a"),
			legacyGrid(metricBlock("m1"), metricBlock("m2")),
		])

		const res = applyEdit(doc, (tr) =>
			tr.setNodeMarkup(4, null, { uid: "m1-resized" }),
		)

		expect.soft(res.transactions).toHaveLength(2)
		expect
			.soft(shape(res.state.doc))
			.toEqual(["paragraph", `${BLOCK_GRID_NAME}(2)`])
		expect.soft(res.state.doc.child(1).attrs).toEqual({ uid: "grid" })
		expect.soft(childUids(res.state.doc.child(1))).toEqual(["m1-resized", "m2"])
	})

	it("turns a legacy grid into a block grid when a local edit removes a block from it", ({
		expect,
	}) => {
		const doc = docOf([
			para("a"),
			legacyGrid(metricBlock("m1"), metricBlock("m2")),
		])

		const res = applyEdit(doc, (tr) => tr.delete(5, 6))

		expect.soft(res.transactions).toHaveLength(2)
		expect
			.soft(shape(res.state.doc))
			.toEqual(["paragraph", `${BLOCK_GRID_NAME}(1)`])
	})

	it("turns every legacy grid one transaction touches", ({ expect }) => {
		// the first grid spans 0-3, the second 3-6
		const doc = docOf([
			legacyGrid(metricBlock("m1")),
			legacyGrid(metricBlock("m2")),
		])

		const res = applyEdit(doc, (tr) =>
			tr.insert(5, metricBlock("m3")).insert(2, metricBlock("m4")),
		)

		expect.soft(res.transactions).toHaveLength(2)
		expect
			.soft(shape(res.state.doc))
			.toEqual([`${BLOCK_GRID_NAME}(2)`, `${BLOCK_GRID_NAME}(2)`])
	})

	it("leaves a legacy grid alone when a local edit lands next to it", ({
		expect,
	}) => {
		const doc = docOf([para("a"), legacyGrid(metricBlock("m1"))])

		const res = applyEdit(doc, (tr) => tr.insert(3, para("b")))

		expect.soft(res.transactions).toHaveLength(1)
		expect
			.soft(shape(res.state.doc))
			.toEqual(["paragraph", "paragraph", `${LEGACY_METRIC_GRID_NAME}(1)`])
	})

	it("leaves a legacy grid alone when the edit comes from another client", ({
		expect,
	}) => {
		const doc = docOf([para("a"), legacyGrid(metricBlock("m1"))])

		const res = applyEdit(
			doc,
			(tr) => tr.setNodeMarkup(4, null, { uid: "m1-resized" }),
			{ remote: true },
		)

		expect.soft(res.transactions).toHaveLength(1)
		expect
			.soft(shape(res.state.doc))
			.toEqual(["paragraph", `${LEGACY_METRIC_GRID_NAME}(1)`])
	})

	it("leaves a legacy grid alone while the editor is read-only", ({
		expect,
	}) => {
		const doc = docOf([para("a"), legacyGrid(metricBlock("m1"))])

		const res = applyEdit(
			doc,
			(tr) => tr.setNodeMarkup(4, null, { uid: "m1-resized" }),
			{ isEditable: false },
		)

		expect.soft(res.transactions).toHaveLength(1)
		expect
			.soft(shape(res.state.doc))
			.toEqual(["paragraph", `${LEGACY_METRIC_GRID_NAME}(1)`])
	})

	it("leaves the document alone when the transaction changes no content", ({
		expect,
	}) => {
		const doc = docOf([legacyGrid(metricBlock("m1"))])

		const res = applyEdit(doc, (tr) => tr.setMeta("unrelated", true))

		expect.soft(res.transactions).toHaveLength(1)
		expect.soft(shape(res.state.doc)).toEqual([`${LEGACY_METRIC_GRID_NAME}(1)`])
	})

	it("knows a document holds a legacy grid from the start", ({ expect }) => {
		const state = migrationState(
			docOf([grid(metricBlock("m1")), legacyGrid(metricBlock("m2"))]),
		)

		expect(holdsLegacyGrid(state)).toBe(true)
	})

	it("knows a document without a legacy grid holds none", ({ expect }) => {
		const state = migrationState(docOf([para("a"), grid(metricBlock("m1"))]))

		expect(holdsLegacyGrid(state)).toBe(false)
	})

	it("finds a legacy grid a remote change brings in", ({ expect }) => {
		const res = applyEdit(
			docOf([para("a")]),
			(tr) => tr.insert(3, legacyGrid(metricBlock("m1"))),
			{ remote: true },
		)

		expect(holdsLegacyGrid(res.state)).toBe(true)
	})

	it("finds no legacy grid in a remote change that brings in none", ({
		expect,
	}) => {
		const res = applyEdit(
			docOf([para("a")]),
			(tr) => tr.insert(3, grid(metricBlock("m1"))),
			{ remote: true },
		)

		expect(holdsLegacyGrid(res.state)).toBe(false)
	})

	it("stops looking once it converts the last legacy grid", ({ expect }) => {
		const doc = docOf([para("a"), legacyGrid(metricBlock("m1"))])

		const res = applyEdit(doc, (tr) =>
			tr.setNodeMarkup(4, null, { uid: "m1-resized" }),
		)

		expect(holdsLegacyGrid(res.state)).toBe(false)
	})

	it("stops looking when a local edit removes the last legacy grid", ({
		expect,
	}) => {
		// the grid spans 3-6
		const res = applyEdit(
			docOf([para("a"), legacyGrid(metricBlock("m1"))]),
			(tr) => tr.delete(3, 6),
		)

		expect(holdsLegacyGrid(res.state)).toBe(false)
	})

	it("keeps looking while a local edit leaves a legacy grid", ({ expect }) => {
		const res = applyEdit(
			docOf([para("a"), legacyGrid(metricBlock("m1"))]),
			(tr) => tr.insertText("b", 1),
		)

		expect(holdsLegacyGrid(res.state)).toBe(true)
	})

	it("keeps looking while a remote change leaves a legacy grid", ({
		expect,
	}) => {
		const res = applyEdit(
			docOf([para("a"), legacyGrid(metricBlock("m1"))]),
			(tr) => tr.insertText("b", 1),
			{ remote: true },
		)

		expect(holdsLegacyGrid(res.state)).toBe(true)
	})

	it("stops looking when a remote change removes the last legacy grid", ({
		expect,
	}) => {
		// the grid spans 3-6
		const res = applyEdit(
			docOf([para("a"), legacyGrid(metricBlock("m1"))]),
			(tr) => tr.delete(3, 6),
			{ remote: true },
		)

		expect(holdsLegacyGrid(res.state)).toBe(false)
	})

	describe(
		"when the document holds no legacy grid",
		{ concurrent: false },
		() => {
			// call accounting on the mocked module function needs these tests to
			// run one at a time
			beforeEach(() => {
				vi.mocked(combineTransactionSteps).mockClear()
			})

			it("replays no steps for a local edit", ({ expect }) => {
				const doc = docOf([para("a"), grid(metricBlock("m1"))])

				const res = applyEdit(doc, (tr) =>
					tr.setNodeMarkup(4, null, { uid: "m1-resized" }),
				)

				expect.soft(vi.mocked(combineTransactionSteps)).not.toHaveBeenCalled()
				expect.soft(res.transactions).toHaveLength(1)
				expect
					.soft(shape(res.state.doc))
					.toEqual(["paragraph", `${BLOCK_GRID_NAME}(1)`])
			})

			it("replays steps for a local edit while one is left", ({ expect }) => {
				const doc = docOf([para("a"), legacyGrid(metricBlock("m1"))])

				applyEdit(doc, (tr) => tr.insertText("b", 1))

				expect(vi.mocked(combineTransactionSteps)).toHaveBeenCalledTimes(1)
			})
		},
	)
})

describe("withoutGridPlaces", () => {
	it.for([
		{
			name: "drops the places of a grid block on its own",
			input: {
				content: () => [
					placed({ widePosX: 12, widePosY: 3, width: 8, stackPos: 2 }),
				],
				intoGrid: false,
			},
			expected: [{ widePosX: null, widePosY: null, stackPos: null }],
		},
		{
			name: "keeps the cells of the blocks of a whole grid",
			input: {
				content: () => [
					placedGrid(placed({ widePosX: 12, widePosY: 3, width: 8 })),
				],
				intoGrid: false,
			},
			expected: [{ widePosX: 12, widePosY: 3, stackPos: null }],
		},
		{
			name: "drops the places of the blocks of a whole grid that lands in a grid",
			input: {
				content: () => [
					placedGrid(placed({ widePosX: 12, widePosY: 3, width: 8 })),
				],
				intoGrid: true,
			},
			expected: [{ widePosX: null, widePosY: null, stackPos: null }],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(
			cellsOf(
				withoutGridPlaces(Fragment.fromArray(input.content()), input.intoGrid),
			),
		).toEqual(expected)
	})

	it("returns the content itself when no block has cells", ({ expect }) => {
		const content = Fragment.fromArray([
			placed({ width: 8 }),
			placedGrid(placed({ width: 8 })),
		])

		expect(withoutGridPlaces(content, true)).toBe(content)
	})
})

function node(name: string, ...blocks: PMNode[]): PMNode {
	return nodeType(schema, name).create(null, blocks)
}

function para(text: string, from: Schema = schema): PMNode {
	return nodeType(from, "paragraph").create(null, from.text(text))
}

function metricBlock(uid: string | null = null): PMNode {
	return nodeType(schema, METRIC_BLOCK_NAME).create({ uid })
}

function grid(...blocks: PMNode[]): PMNode {
	return node(BLOCK_GRID_NAME, ...blocks)
}

function legacyGrid(...blocks: PMNode[]): PMNode {
	return nodeType(schema, LEGACY_METRIC_GRID_NAME).create(
		{ uid: "grid" },
		blocks,
	)
}

function container(...blocks: PMNode[]): PMNode {
	return node("container", ...blocks)
}

function docOf(blocks: PMNode[], from: Schema = schema): PMNode {
	return nodeType(from, "doc").create(null, blocks)
}

// renders the top-level nodes as names, with the child count of every
// grid, so a deletion is visible without dumping the whole document
function shape(parent: PMNode): string[] {
	const names: string[] = []

	parent.forEach((child) => {
		names.push(
			child.type.name === BLOCK_GRID_NAME ||
				child.type.name === LEGACY_METRIC_GRID_NAME
				? `${child.type.name}(${child.childCount})`
				: child.type.name,
		)
	})

	return names
}

function childUids(parent: PMNode): unknown[] {
	const uids: unknown[] = []

	parent.forEach((child) => uids.push(child.attrs.uid))

	return uids
}

// the plugins BlockGrid adds
function gridPlugins(isEditable: boolean): Plugin[] {
	const plugins = BlockGrid.config.addProseMirrorPlugins?.call({
		editor: { isEditable },
	} as never)

	if (!plugins) {
		throw new Error("BlockGrid adds no plugins")
	}

	return plugins
}

function migrationState(doc: PMNode): EditorState {
	return EditorState.create({ doc, plugins: gridPlugins(true) })
}

// whether the migration plugin thinks the document may hold a legacy grid
function holdsLegacyGrid(state: EditorState): unknown {
	return state.plugins[0]?.getState(state)
}

// applies one edit the way the editor does, appended transactions included
function applyEdit(
	doc: PMNode,
	edit: (tr: Transaction) => void,
	{ isEditable = true, remote = false } = {},
) {
	const state = EditorState.create({
		doc,
		plugins: gridPlugins(isEditable),
	})
	const tr = state.tr

	edit(tr)

	if (remote) {
		tr.setMeta(ySyncPluginKey, { isChangeOrigin: true })
	}

	return state.applyTransaction(tr)
}

function placed(attrs: Record<string, unknown>): PMNode {
	return nodeType(placedSchema, METRIC_BLOCK_NAME).create(attrs)
}

function placedGrid(...blocks: PMNode[]): PMNode {
	return nodeType(placedSchema, BLOCK_GRID_NAME).create(null, blocks)
}

// the places of every grid block in the content
function cellsOf(content: Fragment): Record<string, unknown>[] {
	const cells: Record<string, unknown>[] = []

	content.descendants((node) => {
		if (node.type.name === METRIC_BLOCK_NAME) {
			cells.push({
				widePosX: node.attrs.widePosX,
				widePosY: node.attrs.widePosY,
				stackPos: node.attrs.stackPos,
			})
		}
	})

	return cells
}

// the grid layout plugin BlockGrid adds
function layoutPlugin(): Plugin<DecorationSet> {
	const plugin = gridPlugins(true)[1]

	if (!plugin) {
		throw new Error("BlockGrid adds no layout plugin")
	}

	return plugin as Plugin<DecorationSet>
}

// the layout decorations after one edit of the document
function applyLayoutEdit(
	doc: PMNode,
	edit: (tr: Transaction) => void,
): DecorationSet {
	const plugin = layoutPlugin()
	const state = EditorState.create({ doc: doc, plugins: [plugin] })
	const tr = state.tr

	edit(tr)

	const decorations = plugin.getState(state.apply(tr))

	if (!decorations) {
		throw new Error("the layout plugin keeps decorations")
	}

	return decorations
}

function oneBlockGrid(): PMNode {
	return placedGrid(placed({}))
}

// a paragraph between two grids of one block each
function twoGrids(): PMNode {
	return nodeType(placedSchema, "doc").create(null, [
		placedGrid(placed({})),
		nodeType(placedSchema, "paragraph").create(),
		placedGrid(placed({})),
	])
}

function layoutDecorations(doc: PMNode): Decoration[] {
	const plugin = gridPlugins(true)[1]
	const state = EditorState.create({
		doc: doc,
		plugins: plugin ? [plugin] : [],
	})

	return (plugin?.getState(state) as DecorationSet | undefined)?.find() ?? []
}

// where each decoration starts and the style it hands its block
function cellStyles(decorations: Decoration[]) {
	return decorations.map((decoration) => ({
		from: decoration.from,
		style: (decoration as unknown as { type: { attrs: { style: string } } })
			.type.attrs.style,
	}))
}
