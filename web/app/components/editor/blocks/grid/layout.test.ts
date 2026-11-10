import { getSchema } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import { EditorState } from "@tiptap/pm/state"
import { describe, it } from "vitest"
import { GridItemWidth } from "@oxynote/common/grid"
import { ContentDocument } from "../document"
import { BLOCK_GRID_NAME, METRIC_BLOCK_NAME } from "../node-names"
import {
	BlockGridStub,
	GridMetricBlockStub,
	nodeType,
} from "~/components/editor/test-helpers"
import { gridLayoutOf, gridStackOf, writeGridPlaces } from "./layout"

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGridStub,
	GridMetricBlockStub,
])

describe("gridLayoutOf", () => {
	it("lays out a grid node's children", ({ expect }) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME).create(null, [
			nodeType(schema, METRIC_BLOCK_NAME).create({
				width: GridItemWidth.Compact,
			}),
			nodeType(schema, METRIC_BLOCK_NAME).create({
				widePosX: 12,
				widePosY: 0,
				width: 12,
				height: 2,
			}),
		])

		expect(gridLayoutOf(grid)).toEqual([
			{ x: 0, y: 0, w: 8, h: 1 },
			{ x: 12, y: 0, w: 12, h: 2 },
		])
	})

	it("keeps the layout of a grid node it laid out before", ({ expect }) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME).create(null, [
			nodeType(schema, METRIC_BLOCK_NAME).create(),
		])

		expect(gridLayoutOf(grid)).toBe(gridLayoutOf(grid))
	})
})

describe("gridStackOf", () => {
	it("lists a grid node's children by their places in the stacked layout", ({
		expect,
	}) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME).create(null, [
			nodeType(schema, METRIC_BLOCK_NAME).create({ stackPos: 1 }),
			nodeType(schema, METRIC_BLOCK_NAME).create({ stackPos: 0 }),
		])

		expect(gridStackOf(grid)).toEqual([1, 0])
	})

	it("keeps the order of a grid node it listed before", ({ expect }) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME).create(null, [
			nodeType(schema, METRIC_BLOCK_NAME).create(),
		])

		expect(gridStackOf(grid)).toBe(gridStackOf(grid))
	})
})

describe("writeGridPlaces", () => {
	it("writes the cells and stacked places of the blocks that change and only those", ({
		expect,
	}) => {
		const state = gridState(
			{ widePosX: 0, widePosY: 0, stackPos: 0 },
			{ widePosX: 12, widePosY: 0, stackPos: 1 },
		)
		const tr = state.tr

		writeGridPlaces(
			tr,
			0,
			[
				{ x: 0, y: 0, w: 12, h: 1 },
				{ x: 0, y: 1, w: 24, h: 1 },
			],
			[1, 0],
		)

		expect.soft(tr.steps).toHaveLength(2)
		expect.soft(places(tr.doc)).toEqual(["0,0 12x1 1", "0,1 24x1 0"])
	})

	it("leaves the stacked places alone without an order", ({ expect }) => {
		const state = gridState({ stackPos: 1 }, { stackPos: 0 })
		const tr = state.tr

		writeGridPlaces(
			tr,
			0,
			[
				{ x: 0, y: 0, w: 12, h: 1 },
				{ x: 12, y: 0, w: 12, h: 1 },
			],
			null,
		)

		expect(places(tr.doc)).toEqual(["0,0 12x1 1", "12,0 12x1 0"])
	})

	it("leaves the cells alone without a layout", ({ expect }) => {
		const state = gridState({}, {})
		const tr = state.tr

		writeGridPlaces(tr, 0, null, [1, 0])

		expect(places(tr.doc)).toEqual(["null,null 12x1 1", "null,null 12x1 0"])
	})

	it("writes nothing when nothing changes", ({ expect }) => {
		const state = gridState(
			{ widePosX: 0, widePosY: 0, stackPos: 0 },
			{ widePosX: 12, widePosY: 0, stackPos: 1 },
		)
		const tr = state.tr

		writeGridPlaces(
			tr,
			0,
			[
				{ x: 0, y: 0, w: 12, h: 1 },
				{ x: 12, y: 0, w: 12, h: 1 },
			],
			[0, 1],
		)

		expect(tr.steps).toHaveLength(0)
	})
})

// a document with one grid that holds a block for each set of attributes
function gridState(...blocks: Record<string, unknown>[]): EditorState {
	return EditorState.create({
		doc: nodeType(schema, "doc").create(null, [
			nodeType(schema, BLOCK_GRID_NAME).create(
				null,
				blocks.map((attrs) =>
					nodeType(schema, METRIC_BLOCK_NAME).create(attrs),
				),
			),
		]),
	})
}

// each block of the first grid with its stored cells and stacked place
function places(doc: PMNode): string[] {
	return (doc.firstChild?.children ?? []).map(
		(child) =>
			`${String(child.attrs.widePosX)},${String(child.attrs.widePosY)} ${String(child.attrs.width)}x${String(child.attrs.height)} ${String(child.attrs.stackPos)}`,
	)
}
