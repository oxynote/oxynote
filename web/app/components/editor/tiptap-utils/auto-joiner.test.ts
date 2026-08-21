import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import {
	BulletList,
	ListItem,
	OrderedList,
	TaskItem,
	TaskList,
} from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Transaction } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it } from "vitest"
import AutoJoiner from "./auto-joiner"
import { childCountShape, findPluginByKey } from "./test-helpers"
import { paragraph } from "../test-helpers"

function list(type: string, itemType: string, text: string): JSONContent {
	return {
		type,
		content: [{ type: itemType, content: [paragraph(text)] }],
	}
}

function blockquote(text: string): JSONContent {
	return { type: "blockquote", content: [paragraph(text)] }
}

// creates a state whose only plugin is the auto joiner, so applying a
// transaction runs its appendTransaction hook exactly like in a live
// editor
function makeState(
	content: JSONContent[],
	elementsToJoin: string[] = [],
): EditorState {
	const editor = new Editor({
		extensions: [
			Document,
			Paragraph,
			Text,
			BulletList,
			OrderedList,
			ListItem,
			TaskList,
			TaskItem,
			Blockquote,
			AutoJoiner.configure({ elementsToJoin }),
		],
		// json content, because the default empty-string content would
		// send the headless editor down the window-dependent html path
		content: { type: "doc", content },
	})

	return EditorState.create({
		doc: editor.state.doc,
		plugins: [findPluginByKey(editor, "autoJoiner")],
	})
}

// deletes the entire second top-level block
function deleteMiddleBlock(state: EditorState): Transaction {
	const from = state.doc.child(0).nodeSize
	const to = from + state.doc.child(1).nodeSize

	return state.tr.delete(from, to)
}

describe("AutoJoiner", () => {
	it.for([
		{ listType: "bulletList", itemType: "listItem" },
		{ listType: "orderedList", itemType: "listItem" },
		{ listType: "taskList", itemType: "taskItem" },
	])(
		"joins two $listType nodes when the block between them is deleted",
		({ listType, itemType }, { expect }) => {
			const state = makeState([
				list(listType, itemType, "a"),
				paragraph("x"),
				list(listType, itemType, "b"),
			])

			const { state: next, transactions } = state.applyTransaction(
				deleteMiddleBlock(state),
			)

			expect(childCountShape(next.doc)).toEqual([`${listType}:2`])
			expect(transactions).toHaveLength(2)
		},
	)

	it("leaves adjacent lists of different types separate", ({ expect }) => {
		const state = makeState([
			list("bulletList", "listItem", "a"),
			paragraph("x"),
			list("orderedList", "listItem", "b"),
		])

		const { state: next, transactions } = state.applyTransaction(
			deleteMiddleBlock(state),
		)

		expect(childCountShape(next.doc)).toEqual(["bulletList:1", "orderedList:1"])
		expect(transactions).toHaveLength(1)
	})

	it("leaves node types outside the default set separate", ({ expect }) => {
		const state = makeState([blockquote("a"), paragraph("x"), blockquote("b")])

		const { state: next, transactions } = state.applyTransaction(
			deleteMiddleBlock(state),
		)

		expect(childCountShape(next.doc)).toEqual(["blockquote:1", "blockquote:1"])
		expect(transactions).toHaveLength(1)
	})

	it("joins extra node types listed in elementsToJoin", ({ expect }) => {
		const state = makeState(
			[blockquote("a"), paragraph("x"), blockquote("b")],
			["blockquote"],
		)

		const { state: next, transactions } = state.applyTransaction(
			deleteMiddleBlock(state),
		)

		expect(childCountShape(next.doc)).toEqual(["blockquote:2"])
		expect(transactions).toHaveLength(2)
	})

	it("ignores unknown node names in elementsToJoin", ({ expect }) => {
		const state = makeState(
			[
				list("bulletList", "listItem", "a"),
				paragraph("x"),
				list("bulletList", "listItem", "b"),
			],
			["bogus"],
		)

		const { state: next } = state.applyTransaction(deleteMiddleBlock(state))

		expect(childCountShape(next.doc)).toEqual(["bulletList:2"])
	})

	it("keeps lists separate while a block remains between them", ({
		expect,
	}) => {
		const state = makeState([
			list("bulletList", "listItem", "a"),
			paragraph("x"),
			list("bulletList", "listItem", "b"),
		])

		// delete only the paragraph's text, leaving the empty paragraph
		// itself between the two lists
		const from = state.doc.child(0).nodeSize + 1
		const { state: next, transactions } = state.applyTransaction(
			state.tr.delete(from, from + 1),
		)

		expect(childCountShape(next.doc)).toEqual([
			"bulletList:1",
			"paragraph:0",
			"bulletList:1",
		])
		expect(transactions).toHaveLength(1)
	})

	it("ignores transactions that do not change the document", ({ expect }) => {
		const state = makeState([
			list("bulletList", "listItem", "a"),
			list("bulletList", "listItem", "b"),
		])

		const { state: next, transactions } = state.applyTransaction(
			state.tr.setSelection(TextSelection.create(state.doc, 3)),
		)

		expect(next.doc.eq(state.doc)).toBe(true)
		expect(transactions).toHaveLength(1)
	})
})
