import type { JSONContent } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { describe, expect, it } from "vitest"
import {
	buildPositionMap,
	buildPositionMapFromDoc,
	DiffStatus,
	segmentSelectionByDiffStatus,
	type PositionMap,
} from "./position-map"

function paragraph(text: string, attrs?: JSONContent["attrs"]): JSONContent {
	return {
		type: "paragraph",
		attrs,
		content: text === "" ? undefined : [{ type: "text", text }],
	}
}

function doc(...content: JSONContent[]): JSONContent {
	return { type: "doc", content }
}

// minimal schema for the live-document variants: block attrs mirror the
// ones computeMergedDocument stamps, marks mirror the inline diff marks
const schema = new Schema({
	nodes: {
		doc: { content: "block*" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: {
				diffStatus: { default: null },
				modifiedIndex: { default: null },
				originalIndex: { default: null },
				uid: { default: null },
			},
		},
		text: { group: "inline" },
	},
	marks: {
		diffTextAdded: {},
		diffTextRemoved: {},
	},
})

function pmParagraph(
	attrs: Record<string, unknown> | null,
	...content: PMNode[]
): PMNode {
	return schema.nodes.paragraph.create(attrs, content)
}

function pmDoc(...content: PMNode[]): PMNode {
	return schema.nodes.doc.create(null, content)
}

describe("buildPositionMap", () => {
	it("returns no entries for an empty document", () => {
		expect(buildPositionMap({ type: "doc" })).toEqual([])
	})

	it("accumulates positions from estimated node sizes", () => {
		// sizes: "ab" -> 2 + 2 = 4, "" -> 2, nested paragraph -> 2 + (2 + 2) = 6
		const map = buildPositionMap(
			doc(paragraph("ab"), paragraph(""), {
				type: "blockquote",
				content: [paragraph("cd")],
			}),
		)

		expect(map.map((e) => [e.startPos, e.nodeSize])).toEqual([
			[0, 4],
			[4, 2],
			[6, 6],
		])
	})

	it("defaults attribute-less blocks to unchanged modified blocks", () => {
		const [entry] = buildPositionMap(doc(paragraph("ab")))

		expect(entry).toEqual({
			source: "modified",
			blockIndex: 0,
			startPos: 0,
			nodeSize: 4,
			diffStatus: DiffStatus.Unchanged,
			uid: null,
		})
	})

	it.for([
		{
			name: "maps a removed block to its original index",
			attrs: {
				diffStatus: DiffStatus.Removed,
				originalIndex: 3,
				modifiedIndex: null,
				uid: "u1",
			},
			expected: {
				source: "original",
				blockIndex: 3,
				diffStatus: DiffStatus.Removed,
				uid: "u1",
			},
		},
		{
			name: "maps an added block to its modified index",
			attrs: {
				diffStatus: DiffStatus.Added,
				originalIndex: null,
				modifiedIndex: 2,
				uid: "u2",
			},
			expected: {
				source: "modified",
				blockIndex: 2,
				diffStatus: DiffStatus.Added,
				uid: "u2",
			},
		},
		{
			name: "maps a modified block to its modified index",
			attrs: {
				diffStatus: DiffStatus.Modified,
				originalIndex: 1,
				modifiedIndex: 4,
				uid: "u3",
			},
			expected: {
				source: "modified",
				blockIndex: 4,
				diffStatus: DiffStatus.Modified,
				uid: "u3",
			},
		},
	])("$name", ({ attrs, expected }, { expect }) => {
		const [entry] = buildPositionMap(doc(paragraph("ab", attrs)))

		expect(entry).toMatchObject(expected)
	})
})

describe("buildPositionMapFromDoc", () => {
	it("returns no entries for an empty document", () => {
		expect(buildPositionMapFromDoc(pmDoc())).toEqual([])
	})

	it("records real node positions and sizes", () => {
		const map = buildPositionMapFromDoc(
			pmDoc(pmParagraph(null, schema.text("ab")), pmParagraph(null)),
		)

		expect(map.map((e) => [e.startPos, e.nodeSize])).toEqual([
			[0, 4],
			[4, 2],
		])
	})

	it("defaults attribute-less blocks to unchanged modified blocks", () => {
		const [entry] = buildPositionMapFromDoc(
			pmDoc(pmParagraph(null, schema.text("ab"))),
		)

		expect(entry).toEqual({
			source: "modified",
			blockIndex: 0,
			startPos: 0,
			nodeSize: 4,
			diffStatus: DiffStatus.Unchanged,
			uid: null,
		})
	})

	it.for([
		{
			name: "maps a removed block to its original index",
			attrs: {
				diffStatus: DiffStatus.Removed,
				originalIndex: 3,
				modifiedIndex: null,
				uid: "u1",
			},
			expected: {
				source: "original",
				blockIndex: 3,
				diffStatus: DiffStatus.Removed,
				uid: "u1",
			},
		},
		{
			name: "maps an added block to its modified index",
			attrs: {
				diffStatus: DiffStatus.Added,
				originalIndex: null,
				modifiedIndex: 2,
				uid: "u2",
			},
			expected: {
				source: "modified",
				blockIndex: 2,
				diffStatus: DiffStatus.Added,
				uid: "u2",
			},
		},
		{
			name: "maps a modified block to its modified index",
			attrs: {
				diffStatus: DiffStatus.Modified,
				originalIndex: 1,
				modifiedIndex: 4,
				uid: "u3",
			},
			expected: {
				source: "modified",
				blockIndex: 4,
				diffStatus: DiffStatus.Modified,
				uid: "u3",
			},
		},
	])("$name", ({ attrs, expected }, { expect }) => {
		const [entry] = buildPositionMapFromDoc(
			pmDoc(pmParagraph(attrs, schema.text("ab"))),
		)

		expect(entry).toMatchObject(expected)
	})
})

