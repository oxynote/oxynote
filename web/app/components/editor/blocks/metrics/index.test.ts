import { getSchema, Node, type Editor, type Range } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import {
	Fragment,
	type Node as PMNode,
	type NodeType,
	type Schema,
} from "@tiptap/pm/model"
import { EditorState } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../node-names"
import { runCommand } from "../../comments/test-helpers"
import { nodeType as nodeTypeOf } from "~/components/editor/test-helpers"
import {
	deleteEmptyMetricGrids,
	insertMetricBlock,
	MetricBlock,
	MetricGrid,
	setUpMetricBlock,
} from "."
import { MetricBlockWidth, RefreshInterval, TimeRangePreset } from "./utils"

// the block attribute defaults come from defaultMetricConfig, which
// resolves CSS variables through a canvas — unavailable in node
vi.mock("~/assets/css", () => ({
	chartStyles: () => ({ thresholdColors: { default: "#8a3ffc" } }),
}))

// a nesting context standing in for the real containers that accept a
// bare metric block (e.g. the split documentation right side)
const Container = Node.create({
	name: "container",
	group: "block",
	content: `(paragraph | ${METRIC_BLOCK_NAME})+`,
})

const schema = getSchema([
	Document,
	Paragraph,
	Text,
	Container,
	MetricGrid,
	MetricBlock,
])

// a schema that knows paragraphs but neither metric node type
const schemaWithoutMetrics = getSchema([Document, Paragraph, Text])

function nodeType(name: string, from: Schema = schema): NodeType {
	return nodeTypeOf(from, name)
}

function para(text: string, from: Schema = schema): PMNode {
	return nodeType("paragraph", from).create(null, from.text(text))
}

function metricBlock(): PMNode {
	const block = nodeType(METRIC_BLOCK_NAME).createAndFill()

	if (!block) {
		throw new Error("the metric block could not be filled")
	}

	return block
}

function grid(...blocks: PMNode[]): PMNode {
	return nodeType(METRIC_GRID_NAME).create(null, blocks)
}

function container(...blocks: PMNode[]): PMNode {
	return nodeType("container").create(null, blocks)
}

function docOf(blocks: PMNode[], from: Schema = schema): PMNode {
	return nodeType("doc", from).create(null, blocks)
}

// renders the top-level nodes as names, with the child count of every
// grid, so a deletion is visible without dumping the whole document
function shape(node: PMNode): string[] {
	const names: string[] = []

	node.forEach((child) => {
		names.push(
			child.type.name === METRIC_GRID_NAME
				? `${METRIC_GRID_NAME}(${child.childCount})`
				: child.type.name,
		)
	})

	return names
}

describe("deleteEmptyMetricGrids", () => {
	it("deletes an empty grid", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([para("before"), grid(), para("after")]),
		})

		const tr = deleteEmptyMetricGrids(state.tr, schema)

		expect(shape(tr.doc)).toEqual(["paragraph", "paragraph"])
	})

	it("keeps a grid that still holds a metric block", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([grid(metricBlock())]),
		})

		const tr = deleteEmptyMetricGrids(state.tr, schema)

		expect(shape(tr.doc)).toEqual([`${METRIC_GRID_NAME}(1)`])
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
				grid(),
				para("b"),
				grid(),
			]),
		})

		const tr = deleteEmptyMetricGrids(state.tr, schema)

		expect(shape(tr.doc)).toEqual([
			"paragraph",
			`${METRIC_GRID_NAME}(1)`,
			"paragraph",
		])
		expect(tr.steps).toHaveLength(3)
	})

	it("leaves a document without any grid untouched", ({ expect }) => {
		const state = EditorState.create({ doc: docOf([para("a")]) })

		const tr = deleteEmptyMetricGrids(state.tr, schema)

		expect(tr.steps).toHaveLength(0)
	})

	it("returns the transaction unchanged when the schema has no grid type", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([para("a", schemaWithoutMetrics)], schemaWithoutMetrics),
		})
		const original = state.tr

		const tr = deleteEmptyMetricGrids(original, schemaWithoutMetrics)

		expect(tr).toBe(original)
		expect(tr.steps).toHaveLength(0)
	})
})

