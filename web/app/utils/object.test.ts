import { describe, it, vi } from "vitest"
import {
	arraysEqual,
	clone,
	extractInitials,
	extractNameFromEmail,
	isValidDescendent,
	lastFilePathElement,
} from "./object"

describe("clone", () => {
	it("deep clones an object so mutations do not leak back", ({ expect }) => {
		const original = { a: 1, nested: { b: [1, 2] } }

		const copy = clone(original)
		copy.nested.b.push(3)

		expect(copy).not.toBe(original)
		expect(original.nested.b).toEqual([1, 2])
		expect(copy.nested.b).toEqual([1, 2, 3])
	})

	it.for([
		{ name: "returns null as-is", input: null },
		{ name: "returns undefined as-is", input: undefined },
		{ name: "returns numbers as-is", input: 42 },
		{ name: "returns strings as-is", input: "hello" },
	])("$name", ({ input }, { expect }) => {
		expect(clone(input)).toBe(input)
	})
})

describe("isValidDescendent", () => {
	// the function only guards nulls and delegates to the DOM contains
	// check, so a minimal stub is all the element needs to be
	function element(contains: boolean) {
		return { contains: vi.fn(() => contains) } as unknown as HTMLElement
	}

	it("returns true when the ancestor contains the descendent", ({ expect }) => {
		const contains = vi.fn(() => true)
		const ancestor = { contains } as unknown as HTMLElement
		const descendent = element(false)

		expect(isValidDescendent(ancestor, descendent)).toBe(true)
		expect(contains).toHaveBeenCalledTimes(1)
		expect(contains).toHaveBeenCalledWith(descendent)
	})

	it("returns false when the ancestor does not contain the descendent", ({
		expect,
	}) => {
		expect(isValidDescendent(element(false), element(false))).toBe(false)
	})

	it.for([
		{ name: "returns false for a null ancestor", a: null, d: element(true) },
		{ name: "returns false for a null descendent", a: element(true), d: null },
		{
			name: "returns false for undefined arguments",
			a: undefined,
			d: undefined,
		},
	])("$name", ({ a, d }, { expect }) => {
		expect(isValidDescendent(a, d)).toBe(false)
	})
})

describe("lastFilePathElement", () => {
	it.for([
		{
			name: "returns the file name of a unix path",
			input: "a/b/c.txt",
			expected: "c.txt",
		},
		{
			name: "returns the file name of a windows path",
			input: "a\\b\\c.txt",
			expected: "c.txt",
		},
		{ name: "ignores a trailing slash", input: "a/b/", expected: "b" },
		{
			name: "returns a bare file name unchanged",
			input: "c.txt",
			expected: "c.txt",
		},
		{
			name: "returns an empty string for an empty path",
			input: "",
			expected: "",
		},
		{
			name: "returns an empty string for a bare slash",
			input: "/",
			expected: "",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(lastFilePathElement(input)).toBe(expected)
	})
})

describe("arraysEqual", () => {
	it.for([
		{
			name: "returns true for the same reference",
			a: [1],
			b: null,
			same: true,
			expected: true,
		},
		{
			name: "returns true when both are null",
			a: null,
			b: null,
			same: false,
			expected: true,
		},
		{
			name: "returns false when only one is null",
			a: null,
			b: [1],
			same: false,
			expected: false,
		},
		{
			name: "returns true for equal elements",
			a: [1, 2],
			b: [1, 2],
			same: false,
			expected: true,
		},
		{
			name: "returns true for two empty arrays",
			a: [],
			b: [],
			same: false,
			expected: true,
		},
		{
			name: "returns false for different lengths",
			a: [1],
			b: [1, 2],
			same: false,
			expected: false,
		},
		{
			name: "returns false for different elements",
			a: [1, 2],
			b: [1, 3],
			same: false,
			expected: false,
		},
	])("$name", ({ a, b, same, expected }, { expect }) => {
		expect(arraysEqual(a, same ? a : b)).toBe(expected)
	})

	it("compares elements by reference, not structurally", ({ expect }) => {
		expect(arraysEqual([{ x: 1 }], [{ x: 1 }])).toBe(false)
	})
})

describe("extractInitials", () => {
	it.for([
		{
			name: "takes the first letter of each word",
			input: "John Doe",
			expected: "JD",
		},
		{
			name: "splits on hyphens too",
			input: "mary-jane watson",
			expected: "MJW",
		},
		{
			name: "stops at the maximum when given",
			input: "a b c d",
			max: 2,
			expected: "AB",
		},
		{
			name: "skips non-letter characters to the first letter",
			input: "🎉party time",
			expected: "PT",
		},
		{
			name: "upper-cases unicode letters",
			input: "łukasz nowak",
			expected: "ŁN",
		},
		{ name: "ignores extra whitespace", input: "  a   b  ", expected: "AB" },
		{
			name: "returns an empty string for empty input",
			input: "",
			expected: "",
		},
		{
			name: "returns an empty string without any letters",
			input: "123 !!!",
			expected: "",
		},
	])("$name", ({ input, max, expected }, { expect }) => {
		expect(extractInitials(input, max)).toBe(expected)
	})
})

describe("extractNameFromEmail", () => {
	it.for([
		{
			name: "returns the part before the at sign",
			input: "john.doe@test.io",
			expected: "john.doe",
		},
		{
			name: "trims surrounding whitespace",
			input: "  a@test.io  ",
			expected: "a",
		},
		{
			name: "returns an empty string without an at sign",
			input: "john.doe",
			expected: "",
		},
		{
			name: "returns an empty string for a leading at sign",
			input: "@test.io",
			expected: "",
		},
		{
			name: "returns an empty string for an empty input",
			input: "",
			expected: "",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(extractNameFromEmail(input)).toBe(expected)
	})
})
