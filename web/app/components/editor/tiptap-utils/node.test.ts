import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { deleteNode } from "./node"
import { childCountShape } from "./test-helpers"
import { MetricBlockStub, MetricGridStub, paragraph } from "../test-helpers"

function metricGrid(blockCount: number): JSONContent {
	return {
		type: "metricGrid",
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
		extensions: [Document, Paragraph, Text, MetricGridStub, MetricBlockStub],
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
		const editor = makeEditor([paragraph("one"), metricGrid(1)])
		const blockPos = posOfBlock(editor.state.doc, 1) + 1

		const result = deleteNode(editor, blockPos)

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual(["paragraph:1"])
	})

	it("keeps a metric grid that still has blocks after the deletion", ({
		expect,
	}) => {
		const editor = makeEditor([paragraph("one"), metricGrid(2)])
		const blockPos = posOfBlock(editor.state.doc, 1) + 1

		const result = deleteNode(editor, blockPos)

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual([
			"paragraph:1",
			"metricGrid:1",
		])
	})

	it("sweeps unrelated empty metric grids in the same transaction", ({
		expect,
	}) => {
		const editor = makeEditor([
			paragraph("one"),
			metricGrid(0),
			paragraph("two"),
		])

		const result = deleteNode(editor, posOfBlock(editor.state.doc, 2))

		expect(result).toBe(true)
		expect(childCountShape(editor.state.doc)).toEqual(["paragraph:1"])
	})
})
