import { describe, it } from "vitest"
import { removeDuplicates } from "./removeDuplicates"

describe("removeDuplicates", () => {
	it.for<{ name: string; input: unknown[]; expected: unknown[] }>([
		{
			name: "keeps an array without duplicates intact",
			input: [1, 2, 3],
			expected: [1, 2, 3],
		},
		{
			name: "drops repeated numbers",
			input: [1, 2, 1, 3, 2],
			expected: [1, 2, 3],
		},
		{
			name: "drops repeated strings",
			input: ["a", "b", "a"],
			expected: ["a", "b"],
		},
		{
			name: "returns an empty array unchanged",
			input: [],
			expected: [],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(removeDuplicates(input)).toEqual(expected)
	})

	it("keeps the first of two deeply equal objects regardless of key order", ({
		expect,
	}) => {
		const first = { a: 1, b: 2 }

		const result = removeDuplicates([first, { b: 2, a: 1 }, { a: 1, b: 3 }])

		expect(result).toEqual([first, { a: 1, b: 3 }])
		expect(result[0]).toBe(first)
	})

	it("does not conflate a number with its string form", ({ expect }) => {
		expect(removeDuplicates<number | string>([1, "1"])).toEqual([1, "1"])
	})

	it("deduplicates by the custom key function when one is given", ({
		expect,
	}) => {
		const items = [
			{ id: 1, variant: "a" },
			{ id: 1, variant: "b" },
			{ id: 2, variant: "c" },
		]

		const result = removeDuplicates(items, (item: (typeof items)[number]) =>
			String(item.id),
		)

		expect(result).toEqual([
			{ id: 1, variant: "a" },
			{ id: 2, variant: "c" },
		])
	})
})
