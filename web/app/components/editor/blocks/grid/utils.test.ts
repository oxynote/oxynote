import { describe, it } from "vitest"
import {
	gridItemHeightAttribute,
	gridItemPlaceAttributes,
	gridItemWidthAttribute,
	gridSizeLabel,
} from "./utils"

describe("gridItemWidthAttribute", () => {
	it.for([
		{ name: "a number of columns", input: "10", expected: 10 },
		{ name: "the name an older block wrote", input: "compact", expected: 8 },
		{ name: "a missing width as standard", input: null, expected: 12 },
		{ name: "too many columns as standard", input: "30", expected: 12 },
	])("parses $name from pasted HTML", ({ input, expected }, { expect }) => {
		const element = { getAttribute: () => input } as unknown as HTMLElement

		expect(gridItemWidthAttribute().parseHTML(element)).toBe(expected)
	})

	it("renders the width as a data attribute and defaults to standard", ({
		expect,
	}) => {
		const attribute = gridItemWidthAttribute()

		expect(attribute.default).toBe(12)
		expect(attribute.renderHTML({ width: 8 })).toEqual({ "data-width": 8 })
	})
})

describe("gridItemHeightAttribute", () => {
	it.for([
		{ name: "a number of rows", input: "2", expected: 2 },
		{ name: "a missing height as one row", input: null, expected: 1 },
	])("parses $name from pasted HTML", ({ input, expected }, { expect }) => {
		const element = { getAttribute: () => input } as unknown as HTMLElement

		expect(gridItemHeightAttribute().parseHTML(element)).toBe(expected)
	})

	it("renders the height as a data attribute and defaults to one row", ({
		expect,
	}) => {
		const attribute = gridItemHeightAttribute()

		expect(attribute.default).toBe(1)
		expect(attribute.renderHTML({ height: 2 })).toEqual({ "data-height": 2 })
	})
})

describe("gridItemPlaceAttributes", () => {
	it.for([
		{ name: "a column", input: "3", expected: 3 },
		{ name: "the first column", input: "0", expected: 0 },
		{ name: "a missing value", input: null, expected: null },
		{ name: "a negative value", input: "-1", expected: null },
		{ name: "a fraction", input: "1.5", expected: null },
	])("parses $name from pasted HTML", ({ input, expected }, { expect }) => {
		const element = { getAttribute: () => input } as unknown as HTMLElement

		expect(gridItemPlaceAttributes().widePosX.parseHTML(element)).toBe(expected)
	})

	it("renders each set value as a data attribute and leaves unset ones out", ({
		expect,
	}) => {
		const attributes = gridItemPlaceAttributes()
		const stored = { widePosX: 0, widePosY: null, stackPos: 3 }

		expect([
			attributes.widePosX.renderHTML(stored),
			attributes.widePosY.renderHTML(stored),
			attributes.stackPos.renderHTML(stored),
		]).toEqual([{ "data-wide-pos-x": 0 }, {}, { "data-stack-pos": 3 }])
		expect(attributes.widePosX.default).toBeNull()
	})
})

describe("gridSizeLabel", () => {
	it.for([
		{
			name: "a width by its name",
			input: { side: "width", count: 8 },
			expected: "editor.grid-item.width-options.compact",
		},
		{
			name: "a width without a name as columns",
			input: { side: "width", count: 10 },
			expected: 'editor.grid-item.width-options.columns:{"count":10}:10',
		},
		{
			name: "a height by its name",
			input: { side: "height", count: 2 },
			expected: "editor.grid-item.height-options.tall",
		},
		{
			name: "a height without a name as rows",
			input: { side: "height", count: 3 },
			expected: 'editor.grid-item.height-options.rows:{"count":3}:3',
		},
	] as const)("labels $name", ({ input, expected }, { expect }) => {
		expect(gridSizeLabel(fakeT, input.side, input.count)).toBe(expected)
	})
})

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
