// shared helpers for the link menu suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import type { Extensions } from "@tiptap/core"
import { Editor } from "@tiptap/vue-3"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { LinkMark } from "./link-mark"

// the bubble menu reads link marks straight out of a live document and
// listens on the editor's own dom, so the suite drives a real editor
// attached to the page
export function makeLinkEditor(
	content: string,
	extensions: Extensions = [],
): Editor {
	const element = document.createElement("div")
	document.body.appendChild(element)

	return new Editor({
		element: element,
		extensions: [Document, Paragraph, Text, LinkMark, ...extensions],
		content: content,
	})
}

export function destroyLinkEditor(editor: Editor) {
	const element = editor.view.dom.parentElement

	editor.destroy()
	element?.remove()
}

export function linkElement(editor: Editor): HTMLElement {
	const anchor = editor.view.dom.querySelector("a")
	if (!anchor) {
		throw new Error("the test document holds no link")
	}

	return anchor
}
