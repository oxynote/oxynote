import { getSchema, type Editor, type Range } from "@tiptap/core"
import { ContentDocument } from "../document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Fragment, type Node as PMNode, type NodeType } from "@tiptap/pm/model"
import { EditorState } from "@tiptap/pm/state"
import { describe, it, vi } from "vitest"
import { BLOCK_GRID_NAME, GRID_ITEM_GROUP, LOG_BLOCK_NAME } from "../node-names"
import { runCommand } from "../../comments/test-helpers"
import { nodeType as nodeTypeOf } from "~/components/editor/test-helpers"
import { insertLogBlock, LogBlock } from "."
import { LogTimestampFormat } from "./utils"
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
	LogBlock,
])

describe("LogBlock", () => {
	it("is grouped with the grid items so it is only valid inside a grid", ({
		expect,
	}) => {
		expect(LogBlock.config.group).toBe(GRID_ITEM_GROUP)
		expect(
			nodeType("doc").validContent(
				Fragment.from(nodeType(LOG_BLOCK_NAME).create()),
			),
		).toBe(false)
		expect(
			nodeType(BLOCK_GRID_NAME).validContent(
				Fragment.from(nodeType(LOG_BLOCK_NAME).create()),
			),
		).toBe(true)
	})

	it("matches the block marker element", ({ expect }) => {
		expect(LogBlock.config.parseHTML?.call({} as never)).toEqual([
			{ tag: `div[data-type="log-block"]` },
		])
	})

	it("renders a div carrying the block marker alongside the given attributes", ({
		expect,
	}) => {
		expect(
			LogBlock.config.renderHTML?.call({} as never, {
				HTMLAttributes: { class: "custom" },
				node: {} as never,
			}),
		).toEqual(["div", { class: "custom", "data-type": "log-block" }, 0])
	})

	describe("addAttributes", () => {
		it("defaults every config field to the default log config", ({
			expect,
		}) => {
			expect(nodeType(LOG_BLOCK_NAME).createAndFill()?.attrs).toEqual({
				title: "",
				dataSourceId: null,
				queries: [{ name: "Query 1", query: "" }],
				timeRange: METRIC_TIME_RANGE_REFERENCE,
				refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
				timestampFormat: LogTimestampFormat.ReadableTime,
				order: LogOrder.Desc,
				width: GridItemWidth.Standard,
			})
		})

		it.for([
			{
				name: "the stored width",
				attr: GridItemWidth.Wide,
				expected: GridItemWidth.Wide,
			},
			{
				name: "the standard width when the attribute is missing",
				attr: null,
				expected: GridItemWidth.Standard,
			},
		])("parses $name", ({ attr, expected }, { expect }) => {
			const element = {
				getAttribute: vi.fn(() => attr),
			} as unknown as HTMLElement

			expect(widthAttribute().parseHTML?.(element)).toBe(expected)
		})

		it("renders the width as a data attribute", ({ expect }) => {
			expect(
				widthAttribute().renderHTML?.({ width: GridItemWidth.Compact }),
			).toEqual({ "data-width": GridItemWidth.Compact })
		})
	})

	describe("insertLogBlock", () => {
		it("replaces the paragraph with a grid holding one log block", ({
			expect,
		}) => {
			const state = EditorState.create({
				doc: docOf([para("a"), para("b")]),
			})

			const res = runCommand(logBlockCommands().insertLogBlock?.(1), state)

			expect(res.result).toBe(true)
			expect(shape(res.state.doc)).toEqual([
				`${BLOCK_GRID_NAME}(1)`,
				"paragraph",
			])
			expect(res.state.doc.child(0).child(0).type.name).toBe(LOG_BLOCK_NAME)
		})

		it("inserts nothing when the position is not inside a paragraph", ({
			expect,
		}) => {
			const state = EditorState.create({
				doc: docOf([para("a"), para("b")]),
			})

			const res = runCommand(logBlockCommands().insertLogBlock?.(0), state)

			expect(res.result).toBe(false)
			expect(res.tr.steps).toHaveLength(0)
		})
	})
})

describe("insertLogBlock", () => {
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
		chain.insertLogBlock = record("insertLogBlock")
		chain.run = record("run")

		const editor = { chain: () => chain } as unknown as Editor
		const range: Range = { from: 3, to: 7 }

		insertLogBlock(editor, range)

		expect(calls).toEqual([
			["focus", undefined],
			["deleteRange", range],
			["insertLogBlock", 3],
			["run", undefined],
		])
	})
})

function nodeType(name: string): NodeType {
	return nodeTypeOf(schema, name)
}

function para(text: string): PMNode {
	return nodeType("paragraph").create(null, schema.text(text))
}

function docOf(blocks: PMNode[]): PMNode {
	return nodeType("doc").create(null, blocks)
}

// renders the top-level nodes as names, with the child count of every
// grid, so an insertion is visible without dumping the whole document
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

// how an attribute reaches HTML is only visible on its declaration, so the
// tests read it from there
function widthAttribute() {
	const addAttributes = LogBlock.config.addAttributes

	if (!addAttributes) {
		throw new Error("LogBlock declares no attributes")
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
		throw new Error("LogBlock declares no width attribute")
	}

	return width
}

function logBlockCommands() {
	const addCommands = LogBlock.config.addCommands

	if (!addCommands) {
		throw new Error("LogBlock declares no commands")
	}

	return addCommands.call({} as never)
}
