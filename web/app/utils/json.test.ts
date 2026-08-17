import { describe, it } from "vitest"
import { jsonReviver, jsonStableStringify } from "./json"

describe("jsonReviver", () => {
	it.for([
		{
			name: "converts an RFC 3339 date string to a Date",
			input: "2024-06-15T12:30:00Z",
			expected: new Date("2024-06-15T12:30:00Z"),
		},
		{
			name: "leaves a non-date string unchanged",
			input: "hello",
			expected: "hello",
		},
		{
			name: "leaves a date-like string without time unchanged",
			input: "2024-06-15",
			expected: "2024-06-15",
		},
		{
			name: "leaves numbers unchanged",
			input: 42,
			expected: 42,
		},
		{
			name: "leaves booleans unchanged",
			input: true,
			expected: true,
		},
		{
			name: "leaves null unchanged",
			input: null,
			expected: null,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(jsonReviver("key", input)).toEqual(expected)
	})

	it("revives date fields when used with JSON.parse", ({ expect }) => {
		const parsed = JSON.parse(
			'{"at":"2024-06-15T12:30:00Z","name":"doc"}',
			jsonReviver,
		) as { at: Date; name: string }

		expect(parsed.at).toEqual(new Date("2024-06-15T12:30:00Z"))
		expect(parsed.name).toBe("doc")
	})
})

describe("jsonStableStringify", () => {
	it("serializes objects with sorted keys", ({ expect }) => {
		expect(jsonStableStringify({ b: 2, a: 1 })).toBe('{"a":1,"b":2}')
	})

	it("produces identical output regardless of key insertion order", ({
		expect,
	}) => {
		expect(jsonStableStringify({ b: 2, a: { d: 4, c: 3 } })).toBe(
			jsonStableStringify({ a: { c: 3, d: 4 }, b: 2 }),
		)
	})

	it("returns an empty string for undefined", ({ expect }) => {
		expect(jsonStableStringify(undefined)).toBe("")
	})
})
