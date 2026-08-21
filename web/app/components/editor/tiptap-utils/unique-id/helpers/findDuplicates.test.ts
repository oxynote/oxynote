import { describe, it } from "vitest"
import { findDuplicates } from "./findDuplicates"

describe("findDuplicates", () => {
	it.for([
		{
			name: "returns an empty array when every item is unique",
			input: ["a", "b", "c"],
			expected: [],
		},
		{
			name: "returns a repeated item once",
			input: ["a", "b", "a"],
			expected: ["a"],
		},
		{
			name: "lists an item repeated many times only once",
			input: ["a", "a", "a", "b"],
			expected: ["a"],
		},
		{
			name: "finds several duplicated items",
			input: [1, 2, 1, 3, 2],
			expected: [1, 2],
		},
		{
			name: "returns an empty array for an empty input",
			input: [],
			expected: [],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(findDuplicates(input)).toEqual(expected)
	})

	it("finds an object repeated by reference", ({ expect }) => {
		const shared = { id: "x" }

		expect(findDuplicates([shared, shared])).toEqual([shared])
	})

	it("ignores distinct objects with an equal shape", ({ expect }) => {
		expect(findDuplicates([{ id: "x" }, { id: "x" }])).toEqual([])
	})
})