describe("MetricGrid", () => {
	it("accepts zero metric blocks so emptying operations stay valid", ({
		expect,
	}) => {
		expect(MetricGrid.config.content).toBe(`${METRIC_BLOCK_NAME}*`)
		expect(nodeType(METRIC_GRID_NAME).createAndFill()).not.toBeNull()
	})

	it("matches the grid marker element", ({ expect }) => {
		expect(MetricGrid.config.parseHTML?.call({} as never)).toEqual([
			{ tag: `div[data-type="metric-grid"]` },
		])
	})

	it("renders a div carrying the grid marker alongside the given attributes", ({
		expect,
	}) => {
		expect(
			MetricGrid.config.renderHTML?.call({} as never, {
				HTMLAttributes: { class: "custom" },
				node: {} as never,
			}),
		).toEqual(["div", { class: "custom", "data-type": "metric-grid" }, 0])
	})
})

describe("MetricBlock", () => {
	it("is grouped under its own name so it is only valid inside a grid", ({
		expect,
	}) => {
		expect(MetricBlock.config.group).toBe(METRIC_BLOCK_NAME)
		expect(
			nodeType("doc").validContent(
				Fragment.from(nodeType(METRIC_BLOCK_NAME).create()),
			),
		).toBe(false)
	})

	it("matches the block marker element", ({ expect }) => {
		expect(MetricBlock.config.parseHTML?.call({} as never)).toEqual([
			{ tag: `div[data-type="metric-block"]` },
		])
	})

	it("renders a div carrying the block marker alongside the given attributes", ({
		expect,
	}) => {
		expect(
			MetricBlock.config.renderHTML?.call({} as never, {
				HTMLAttributes: { class: "custom" },
				node: {} as never,
			}),
		).toEqual(["div", { class: "custom", "data-type": "metric-block" }, 0])
	})

	describe("addAttributes", () => {
		it("defaults every config field to the default metric config", ({
			expect,
		}) => {
			expect(nodeType(METRIC_BLOCK_NAME).createAndFill()?.attrs).toEqual({
				config: null,
				title: "",
				dataSourceId: null,
				visualizationType: null,
				queries: [{ name: "Query 1", query: "", legendFormat: "" }],
				timeRange: TimeRangePreset.Last5Minutes,
				refreshInterval: RefreshInterval.M5,
				thresholds: null,
				baseThresholdColor: "#8a3ffc",
				decimals: null,
				unitType: null,
				unitCustom: null,
				axisBoundsMin: null,
				axisBoundsMax: null,
				width: MetricBlockWidth.Standard,
			})
		})

		it.for([
			{
				name: "the stored width",
				attr: MetricBlockWidth.Wide,
				expected: "wide",
			},
			{
				name: "the standard width when the attribute is missing",
				attr: null,
				expected: MetricBlockWidth.Standard,
			},
			{
				name: "the standard width when the attribute is empty",
				attr: "",
				expected: MetricBlockWidth.Standard,
			},
		])("parses $name", ({ attr, expected }, { expect }) => {
			const element = {
				getAttribute: vi.fn(() => attr),
			} as unknown as HTMLElement

			expect(nodeType(METRIC_BLOCK_NAME).spec.attrs?.width?.default).toBe(
				MetricBlockWidth.Standard,
			)
			expect(widthAttribute().parseHTML?.(element)).toBe(expected)
		})

		it("renders the width as a data attribute", ({ expect }) => {
			expect(
				widthAttribute().renderHTML?.({ width: MetricBlockWidth.Compact }),
			).toEqual({ "data-width": MetricBlockWidth.Compact })
		})
	})

	describe("insertMetricBlock", () => {
		it("replaces the paragraph with a grid holding one metric block", ({
			expect,
		}) => {
			const state = EditorState.create({
				doc: docOf([para("a"), para("b")]),
			})

			const res = runCommand(
				metricBlockCommands().insertMetricBlock?.(1),
				state,
			)

			expect(res.result).toBe(true)
			expect(shape(res.state.doc)).toEqual([
				`${METRIC_GRID_NAME}(1)`,
				"paragraph",
			])
		})

		it("returns false when the position is not inside a paragraph", ({
			expect,
		}) => {
			const state = EditorState.create({
				doc: docOf([container(para("a"))]),
			})

			const res = runCommand(
				metricBlockCommands().insertMetricBlock?.(1),
				state,
			)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})

		it("reports success without dispatching when only asked whether it can run", ({
			expect,
		}) => {
			const state = EditorState.create({
				doc: docOf([para("a"), para("b")]),
			})

			const res = runCommand(
				metricBlockCommands().insertMetricBlock?.(1),
				state,
				{ dispatch: false },
			)

			expect(res.result).toBe(true)
			expect(res.tr.steps).toHaveLength(0)
			expect(shape(res.state.doc)).toEqual(["paragraph", "paragraph"])
		})
	})
})

