import type { JSONContent } from "@tiptap/core"
import { Editor, Node } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { describe, it } from "vitest"
import { IMAGE_BLOCK_NAME } from "~/components/editor/blocks/node-names"
import { DiffAttributes } from "./diff-attributes"
import { DiffDecorations } from "./diff-decorations"
import { DiffStatus } from "./position-map"
import { doc, paragraph } from "~/components/editor/test-helpers"

function heading(text: string, attrs?: JSONContent["attrs"]): JSONContent {
	return {
		type: "heading",
		attrs: { level: 1, ...attrs },
		content: [{ type: "text", text }],
	}
}

function taskItem(
	attrs: JSONContent["attrs"],
	...content: JSONContent[]
): JSONContent {
	return {
		type: "taskList",
		content: [{ type: "taskItem", attrs, content }],
	}
}

// stand-in for the real image block, whose node view brings a vue
// component; the decoration plugin only looks at the node type name
const ImageBlockStub = Node.create({
	name: IMAGE_BLOCK_NAME,
	group: "block",
	atom: true,

	renderHTML() {
		return ["div", { "data-type": IMAGE_BLOCK_NAME }]
	},
})

function makeEditor(content: JSONContent, opaqueTypes?: string[]): Editor {
	const editor = new Editor({
		element: document.createElement("div"),
		injectCSS: false,
		extensions: [
			Document,
			Paragraph,
			Text,
			Heading,
			HorizontalRule,
			TaskList,
			TaskItem,
			ImageBlockStub,
			DiffAttributes.configure({
				types: [
					Paragraph.name,
					Heading.name,
					HorizontalRule.name,
					TaskItem.name,
					IMAGE_BLOCK_NAME,
				],
			}),
			opaqueTypes
				? DiffDecorations.configure({ opaqueTypes })
				: DiffDecorations,
		],
	})

	// the decoration plugin starts empty and rebuilds on the first
	// document change, exactly like the diff editor's recompute flow
	editor.commands.setContent(content)

	return editor
}

function diffElements(editor: Editor): NodeListOf<Element> {
	return editor.view.dom.querySelectorAll('[class*="diff-"]')
}

