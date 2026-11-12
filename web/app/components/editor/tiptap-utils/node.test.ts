import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import { ContentDocument } from "../blocks/document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { deleteNode, stripNode } from "./node"
import { childCountShape } from "./test-helpers"
import { MetricBlockStub, BlockGridStub, paragraph } from "../test-helpers"

function blockGrid(blockCount: number): JSONContent {
	return {
		type: "blockGrid",
		content: Array.from({ length: blockCount }, () => ({
			type: "metricBlock",
		})),
	}
}

// without an element the editor never mounts a view, so dispatching
// through the headless view proxy applies transactions straight to the
// editor state
function makeEditor(content: JSONContent[]): Editor {
	return new Editor({
		extensions: [
			ContentDocument,
			Paragraph,
			Text,
			BlockGridStub,
			MetricBlockStub,
		],
		content: { type: "doc", content },
	})
}

// returns the absolute position of the top-level block at the given
// index
function posOfBlock(doc: Node, index: number): number {
	let pos = 0

	for (let i = 0; i < index; i++) {
		pos += doc.child(i).nodeSize
	}

	return pos
}

describe("deleteNode", () => {
	it("deletes the node at the given position", ({ expect }) => {
		const editor = makeEditor([paragraph("one"), paragraph("two")])

		const result = deleteNode(editor, posOfBlock(editor.state.doc, 1))

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual(["paragraph:1"])
		expect(editor.state.doc.textContent).toBe("one")
	})

	it("returns false and keeps the document when no node exists at the position", ({
		expect,
	}) => {
		const editor = makeEditor([paragraph("one"), paragraph("two")])

		const result = deleteNode(editor, editor.state.doc.content.size)

		expect(result).toBe(false)
		expect(childCountShape(editor.state.doc)).toEqual([
			"paragraph:1",
			"paragraph:1",
		])
	})

	it("removes a metric grid emptied by the deletion", ({ expect }) => {
		const editor = makeEditor([paragraph("one"), blockGrid(1)])
		const blockPos = posOfBlock(editor.state.doc, 1) + 1

		const result = deleteNode(editor, blockPos)

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual(["paragraph:1"])
	})

	it("keeps a metric grid that still has blocks after the deletion", ({
		expect,
	}) => {
		const editor = makeEditor([paragraph("one"), blockGrid(2)])
		const blockPos = posOfBlock(editor.state.doc, 1) + 1

		const result = deleteNode(editor, blockPos)

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual([
			"paragraph:1",
			"blockGrid:1",
		])
	})

	it("sweeps unrelated empty metric grids in the same transaction", ({
		expect,
	}) => {
		const editor = makeEditor([
			paragraph("one"),
			blockGrid(0),
			paragraph("two"),
		])

		const result = deleteNode(editor, posOfBlock(editor.state.doc, 2))

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual(["paragraph:1"])
	})
})

describe("stripNode", () => {
	it("drops the uid and the comment of the node and of every node inside it", ({
		expect,
	}) => {
		const stripped = stripNode({
			type: "callout",
			attrs: { uid: "c-1", nodeCommentId: "t-1", tone: "info" },
			content: [
				{
					type: "paragraph",
					attrs: { uid: "p-1" },
					content: [
						{
							type: "text",
							text: "hi",
							marks: [{ type: "comment" }, { type: "bold" }],
						},
					],
				},
			],
		})

		expect(stripped).toEqual({
			type: "callout",
			attrs: { uid: null, nodeCommentId: null, tone: "info" },
			content: [
				{
					type: "paragraph",
					attrs: { uid: null },
					content: [{ type: "text", text: "hi", marks: [{ type: "bold" }] }],
				},
			],
		})
	})

	it("leaves a node without a uid or a comment as it is", ({ expect }) => {
		expect(stripNode({ type: "paragraph", attrs: { level: 1 } })).toEqual({
			type: "paragraph",
			attrs: { level: 1 },
		})
	})
})