describe("segmentSelectionByDiffStatus", () => {
	// three adjacent blocks: [0, 4), [4, 6), [6, 12)
	const map: PositionMap = [
		{
			source: "modified",
			blockIndex: 0,
			startPos: 0,
			nodeSize: 4,
			diffStatus: DiffStatus.Unchanged,
			uid: "a",
		},
		{
			source: "modified",
			blockIndex: 1,
			startPos: 4,
			nodeSize: 2,
			diffStatus: DiffStatus.Added,
			uid: "b",
		},
		{
			source: "original",
			blockIndex: 1,
			startPos: 6,
			nodeSize: 6,
			diffStatus: DiffStatus.Removed,
			uid: "c",
		},
	]

	it("splits a selection spanning several blocks into one segment each", () => {
		expect(segmentSelectionByDiffStatus(map, 2, 8)).toEqual([
			{
				status: DiffStatus.Unchanged,
				nodeUid: "a",
				from: 2,
				to: 4,
				fromOffset: 2,
				toOffset: 4,
			},
			{
				status: DiffStatus.Added,
				nodeUid: "b",
				from: 4,
				to: 6,
				fromOffset: 0,
				toOffset: 2,
			},
			{
				status: DiffStatus.Removed,
				nodeUid: "c",
				from: 6,
				to: 8,
				fromOffset: 0,
				toOffset: 2,
			},
		])
	})

	it("clamps a segment to the block it falls inside", () => {
		expect(segmentSelectionByDiffStatus(map, 5, 6)).toEqual([
			{
				status: DiffStatus.Added,
				nodeUid: "b",
				from: 5,
				to: 6,
				fromOffset: 1,
				toOffset: 2,
			},
		])
	})

	it("returns no segments for a selection beyond the mapped range", () => {
		expect(segmentSelectionByDiffStatus(map, 12, 20)).toEqual([])
	})

	it("returns no segments for an empty selection", () => {
		expect(segmentSelectionByDiffStatus(map, 4, 4)).toEqual([])
	})

	it("treats a modified entry as a single segment without a document", () => {
		const modifiedMap: PositionMap = [
			{
				source: "modified",
				blockIndex: 0,
				startPos: 0,
				nodeSize: 6,
				diffStatus: DiffStatus.Modified,
				uid: "m1",
			},
		]

		expect(segmentSelectionByDiffStatus(modifiedMap, 1, 5)).toEqual([
			{
				status: DiffStatus.Modified,
				nodeUid: "m1",
				from: 1,
				to: 5,
				fromOffset: 1,
				toOffset: 5,
			},
		])
	})

	it("splits a modified block into sub-segments by inline diff marks", () => {
		// paragraph layout: open token at 0, "keep" [1,5), removed "old"
		// [5,8), added "new" [8,11), close token at 11
		const para = pmParagraph(
			{ diffStatus: DiffStatus.Modified, modifiedIndex: 0, uid: "m1" },
			schema.text("keep"),
			schema.text("old", [schema.marks.diffTextRemoved.create()]),
			schema.text("new", [schema.marks.diffTextAdded.create()]),
		)
		const document = pmDoc(para)
		const docMap = buildPositionMapFromDoc(document)

		const segments = segmentSelectionByDiffStatus(
			docMap,
			0,
			para.nodeSize,
			document,
			{
				removed: schema.marks.diffTextRemoved,
				added: schema.marks.diffTextAdded,
			},
		)

		expect(segments).toEqual([
			// gap before the first text node (the paragraph's open token)
			{
				status: DiffStatus.Modified,
				nodeUid: "m1",
				from: 0,
				to: 1,
				fromOffset: 0,
				toOffset: 1,
			},
			{
				status: DiffStatus.Unchanged,
				nodeUid: "m1",
				from: 1,
				to: 5,
				fromOffset: 1,
				toOffset: 5,
			},
			{
				status: DiffStatus.Removed,
				nodeUid: "m1",
				from: 5,
				to: 8,
				fromOffset: 5,
				toOffset: 8,
			},
			{
				status: DiffStatus.Added,
				nodeUid: "m1",
				from: 8,
				to: 11,
				fromOffset: 8,
				toOffset: 11,
			},
			// trailing gap after the last text node (the close token)
			{
				status: DiffStatus.Modified,
				nodeUid: "m1",
				from: 11,
				to: 12,
				fromOffset: 11,
				toOffset: 12,
			},
		])
	})
})
