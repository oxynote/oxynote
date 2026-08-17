import { Schema } from "@tiptap/pm/model"
import { describe, expect, it } from "vitest"
import { extractTokensFromJSON, extractTokensFromPMNode } from "./diff-tokens"

// minimal schema mirroring the inline shapes the extractors see: text
// with marks plus a non-text inline node (hard break)
const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		hardBreak: { group: "inline", inline: true },
		text: { group: "inline" },
	},
	marks: {
		bold: {},
		comment: {},
	},
})

describe("extractTokensFromJSON", () => {
	it("returns no tokens for a node without inline content", () => {
		expect(extractTokensFromJSON({ type: "paragraph" })).toEqual([])
	})

	it("emits one token per character sharing the source marks", () => {
		const marks = [{ type: "bold" }]
		const tokens = extractTokensFromJSON({
			type: "paragraph",
			content: [{ type: "text", text: "ab", marks }],
		})

		expect(tokens).toEqual([
			{ text: "a", marks: [{ type: "bold" }] },
			{ text: "b", marks: [{ type: "bold" }] },
		])

		// tokens from one text node share the marks array reference —
		// tokensToJSON in inline-diff-expansion relies on this to group runs
		expect(tokens[0]?.marks).toBe(tokens[1]?.marks)
	})

	it("strips comment marks but keeps others", () => {
		const tokens = extractTokensFromJSON({
			type: "paragraph",
			content: [
				{
					type: "text",
					text: "a",
					marks: [
						{ type: "comment", attrs: { commentId: "c1" } },
						{ type: "bold" },
					],
				},
			],
		})

		expect(tokens).toEqual([{ text: "a", marks: [{ type: "bold" }] }])
	})

	it("skips non-text inline nodes", () => {
		const tokens = extractTokensFromJSON({
			type: "paragraph",
			content: [
				{ type: "text", text: "a" },
				{ type: "hardBreak" },
				{ type: "text", text: "b" },
			],
		})

		expect(tokens.map((t) => t.text)).toEqual(["a", "b"])
	})

	it("treats an emoji as a single token", () => {
		const tokens = extractTokensFromJSON({
			type: "paragraph",
			content: [{ type: "text", text: "👍!" }],
		})

		expect(tokens.map((t) => t.text)).toEqual(["👍", "!"])
	})
})

describe("extractTokensFromPMNode", () => {
	it("returns no tokens for an empty textblock", () => {
		expect(extractTokensFromPMNode(schema.nodes.paragraph.create())).toEqual([])
	})

	it("emits one token per character with serialized marks", () => {
		const node = schema.nodes.paragraph.create(null, [
			schema.text("ab", [schema.marks.bold.create()]),
		])

		expect(extractTokensFromPMNode(node)).toEqual([
			{ text: "a", marks: [{ type: "bold" }] },
			{ text: "b", marks: [{ type: "bold" }] },
		])
	})

	it("strips comment marks but keeps others", () => {
		const node = schema.nodes.paragraph.create(null, [
			schema.text("a", [
				schema.marks.comment.create(),
				schema.marks.bold.create(),
			]),
		])

		expect(extractTokensFromPMNode(node)).toEqual([
			{ text: "a", marks: [{ type: "bold" }] },
		])
	})

	it("skips non-text inline nodes", () => {
		const node = schema.nodes.paragraph.create(null, [
			schema.text("a"),
			schema.nodes.hardBreak.create(),
			schema.text("b"),
		])

		expect(extractTokensFromPMNode(node).map((t) => t.text)).toEqual(["a", "b"])
	})
})
