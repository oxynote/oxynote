import { describe, it } from "vitest"
import { cleanSentenceCase } from "./string"

describe("cleanSentenceCase", () => {
	it.for([
		{
			name: "capitalizes the first letter",
			input: "hello world",
			expected: "Hello world",
		},
		{
			name: "returns an empty string unchanged",
			input: "",
			expected: "",
		},
		{
			name: "keeps an already capitalized string unchanged",
			input: "Hello",
			expected: "Hello",
		},
		{
			name: "capitalizes a single-character string",
			input: "a",
			expected: "A",
		},
		{
			name: "leaves a non-letter first character unchanged",
			input: "1st item",
			expected: "1st item",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(cleanSentenceCase(input)).toBe(expected)
	})
})
