import type { JSONContent } from "@tiptap/core"
import { describe, expect, it } from "vitest"
import { computeMergedDocument } from "./compute-merged-document"
import { DiffStatus } from "./position-map"
import { doc } from "../test-helpers"

function paragraph(text: string, uid?: string): JSONContent {
	return {
		type: "paragraph",
		attrs: uid === undefined ? {} : { uid },
		content: [{ type: "text", text }],
	}
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

	it.for([
		{
			name: "a metric block whose only change is its simulation preset as modified",
			original: { simulationPreset: "cpu_usage", simulationActive: false },
			modified: { simulationPreset: "error_rate", simulationActive: false },
			expected: DiffStatus.Modified,
		},
		{
			name: "a metric block whose only change is its simulation flag as unchanged",
			original: { simulationPreset: "cpu_usage", simulationActive: true },
			modified: { simulationPreset: "cpu_usage", simulationActive: false },
			expected: DiffStatus.Unchanged,
		},
	])("marks $name", ({ original, modified, expected }) => {
		const grid = (attrs: Record<string, unknown>): JSONContent => ({
			type: "blockGrid",
			attrs: { uid: "g1" },
			content: [{ type: "metricBlock", attrs: { uid: "m1", ...attrs } }],
		})

		const result = computeMergedDocument(
			doc(grid(original)),
			doc(grid(modified)),
		)

		expect(result.doc.content?.[0]?.content?.[0]?.attrs?.diffStatus).toBe(
			expected,
		)
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

	describe("when blocks sit in a grid", () => {
		it("marks a block a drop pushed down as modified", () => {
			const result = computeMergedDocument(
				doc(grid(block("a", 0, 0, 12), block("b", 12, 0, 12))),
				doc(
					grid(
						block("a", 0, 0, 12),
						block("x", 12, 0, 12),
						block("b", 12, 1, 12),
					),
				),
			)

			expect(gridShape(result.doc.content?.[0])).toEqual([
				[DiffStatus.Unchanged, "a"],
				[DiffStatus.Added, "x"],
				[DiffStatus.Modified, "b"],
			])
		})

		it("marks blocks that show in the same places as unchanged however they store them", () => {
			const result = computeMergedDocument(
				doc(grid(legacy("a"), legacy("b"))),
				doc(grid(block("a", 0, 0, 12), block("b", 12, 0, 12))),
			)

			expect(gridShape(result.doc.content?.[0])).toEqual([
				[DiffStatus.Unchanged, "a"],
				[DiffStatus.Unchanged, "b"],
			])
		})

		it("marks blocks whose only change is their stacked place as modified", () => {
			const result = computeMergedDocument(
				doc(grid(block("a", 0, 0, 12), block("b", 12, 0, 12))),
				doc(
					grid(
						block("a", 0, 0, 12, { stackPos: 1 }),
						block("b", 12, 0, 12, { stackPos: 0 }),
					),
				),
			)

			expect(gridShape(result.doc.content?.[0])).toEqual([
				[DiffStatus.Modified, "a"],
				[DiffStatus.Modified, "b"],
			])
		})

		it("keeps a block that only moved in the document as unchanged instead of removed and added", () => {
			const result = computeMergedDocument(
				doc(grid(block("a", 0, 0, 12), block("b", 12, 0, 12))),
				doc(grid(block("b", 12, 0, 12), block("a", 0, 0, 12))),
			)

			expect(gridShape(result.doc.content?.[0])).toEqual([
				[DiffStatus.Unchanged, "b"],
				[DiffStatus.Unchanged, "a"],
			])
		})

		it("gives a block whose only change is its size the same places on both sides", () => {
			const result = computeMergedDocument(
				doc(grid(block("a", 0, 0, 8))),
				doc(grid(block("a", 0, 0, 12))),
			)
			const merged = result.doc.content?.[0]?.content?.[0]
			const oldNode = merged?.attrs?.oldNode as JSONContent | undefined

			expect(merged?.attrs?.diffStatus).toBe(DiffStatus.Modified)
			expect(merged?.attrs?.gridPlace).toEqual(oldNode?.attrs?.gridPlace)
			expect(merged?.attrs?.stackPlace).toBe(oldNode?.attrs?.stackPlace)
		})

		it("moves a removed block into a copy of its grid right after it", () => {
			const result = computeMergedDocument(
				doc(grid(block("a", 0, 0, 12), block("b", 12, 0, 12))),
				doc(grid(block("a", 0, 0, 12))),
			)
			const removed = result.doc.content?.[1]?.content?.[0]

			expect(result.doc.content?.map(gridShape)).toEqual([
				[[DiffStatus.Unchanged, "a"]],
				[[DiffStatus.Removed, "b"]],
			])
			expect(result.doc.content?.[1]?.attrs?.uid).toBe("g1-removed")
			expect(removed?.attrs).toMatchObject({
				widePosX: null,
				widePosY: null,
				stackPos: null,
				gridPlace: null,
				stackPlace: null,
			})
			expect(
				(removed?.attrs?.oldNode as JSONContent | undefined)?.attrs,
			).toMatchObject({ gridPlace: { row: 0, column: 12 }, stackPlace: 1 })
		})
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

// a grid of metric blocks. The diff places them by their cells.
function grid(...blocks: JSONContent[]): JSONContent {
	return { type: "blockGrid", attrs: { uid: "g1" }, content: blocks }
}

function block(
	uid: string,
	x: number,
	y: number,
	w: number,
	attrs: Record<string, unknown> = {},
): JSONContent {
	return {
		type: "metricBlock",
		attrs: { uid, widePosX: x, widePosY: y, width: w, height: 1, ...attrs },
	}
}

// a block saved before blocks stored their cells
function legacy(uid: string): JSONContent {
	return { type: "metricBlock", attrs: { uid, width: "standard" } }
}

// the status and uid of each block of a merged grid
function gridShape(grid: JSONContent | undefined): [unknown, unknown][] {
	return (grid?.content ?? []).map((child) => [
		child.attrs?.diffStatus,
		child.attrs?.uid,
	])
}