describe("DiffDecorations", () => {
	it("adds no decorations to unchanged blocks", ({ expect }) => {
		const editor = makeEditor(
			doc(
				paragraph("plain"),
				paragraph("kept", { diffStatus: DiffStatus.Unchanged }),
			),
		)

		expect(diffElements(editor)).toHaveLength(0)

		editor.destroy()
	})

	it.for([
		{ status: DiffStatus.Added, cssClass: "diff-added" },
		{ status: DiffStatus.Removed, cssClass: "diff-removed" },
	])(
		"marks a $status heading with the $cssClass class",
		({ status, cssClass }, { expect }) => {
			const editor = makeEditor(doc(heading("title", { diffStatus: status })))

			const h1 = editor.view.dom.querySelector("h1")
			expect(h1?.classList.contains(cssClass)).toBe(true)
			expect(editor.view.dom.querySelector(".diff-overlay")).toBeNull()

			editor.destroy()
		},
	)

	it("covers an added paragraph with an overlay widget", ({ expect }) => {
		const editor = makeEditor(
			doc(paragraph("new", { diffStatus: DiffStatus.Added })),
		)

		const p = editor.view.dom.querySelector("p")
		expect(p?.classList.contains("diff-overlay-anchor")).toBe(true)

		const overlay = p?.querySelector<HTMLSpanElement>(
			"span.diff-overlay.diff-added",
		)
		expect(overlay).not.toBeNull()
		expect(overlay?.getAttribute("contenteditable")).toBe("false")
		expect(overlay?.style.top).toBe("-0.2em")
		expect(overlay?.style.left).toBe("-0.2em")

		editor.destroy()
	})

	it("treats an added task item as one unit without decorating its children", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc(
				taskItem(
					{ diffStatus: DiffStatus.Added },
					paragraph("todo", { diffStatus: DiffStatus.Added }),
				),
			),
		)

		expect(editor.view.dom.querySelectorAll(".diff-overlay")).toHaveLength(1)
		const li = editor.view.dom.querySelector("li")
		expect(li?.classList.contains("diff-overlay-anchor")).toBe(true)
		const p = editor.view.dom.querySelector("p")
		expect(p?.classList.contains("diff-overlay-anchor")).toBe(false)

		editor.destroy()
	})

	it("marks a modified opaque node with the diff-modified class", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc({
				type: "horizontalRule",
				attrs: { diffStatus: DiffStatus.Modified },
			}),
		)

		const hr = editor.view.dom.querySelector("hr")
		expect(hr?.classList.contains("diff-modified")).toBe(true)

		editor.destroy()
	})

	it("treats configured opaque types as whole-node modifications", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc(
				heading("new title", {
					diffStatus: DiffStatus.Modified,
					oldNode: heading("old title"),
				}),
			),
			[Heading.name],
		)

		const h1 = editor.view.dom.querySelector("h1")
		expect(h1?.classList.contains("diff-modified")).toBe(true)
		expect(editor.view.dom.querySelector(".diff-text-added")).toBeNull()
		expect(editor.view.dom.querySelector(".diff-text-removed")).toBeNull()

		editor.destroy()
	})

	it("skips self-decorated image blocks", ({ expect }) => {
		const editor = makeEditor(
			doc({
				type: IMAGE_BLOCK_NAME,
				attrs: { diffStatus: DiffStatus.Added },
			}),
		)

		expect(diffElements(editor)).toHaveLength(0)

		editor.destroy()
	})

	it("highlights the whole task item when its checked state changed", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc(
				taskItem(
					{
						checked: true,
						diffStatus: DiffStatus.Modified,
						oldNode: { type: "taskItem", attrs: { checked: false } },
					},
					paragraph("task"),
				),
			),
		)

		const li = editor.view.dom.querySelector("li")
		expect(li?.classList.contains("diff-overlay-anchor")).toBe(true)
		expect(li?.querySelector("span.diff-overlay.diff-modified")).not.toBeNull()

		editor.destroy()
	})

	it("recurses into a modified task item whose checked state is unchanged", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc(
				taskItem(
					{
						checked: true,
						diffStatus: DiffStatus.Modified,
						oldNode: { type: "taskItem", attrs: { checked: true } },
					},
					paragraph("new", {
						diffStatus: DiffStatus.Modified,
						oldNode: paragraph("old"),
					}),
				),
			),
		)

		expect(editor.view.dom.querySelector(".diff-overlay")).toBeNull()
		const p = editor.view.dom.querySelector("p")
		expect(p?.querySelector("span.diff-text-added")?.textContent).toBe("new")
		expect(p?.querySelector("span.diff-text-removed")?.textContent).toBe("old")

		editor.destroy()
	})

	it("decorates inserted text in a modified textblock", ({ expect }) => {
		const editor = makeEditor(
			doc(
				paragraph("hello brave world", {
					diffStatus: DiffStatus.Modified,
					oldNode: paragraph("hello world"),
				}),
			),
		)

		const p = editor.view.dom.querySelector("p")
		expect(p?.querySelector("span.diff-text-added")?.textContent).toBe("brave ")
		expect(p?.querySelector("span.diff-text-removed")).toBeNull()
		expect(p?.textContent).toBe("hello brave world")

		editor.destroy()
	})

	it("injects deleted text as a removed-text widget", ({ expect }) => {
		const editor = makeEditor(
			doc(
				paragraph("hello world", {
					diffStatus: DiffStatus.Modified,
					oldNode: paragraph("hello cruel world"),
				}),
			),
		)

		const p = editor.view.dom.querySelector("p")
		expect(p?.querySelector("span.diff-text-removed")?.textContent).toBe(
			"cruel ",
		)
		expect(p?.querySelector("span.diff-text-added")).toBeNull()

		// the widget sits between the surviving halves of the text
		expect(p?.textContent).toBe("hello cruel world")

		editor.destroy()
	})

	it("leaves a modified textblock without an old version undecorated", ({
		expect,
	}) => {
		const editor = makeEditor(
			doc(paragraph("text", { diffStatus: DiffStatus.Modified })),
		)

		expect(diffElements(editor)).toHaveLength(0)

		editor.destroy()
	})

	it("groups deleted text into spans by mark run", ({ expect }) => {
		const editor = makeEditor(
			doc(
				paragraph("", {
					diffStatus: DiffStatus.Modified,
					oldNode: {
						type: "paragraph",
						content: [
							{ type: "text", text: "ab", marks: [{ type: "bold" }] },
							{ type: "text", text: "cd" },
						],
					},
				}),
			),
		)

		const wrapper = editor.view.dom.querySelector("span.diff-text-removed")
		const runs = Array.from(wrapper?.children ?? []) as HTMLElement[]
		expect(runs.map((run) => [run.style.fontWeight, run.textContent])).toEqual([
			["bold", "ab"],
			["", "cd"],
		])

		editor.destroy()
	})

	it("styles deleted text according to its marks", ({ expect }) => {
		const editor = makeEditor(
			doc(
				paragraph("", {
					diffStatus: DiffStatus.Modified,
					oldNode: {
						type: "paragraph",
						content: [
							{
								type: "text",
								text: "x",
								marks: [{ type: "bold" }, { type: "italic" }, { type: "code" }],
							},
							{ type: "text", text: "y", marks: [{ type: "underline" }] },
							{ type: "text", text: "z", marks: [{ type: "strike" }] },
						],
					},
				}),
			),
		)

		const wrapper = editor.view.dom.querySelector("span.diff-text-removed")
		const runs = Array.from(wrapper?.children ?? []) as HTMLElement[]

		expect(runs).toHaveLength(3)
		expect(runs[0]?.style.fontWeight).toBe("bold")
		expect(runs[0]?.style.fontStyle).toBe("italic")
		expect(runs[0]?.style.fontFamily).toBe("monospace")
		expect(runs[0]?.style.fontSize).toBe("0.9em")
		expect(runs[1]?.style.textDecoration).toBe("underline")
		expect(runs[2]?.style.textDecoration).toBe("line-through")

		editor.destroy()
	})
})
