import { describe, expect, it } from "vitest"
import { lcs } from "./lcs"

describe("lcs", () => {
	it.for([
		{
			name: "matches identical arrays pairwise",
			a: [1, 2, 3],
			b: [1, 2, 3],
			expected: [
				[0, 0],
				[1, 1],
				[2, 2],
			],
		},
		{
			name: "returns no pairs when the first array is empty",
			a: [],
			b: [1, 2],
			expected: [],
		},
		{
			name: "returns no pairs when the second array is empty",
			a: [1, 2],
			b: [],
			expected: [],
		},
		{
			name: "returns no pairs when both arrays are empty",
			a: [],
			b: [],
			expected: [],
		},
		{
			name: "returns no pairs when the arrays share no elements",
			a: [1, 2],
			b: [3, 4],
			expected: [],
		},
		{
			name: "matches an overlapping run at shifted positions",
			a: [1, 2, 3],
			b: [2, 3, 4],
			expected: [
				[1, 0],
				[2, 1],
			],
		},
		{
			name: "matches a subsequence across gaps",
			a: [1, 9, 2, 9, 3],
			b: [1, 2, 3],
			expected: [
				[0, 0],
				[2, 1],
				[4, 2],
			],
		},
		{
			name: "prefers the earliest position for a duplicate in the first array",
			a: [7, 7],
			b: [7],
			expected: [[0, 0]],
		},
		{
			name: "prefers the earliest position for a duplicate in the second array",
			a: [7],
			b: [7, 7],
			expected: [[0, 0]],
		},
	])("$name", ({ a, b, expected }, { expect }) => {
		expect(lcs(a, b)).toEqual(expected)
	})

	it("matches through a custom equality function", () => {
		const a = [{ id: 1 }, { id: 2 }]
		const b = [{ id: 2 }]

		expect(lcs(a, b, (x, y) => x.id === y.id)).toEqual([[1, 0]])
	})
})
