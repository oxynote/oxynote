import { getSchema, type Editor, type Range } from "@tiptap/core"
import { ContentDocument } from "../document"
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
import {
	BLOCK_GRID_NAME,
	GRID_ITEM_GROUP,
	METRIC_BLOCK_NAME,
} from "../node-names"
import { runCommand } from "../../comments/test-helpers"
import { nodeType as nodeTypeOf } from "~/components/editor/test-helpers"
import { insertMetricBlock, MetricBlock } from "."
import { BlockGrid } from "../grid"
import { GridItemWidth } from "../grid/utils"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGrid,
	MetricBlock,
])

function nodeType(name: string, from: Schema = schema): NodeType {
	return nodeTypeOf(from, name)
}

function para(text: string, from: Schema = schema): PMNode {
	return nodeType("paragraph", from).create(null, from.text(text))
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
			child.type.name === BLOCK_GRID_NAME
				? `${BLOCK_GRID_NAME}(${child.childCount})`
				: child.type.name,
		)
	})

	return names
}

describe("MetricBlock", () => {
	it("is grouped with the grid items so it is only valid inside a grid", ({
		expect,
	}) => {
		expect(MetricBlock.config.group).toBe(GRID_ITEM_GROUP)
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
				visualizationType: GenericQueryChartType.Line,
				queries: [{ name: "Query 1", query: "", legendFormat: "" }],
				timeRange: METRIC_TIME_RANGE_REFERENCE,
				refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
				thresholds: null,
				baseThresholdColor: "green",
				decimals: null,
				unitType: null,
				unitCustom: null,
				axisBoundsMin: null,
				axisBoundsMax: null,
				simulationPreset: null,
				simulationActive: false,
				width: GridItemWidth.Standard,
			})
		})

		it.for([
			{
				name: "the stored width",
				attr: GridItemWidth.Wide,
				expected: "wide",
			},
			{
				name: "the standard width when the attribute is missing",
				attr: null,
				expected: GridItemWidth.Standard,
			},
			{
				name: "the standard width when the attribute is empty",
				attr: "",
				expected: GridItemWidth.Standard,
			},
		])("parses $name", ({ attr, expected }, { expect }) => {
			const element = {
				getAttribute: vi.fn(() => attr),
			} as unknown as HTMLElement

			expect(nodeType(METRIC_BLOCK_NAME).spec.attrs?.width?.default).toBe(
				GridItemWidth.Standard,
			)
			expect(widthAttribute().parseHTML?.(element)).toBe(expected)
		})

		it("renders the width as a data attribute", ({ expect }) => {
			expect(
				widthAttribute().renderHTML?.({ width: GridItemWidth.Compact }),
			).toEqual({ "data-width": GridItemWidth.Compact })
		})

		it("keeps the simulation flag out of the rendered HTML", ({ expect }) => {
			expect(metricAttributes().simulationActive?.rendered).toBe(false)
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
				`${BLOCK_GRID_NAME}(1)`,
				"paragraph",
			])
		})
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

// how an attribute reaches HTML is only visible on its declaration, so the
// tests read it from there
function metricAttributes() {
	const addAttributes = MetricBlock.config.addAttributes

	if (!addAttributes) {
		throw new Error("MetricBlock declares no attributes")
	}

	return addAttributes.call({} as never) as Record<
		string,
		{
			rendered?: boolean
			parseHTML?: (element: HTMLElement) => unknown
			renderHTML?: (attributes: Record<string, unknown>) => unknown
		}
	>
}

function widthAttribute() {
	const width = metricAttributes().width

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
