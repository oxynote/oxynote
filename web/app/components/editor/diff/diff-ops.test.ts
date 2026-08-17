import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { computeTokenDiff } from "./diff-ops"
import type { DiffToken } from "./diff-tokens"

// builds one token per code point, all sharing the same marks array —
// exactly how tokens coming from a single text node look
function tokens(text: string, marks: JSONContent[] = []): DiffToken[] {
	// eslint-disable-next-line @typescript-eslint/no-misused-spread -- code point granularity is exactly what DiffToken uses
	return [...text].map((ch) => ({ text: ch, marks }))
}

// compresses ops into [type, text] pairs for readable assertions
function shape(oldText: string, newText: string): [string, string][]
function shape(
	oldTokens: DiffToken[],
	newTokens: DiffToken[],
): [string, string][]
function shape(
	oldInput: string | DiffToken[],
	newInput: string | DiffToken[],
): [string, string][] {
	const oldTokens = typeof oldInput === "string" ? tokens(oldInput) : oldInput
	const newTokens = typeof newInput === "string" ? tokens(newInput) : newInput

	return computeTokenDiff(oldTokens, newTokens).map((op) => [
		op.type,
		op.tokens.map((t) => t.text).join(""),
	])
}

describe("computeTokenDiff", () => {
	it.for([
		{
			name: "returns one equal op for identical text",
			oldText: "hello world",
			newText: "hello world",
			expected: [["equal", "hello world"]],
		},
		{
			name: "returns no ops for empty inputs",
			oldText: "",
			newText: "",
			expected: [],
		},
		{
			name: "emits an insert for appended text",
			oldText: "hello",
			newText: "hello world",
			expected: [
				["equal", "hello"],
				["insert", " world"],
			],
		},
		{
			name: "emits a delete for removed text",
			oldText: "hello world",
			newText: "hello",
			expected: [
				["equal", "hello"],
				["delete", " world"],
			],
		},
		{
			name: "emits a delete and insert pair for a replaced word",
			oldText: "hello world",
			newText: "goodbye world",
			expected: [
				["delete", "hello"],
				["insert", "goodbye"],
				["equal", " world"],
			],
		},
		{
			name: "treats an emoji as a single unit",
			oldText: "👍",
			newText: "👍!",
			expected: [
				["equal", "👍"],
				["insert", "!"],
			],
		},
	])("$name", ({ oldText, newText, expected }, { expect }) => {
		expect(shape(oldText, newText)).toEqual(expected)
	})

	it("emits a delete and insert pair when only marks change", () => {
		const bold = [{ type: "bold" }]
		const oldTokens = [...tokens("plain "), ...tokens("word")]
		const newTokens = [...tokens("plain "), ...tokens("word", bold)]

		expect(shape(oldTokens, newTokens)).toEqual([
			["equal", "plain "],
			["delete", "word"],
			["insert", "word"],
		])
	})

	it("expands a single-character mark change to the whole word", () => {
		const bold = [{ type: "bold" }]
		const oldTokens = tokens("abc")
		const newTokens = [...tokens("a"), ...tokens("b", bold), ...tokens("c")]

		expect(shape(oldTokens, newTokens)).toEqual([
			["delete", "abc"],
			["insert", "abc"],
		])
	})
})
