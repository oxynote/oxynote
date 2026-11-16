import type { JSONContent } from "@tiptap/core"
import { describe, it } from "vitest"
import { gridItemRow, removedAfterGrid, withGridPlaces } from "./grid-place"
import { DiffStatus } from "./position-map"

describe("withGridPlaces", () => {
	it("gives every block of a grid where it shows in both layouts", ({
		expect,
	}) => {
		const placed = withGridPlaces({
			type: "doc",
			content: [
				{ type: "paragraph" },
				grid(
					{ uid: "a", widePosX: 12, widePosY: 0, width: 12, height: 1 },
					{ uid: "b", widePosX: 0, widePosY: 0, width: 12, height: 2 },
					{ uid: "c", width: "standard", stackPos: 0 },
				),
			],
		})

		expect(placed.content?.[0]).toEqual({ type: "paragraph" })
		expect(
			placed.content?.[1]?.content?.map((block): unknown[] => [
				block.attrs?.uid,
				block.attrs?.gridPlace,
				block.attrs?.stackPlace,
			]),
		).toEqual([
			["a", { row: 0, column: 12 }, 1],
			["b", { row: 0, column: 0 }, 0],
			["c", { row: 1, column: 12 }, 2],
		])
	})
})

describe("removedAfterGrid", () => {
	it.for([
		{
			name: "keeps a grid with no removed block whole",
			input: [DiffStatus.Unchanged, DiffStatus.Added],
		},
		{
			name: "keeps a grid with only removed blocks whole",
			input: [DiffStatus.Removed, DiffStatus.Removed],
		},
	])("$name", ({ input }, { expect }) => {
		const merged = grid(
			...input.map((status, i) => ({ uid: `b${i}`, diffStatus: status })),
		)

		expect(removedAfterGrid(merged)).toEqual([merged])
	})

	it("gives the copy of a grid without a uid no uid", ({ expect }) => {
		const merged: JSONContent = {
			type: "blockGrid",
			attrs: {},
			content: [
				{ type: "metricBlock", attrs: { diffStatus: DiffStatus.Unchanged } },
				{ type: "metricBlock", attrs: { diffStatus: DiffStatus.Removed } },
			],
		}

		expect(removedAfterGrid(merged)[1]?.attrs?.uid).toBeNull()
	})
})

describe("gridItemRow", () => {
	it.for([
		{
			name: "a named width",
			input: { name: "width", oldValue: 8, newValue: 24 },
			expected: {
				label: "editor.diff-change-marker.grid-item.width",
				oldValue: "editor.grid-item.width-options.compact",
				newValue: "editor.grid-item.width-options.wide",
			},
		},
		{
			name: "a width without a name",
			input: { name: "width", oldValue: 10, newValue: 12 },
			expected: {
				label: "editor.diff-change-marker.grid-item.width",
				oldValue: 'editor.grid-item.width-options.columns:{"count":10}:10',
				newValue: "editor.grid-item.width-options.standard",
			},
		},
		{
			name: "a height without a name",
			input: { name: "height", oldValue: 3, newValue: 2 },
			expected: {
				label: "editor.diff-change-marker.grid-item.height",
				oldValue: 'editor.grid-item.height-options.rows:{"count":3}:3',
				newValue: "editor.grid-item.height-options.tall",
			},
		},
		{
			name: "a place in the wide layout, counted from one",
			input: {
				name: "gridPlace",
				oldValue: { row: 0, column: 12 },
				newValue: { row: 1, column: 0 },
			},
			expected: {
				label: "editor.diff-change-marker.grid-item.wide-layout",
				oldValue:
					'editor.diff-change-marker.grid-item.wide-place:{"row":1,"column":13}',
				newValue:
					'editor.diff-change-marker.grid-item.wide-place:{"row":2,"column":1}',
			},
		},
		{
			name: "a place in the stacked layout as an ordinal",
			input: { name: "stackPlace", oldValue: 0, newValue: 10 },
			expected: {
				label: "editor.diff-change-marker.grid-item.stacked-layout",
				oldValue:
					'editor.diff-change-marker.grid-item.stacked-place.one:{"place":1}',
				newValue:
					'editor.diff-change-marker.grid-item.stacked-place.other:{"place":11}',
			},
		},
		{
			name: "the old place of a removed block alone",
			input: { name: "stackPlace", oldValue: 2, newValue: undefined },
			expected: {
				label: "editor.diff-change-marker.grid-item.stacked-layout",
				oldValue:
					'editor.diff-change-marker.grid-item.stacked-place.few:{"place":3}',
				newValue: null,
			},
		},
	])("formats $name", ({ input, expected }, { expect }) => {
		expect(gridItemRow(input, fakeT, "en")).toEqual(expected)
	})

	it("returns nothing for a change that is no size or place", ({ expect }) => {
		expect(
			gridItemRow({ name: "title", oldValue: "a", newValue: "b" }, fakeT, "en"),
		).toBeNull()
	})
})

function grid(...blocks: Record<string, unknown>[]): JSONContent {
	return {
		type: "blockGrid",
		attrs: { uid: "g1" },
		content: blocks.map((attrs) => ({ type: "metricBlock", attrs: attrs })),
	}
}

// answers with the key, its params and its count instead of the text
function fakeT(
	key: string,
	params?: Record<string, unknown>,
	plural?: number,
): string {
	return [key, params && JSON.stringify(params), plural]
		.filter((part) => part !== undefined)
		.join(":")
}