// the width attribute owns the only parse/render pair among the block
// attributes, so the tests reach it through the declaration
function widthAttribute() {
	const addAttributes = MetricBlock.config.addAttributes

	if (!addAttributes) {
		throw new Error("MetricBlock declares no attributes")
	}

	const attributes = addAttributes.call({} as never) as Record<
		string,
		{
			parseHTML?: (element: HTMLElement) => unknown
			renderHTML?: (attributes: Record<string, unknown>) => unknown
		}
	>

	const width = attributes.width

	if (!width) {
		throw new Error("MetricBlock declares no width attribute")
	}

	return width
}

function metricBlockCommands() {
	const addCommands = MetricBlock.config.addCommands

	if (!addCommands) {
		throw new Error("MetricBlock declares no commands")
	}

	return addCommands.call({} as never)
}

describe("setUpMetricBlock", () => {
	it("wraps the metric block in a grid at the document root", ({ expect }) => {
		const state = EditorState.create({
			doc: docOf([para("before"), para("here"), para("after")]),
		})
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 11)

		expect(res).toBe(true)
		expect(shape(tr.doc)).toEqual([
			"paragraph",
			`${METRIC_GRID_NAME}(1)`,
			"paragraph",
		])
	})

	it("inserts a bare metric block when the paragraph is nested", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([container(para("a"), para("b"))]),
		})
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 2)

		expect(res).toBe(true)
		expect(shape(tr.doc.child(0))).toEqual([METRIC_BLOCK_NAME, "paragraph"])
	})

	it("replaces the paragraph when it is the only block of its parent", ({
		expect,
	}) => {
		const state = EditorState.create({ doc: docOf([para("a")]) })
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 1)

		expect(res).toBe(true)
		expect(shape(tr.doc)).toEqual([`${METRIC_GRID_NAME}(1)`])
	})

	it("replaces the only paragraph of a nested parent", ({ expect }) => {
		const state = EditorState.create({ doc: docOf([container(para("a"))]) })
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 2)

		expect(res).toBe(true)
		expect(shape(tr.doc.child(0))).toEqual([METRIC_BLOCK_NAME])
	})

	it("returns false when the position is not inside a paragraph", ({
		expect,
	}) => {
		const state = EditorState.create({ doc: docOf([container(para("a"))]) })
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 1)

		expect(res).toBe(false)
		expect(tr.steps).toHaveLength(0)
	})

	it("returns false when the schema declares no metric node types", ({
		expect,
	}) => {
		const state = EditorState.create({
			doc: docOf([para("a", schemaWithoutMetrics)], schemaWithoutMetrics),
		})
		const tr = state.tr

		const res = setUpMetricBlock(state, tr, 1)

		expect(res).toBe(false)
		expect(tr.steps).toHaveLength(0)
	})
})

describe("insertMetricBlock", () => {
	it("focuses the editor, drops the trigger range and inserts at its start", ({
		expect,
	}) => {
		const calls: [string, unknown][] = []
		const chain: Record<string, (arg?: unknown) => unknown> = {}
		const record = (name: string) => (arg?: unknown) => {
			calls.push([name, arg])

			return chain
		}

		chain.focus = record("focus")
		chain.deleteRange = record("deleteRange")
		chain.insertMetricBlock = record("insertMetricBlock")
		chain.run = record("run")

		const editor = { chain: () => chain } as unknown as Editor
		const range: Range = { from: 3, to: 7 }

		insertMetricBlock(editor, range)

		expect(calls).toEqual([
			["focus", undefined],
			["deleteRange", range],
			["insertMetricBlock", 3],
			["run", undefined],
		])
	})
})
