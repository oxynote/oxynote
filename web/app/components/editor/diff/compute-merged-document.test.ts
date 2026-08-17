import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { computeMergedDocument } from "./compute-merged-document"
import { DiffStatus } from "./position-map"

function paragraph(text: string, uid?: string): JSONContent {
	return {
		type: "paragraph",
		attrs: uid === undefined ? {} : { uid },
		content: [{ type: "text", text }],
	}
}

function doc(...content: JSONContent[]): JSONContent {
	return { type: "doc", content }
}

// compresses the merged doc into [status, text] pairs for readable
// assertions; the inline expansion of modified blocks makes their full
// content shape a concern of expandInlineDiffs, not of this suite
function shape(result: { doc: JSONContent }): [unknown, string][] {
	return (result.doc.content ?? []).map((block) => [
		block.attrs?.diffStatus,
		(block.content ?? [])
			.map((child) => (child.type === "text" ? child.text : ""))
			.join(""),
	])
}

describe("computeMergedDocument", () => {
	it("marks every block of identical documents as unchanged", () => {
		const original = doc(paragraph("one", "u1"), paragraph("two", "u2"))
		const modified = doc(paragraph("one", "u1"), paragraph("two", "u2"))

		const result = computeMergedDocument(original, modified)

		expect(shape(result)).toEqual([
			[DiffStatus.Unchanged, "one"],
			[DiffStatus.Unchanged, "two"],
		])
		expect(result.positionMap.map((e) => e.diffStatus)).toEqual([
			DiffStatus.Unchanged,
			DiffStatus.Unchanged,
		])
	})

	it("marks a block only present in the modified document as added", () => {
		const original = doc(paragraph("one", "u1"))
		const modified = doc(paragraph("one", "u1"), paragraph("two", "u2"))

		const result = computeMergedDocument(original, modified)

		expect(shape(result)).toEqual([
			[DiffStatus.Unchanged, "one"],
			[DiffStatus.Added, "two"],
		])

		const added = result.doc.content?.[1]
		expect(added?.attrs?.modifiedIndex).toBe(1)
		expect(added?.attrs?.originalIndex).toBeNull()
	})

	it("keeps a block only present in the original document as removed, in place", () => {
		const original = doc(
			paragraph("one", "u1"),
			paragraph("gone", "u2"),
			paragraph("three", "u3"),
		)
		const modified = doc(paragraph("one", "u1"), paragraph("three", "u3"))

		const result = computeMergedDocument(original, modified)

		expect(shape(result)).toEqual([
			[DiffStatus.Unchanged, "one"],
			[DiffStatus.Removed, "gone"],
			[DiffStatus.Unchanged, "three"],
		])

		const removed = result.doc.content?.[1]
		expect(removed?.attrs?.originalIndex).toBe(1)
		expect(removed?.attrs?.modifiedIndex).toBeNull()
	})

	it("marks a uid-matched block with changed content as modified and expands the inline diff", () => {
		const original = doc(paragraph("before", "u1"))
		const modified = doc(paragraph("after", "u1"))

		const result = computeMergedDocument(original, modified)

		const block = result.doc.content?.[0]
		expect(block?.attrs?.diffStatus).toBe(DiffStatus.Modified)
		expect(block?.attrs?.originalIndex).toBe(0)
		expect(block?.attrs?.modifiedIndex).toBe(0)
		// the inline expansion consumes oldNode and materializes the old
		// text as marked content instead
		expect(block?.attrs?.oldNode).toBeNull()
		expect(block?.content).toEqual([
			{
				type: "text",
				text: "before",
				marks: [{ type: "diffTextRemoved" }],
			},
			{
				type: "text",
				text: "after",
				marks: [{ type: "diffTextAdded" }],
			},
		])
	})

	it("matches blocks without uids by content through the LCS fallback", () => {
		const original = doc(paragraph("same"), paragraph("gone"))
		const modified = doc(paragraph("same"), paragraph("new"))

		const result = computeMergedDocument(original, modified)

		expect(shape(result)).toEqual([
			[DiffStatus.Unchanged, "same"],
			[DiffStatus.Added, "new"],
			[DiffStatus.Removed, "gone"],
		])
	})

	it("shows a reordered pair as one kept block plus a remove and an add", () => {
		const original = doc(paragraph("alpha", "u1"), paragraph("beta", "u2"))
		const modified = doc(paragraph("beta", "u2"), paragraph("alpha", "u1"))

		const result = computeMergedDocument(original, modified)
		const statuses = shape(result)

		const count = (status: DiffStatus) =>
			statuses.filter(([s]) => s === status).length

		expect(statuses).toHaveLength(3)
		expect(count(DiffStatus.Unchanged)).toBe(1)
		expect(count(DiffStatus.Added)).toBe(1)
		expect(count(DiffStatus.Removed)).toBe(1)
	})

	it("aligns the position map with the merged content", () => {
		const original = doc(paragraph("one", "u1"), paragraph("gone", "u2"))
		const modified = doc(paragraph("one", "u1"), paragraph("new", "u3"))

		const result = computeMergedDocument(original, modified)

		expect(result.positionMap.map((e) => [e.diffStatus, e.uid])).toEqual([
			[DiffStatus.Unchanged, "u1"],
			[DiffStatus.Added, "u3"],
			[DiffStatus.Removed, "u2"],
		])
	})

	it("produces the golden merged document for a mixed change set", ({
		expect,
	}) => {
		const original = doc(
			paragraph("kept", "u1"),
			paragraph("dropped", "u2"),
			paragraph("edited before", "u3"),
		)
		const modified = doc(
			paragraph("kept", "u1"),
			paragraph("edited after", "u3"),
			paragraph("brand new", "u4"),
		)

		expect(computeMergedDocument(original, modified).doc).toMatchSnapshot()
	})
})
