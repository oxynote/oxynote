import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { expandInlineDiffs } from "./inline-diff-expansion"
import { DiffStatus } from "./position-map"

function text(
	t: string,
	marks?: NonNullable<JSONContent["marks"]>,
): JSONContent {
	return marks === undefined
		? { type: "text", text: t }
		: { type: "text", text: t, marks }
}

// builds a modified block carrying its old content in the oldNode attr,
// the shape computeMergedDocument hands to the expansion
function modified(
	content: JSONContent[] | undefined,
	oldContent: JSONContent[] | undefined,
	type = "paragraph",
): JSONContent {
	return {
		type,
		attrs: {
			diffStatus: DiffStatus.Modified,
			oldNode: { type, content: oldContent },
		},
		content,
	}
}

function doc(...content: JSONContent[]): JSONContent {
	return { type: "doc", content }
}

describe("expandInlineDiffs", () => {
	it("returns the same document object when no block is modified", () => {
		const input = doc({ type: "paragraph", content: [text("plain")] })

		expect(expandInlineDiffs(input)).toBe(input)
	})

	it("expands a modified textblock into removed and added runs", () => {
		const result = expandInlineDiffs(
			doc(modified([text("after")], [text("before")])),
		)

		const block = result.content?.[0]
		expect(block?.attrs?.oldNode).toBeNull()
		expect(block?.attrs?.diffStatus).toBe(DiffStatus.Modified)
		expect(block?.content).toEqual([
			{ type: "text", text: "before", marks: [{ type: "diffTextRemoved" }] },
			{ type: "text", text: "after", marks: [{ type: "diffTextAdded" }] },
		])
	})

	it("keeps unchanged text unmarked and marks only the insertion", () => {
		const result = expandInlineDiffs(
			doc(modified([text("keep new")], [text("keep")])),
		)

		expect(result.content?.[0]?.content).toEqual([
			{ type: "text", text: "keep" },
			{ type: "text", text: " new", marks: [{ type: "diffTextAdded" }] },
		])
	})

	it("preserves comment marks on kept text and strips them from removed text", () => {
		const comment = { type: "comment", attrs: { commentId: "c1" } }
		const result = expandInlineDiffs(
			doc(modified([text("keep", [comment])], [text("keep gone", [comment])])),
		)

		expect(result.content?.[0]?.content).toEqual([
			{ type: "text", text: "keep", marks: [comment] },
			{ type: "text", text: " gone", marks: [{ type: "diffTextRemoved" }] },
		])
	})

	it("merges runs from separate source text nodes with identical marks", () => {
		const result = expandInlineDiffs(
			doc(modified([text("ab"), text("cd")], [text("abcd")])),
		)

		expect(result.content?.[0]?.content).toEqual([
			{ type: "text", text: "abcd" },
		])
	})

	it("materializes fully deleted content as removed text", () => {
		const result = expandInlineDiffs(doc(modified(undefined, [text("gone")])))

		expect(result.content?.[0]?.content).toEqual([
			{ type: "text", text: "gone", marks: [{ type: "diffTextRemoved" }] },
		])
	})

	it("leaves textblocks with non-text inline content unexpanded", () => {
		const block = modified([text("a"), { type: "hardBreak" }], [text("b")])
		const input = doc(block)

		const result = expandInlineDiffs(input)

		expect(result).toBe(input)
		expect(result.content?.[0]?.attrs?.oldNode).toEqual({
			type: "paragraph",
			content: [text("b")],
		})
	})

	it("leaves contentless atom nodes for component-level diffing", () => {
		const block = modified(undefined, undefined, "metricBlock")
		const input = doc(block)

		const result = expandInlineDiffs(input)

		expect(result).toBe(input)
		expect(result.content?.[0]?.attrs?.oldNode).toEqual({
			type: "metricBlock",
			content: undefined,
		})
	})

	it("records the new text on blocks with modified text content", () => {
		const result = expandInlineDiffs(
			doc(modified([text("graph TD")], [text("graph LR")], "mermaidBlock")),
		)

		expect(result.content?.[0]?.attrs?.modifiedTextContent).toBe("graph TD")
	})

	it("expands modified blocks nested inside containers", () => {
		const input = doc({
			type: "blockquote",
			content: [modified([text("after")], [text("before")])],
		})

		const result = expandInlineDiffs(input)

		expect(result).not.toBe(input)
		const nested = result.content?.[0]?.content?.[0]
		expect(nested?.attrs?.oldNode).toBeNull()
		expect(nested?.content).toEqual([
			{ type: "text", text: "before", marks: [{ type: "diffTextRemoved" }] },
			{ type: "text", text: "after", marks: [{ type: "diffTextAdded" }] },
		])
	})
})
