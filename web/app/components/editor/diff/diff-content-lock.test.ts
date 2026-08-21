import { Editor } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it } from "vitest"
import {
	DIFF_COMMENT_TX_META,
	DIFF_RECOMPUTE_TX_META,
	DiffContentLock,
} from "./diff-content-lock"

// a headless editor resolves the extension into its plugin through the
// real tiptap wiring; the state is then driven directly because tiptap
// only attaches plugins to mounted editors
function makeState(): EditorState {
	const editor = new Editor({
		element: null,
		extensions: [Document, Paragraph, Text, DiffContentLock],
		content: {
			type: "doc",
			content: [
				{ type: "paragraph", content: [{ type: "text", text: "hello" }] },
			],
		},
	})

	const state = EditorState.create({
		doc: editor.state.doc,
		plugins: editor.extensionManager.plugins,
	})

	editor.destroy()

	return state
}

describe("DiffContentLock", () => {
	it("blocks transactions that change the document", ({ expect }) => {
		const state = makeState()

		const next = state.apply(state.tr.insertText("x", 1))

		expect(next).toBe(state)
		expect(next.doc.textContent).toBe("hello")
	})

	it("allows selection-only transactions", ({ expect }) => {
		const state = makeState()

		const next = state.apply(
			state.tr.setSelection(TextSelection.create(state.doc, 3)),
		)

		expect(next.selection.from).toBe(3)
	})

	it.for([
		{ name: "comment", meta: DIFF_COMMENT_TX_META },
		{ name: "recompute", meta: DIFF_RECOMPUTE_TX_META },
	])(
		"allows document changes marked as a $name transaction",
		({ meta }, { expect }) => {
			const state = makeState()

			const next = state.apply(state.tr.insertText("x", 1).setMeta(meta, true))

			expect(next.doc.textContent).toBe("xhello")
		},
	)
})
