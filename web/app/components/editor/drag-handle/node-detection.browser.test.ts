import type { JSONContent } from "@tiptap/core"
import { Editor, Node as TiptapNode } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import {
	BulletList,
	ListItem,
	OrderedList,
	TaskItem,
	TaskList,
} from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { afterEach, describe, it, vi } from "vitest"
import {
	findDraggableNodeAtCoords,
	isDraggableNodeType,
} from "./node-detection"
import { DRAG_HANDLE_APPROX_WIDTH_PX } from "./config"
import { bulletList, taskList } from "./test-helpers"
import { paragraph } from "~/components/editor/test-helpers"

// deterministic spacing so the Y-based detection paths (margins, gaps,
// nested list bounds) behave the same on every run
const style = document.createElement("style")
style.textContent = `
	body { margin: 0; }
	.ProseMirror p { margin: 8px 0; }
	.ProseMirror ul, .ProseMirror ol { margin: 8px 0; padding-left: 32px; }
	.ProseMirror li { margin: 4px 0; }
	.ProseMirror blockquote { margin: 8px 0 8px 16px; }
	.ProseMirror hr { margin: 12px 0; }
`
document.head.appendChild(style)

// stands in for the real image block node view: a draggable atom leaf
// whose outer DOM carries the data-node-view-wrapper attribute, exactly
// like tiptap's vue node view renderer emits, without needing a vue app
const ImageBlockStub = TiptapNode.create({
	name: "imageBlock",
	group: "block",
	atom: true,

	renderHTML() {
		return [
			"div",
			{ "data-node-view-wrapper": "", style: "height: 40px" },
			["span", { style: "display: block; height: 20px" }, "image"],
		]
	},
})

function listItem(...blocks: JSONContent[]): JSONContent {
	return { type: "listItem", content: blocks }
}

function taskItem(...blocks: JSONContent[]): JSONContent {
	return { type: "taskItem", attrs: { checked: false }, content: blocks }
}

describe("isDraggableNodeType", () => {
	it.for([
		{ typeName: "paragraph", expected: true },
		{ typeName: "heading", expected: true },
		{ typeName: "listItem", expected: true },
		{ typeName: "taskItem", expected: true },
		{ typeName: "horizontalRule", expected: true },
		{ typeName: "bulletList", expected: false },
		{ typeName: "doc", expected: false },
		{ typeName: "metricGrid", expected: false },
	])(
		"returns $expected for $typeName",
		({ typeName, expected }, { expect }) => {
			expect(isDraggableNodeType(typeName)).toBe(expected)
		},
	)
})

// the tests position real editors in the shared page and query
// document.elementFromPoint, so they cannot interleave
describe("findDraggableNodeAtCoords", { concurrent: false }, () => {
	const cleanups: (() => void)[] = []

	afterEach(() => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}
	})

	function mountEditor(
		content: JSONContent[],
		opts: { marginLeft?: number } = {},
	): Editor {
		const container = document.createElement("div")
		container.style.width = "400px"
		container.style.marginLeft = `${opts.marginLeft ?? 0}px`
		document.body.appendChild(container)

		const editor = new Editor({
			element: container,
			extensions: [
				Document,
				Paragraph,
				Text,
				Heading,
				Blockquote,
				HorizontalRule,
				BulletList,
				OrderedList,
				ListItem,
				TaskList,
				TaskItem.configure({ nested: true }),
				ImageBlockStub,
			],
			content: { type: "doc", content },
		})

		cleanups.push(() => {
			editor.destroy()
			container.remove()
		})

		return editor
	}

	function q(editor: Editor, selector: string, index = 0): HTMLElement {
		const el = editor.view.dom.querySelectorAll<HTMLElement>(selector)[index]

		if (!el) {
			throw new Error(`no element for ${selector}[${index}]`)
		}

		return el
	}

	function center(el: Element): { x: number; y: number } {
		const rect = el.getBoundingClientRect()

		return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
	}

	it("returns the paragraph under the cursor", ({ expect }) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p2 = q(editor, "p", 1)
		const { x, y } = center(p2)

		const result = findDraggableNodeAtCoords(editor, x, y)

		expect(result?.node.type.name).toBe("paragraph")
		expect(result?.node.textContent).toBe("two")
		expect(result?.dom).toBe(p2)
		expect(result?.pos).toBe(5)
		expect(result?.depth).toBe(1)
	})

	it("detects a paragraph when hovering the whitespace between blocks", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p1 = q(editor, "p", 0)
		const p2 = q(editor, "p", 1)
		const y =
			(p1.getBoundingClientRect().bottom + p2.getBoundingClientRect().top) / 2

		const result = findDraggableNodeAtCoords(editor, center(p1).x, y)

		expect(result?.node.textContent).toBe("one")
		expect(result?.dom).toBe(p1)
	})

	it("returns null when an ignored element overlaps the cursor row", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p2 = q(editor, "p", 1)
		p2.classList.add("drag-handle-ignore")
		const { x, y } = center(p2)

		expect(findDraggableNodeAtCoords(editor, x, y)).toBeNull()
	})

	it("keeps detecting when an ignored element sits on another row", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p1 = q(editor, "p", 0)
		p1.classList.add("drag-handle-ignore")
		const p2 = q(editor, "p", 1)
		const { x, y } = center(p2)

		const result = findDraggableNodeAtCoords(editor, x, y)

		expect(result?.node.textContent).toBe("two")
		expect(result?.dom).toBe(p2)
	})

	it("returns null when the hovered element opts out with the ignore-self class", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p2 = q(editor, "p", 1)
		p2.classList.add("drag-handle-ignore-self")
		const { x, y } = center(p2)

		expect(findDraggableNodeAtCoords(editor, x, y)).toBeNull()
	})

	it("falls back to row detection for elements outside the editor carrying the ignore class", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p2 = q(editor, "p", 1)
		const rect = p2.getBoundingClientRect()

		// an overlay outside the editor DOM, covering the second
		// paragraph, like the drag handle itself does
		const overlay = document.createElement("div")
		overlay.className = "drag-handle-ignore"
		overlay.style.position = "fixed"
		overlay.style.top = `${rect.top}px`
		overlay.style.left = `${rect.left}px`
		overlay.style.width = `${rect.width + 20}px`
		overlay.style.height = `${rect.height}px`
		document.body.appendChild(overlay)
		cleanups.push(() => {
			overlay.remove()
		})

		const result = findDraggableNodeAtCoords(editor, center(p2).x, center(p2).y)

		expect(result?.node.textContent).toBe("two")
		expect(result?.dom).toBe(p2)
	})

	it("falls back to row detection for an element that lives outside the editor", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])
		const p2 = q(editor, "p", 1)
		const rect = p2.getBoundingClientRect()

		// no ignore class this time: the element simply has no position in
		// the document, so posAtDOM reports none for it or its ancestors
		const overlay = document.createElement("div")
		overlay.style.position = "fixed"
		overlay.style.top = `${rect.top}px`
		overlay.style.left = `${rect.left}px`
		overlay.style.width = `${rect.width + 20}px`
		overlay.style.height = `${rect.height}px`
		document.body.appendChild(overlay)
		cleanups.push(() => {
			overlay.remove()
		})

		const result = findDraggableNodeAtCoords(editor, center(p2).x, center(p2).y)

		expect(result?.node.textContent).toBe("two")
		expect(result?.dom).toBe(p2)
	})

	it("detects a horizontal rule near the cursor row", ({ expect }) => {
		const editor = mountEditor([
			paragraph("one"),
			{ type: "horizontalRule" },
			paragraph("three"),
		])
		const hr = q(editor, "hr")
		const rect = hr.getBoundingClientRect()

		const result = findDraggableNodeAtCoords(
			editor,
			center(q(editor, "p", 0)).x,
			(rect.top + rect.bottom) / 2 + 5,
		)

		expect(result?.node.type.name).toBe("horizontalRule")
		expect(result?.dom).toBe(hr)
	})

	it("does not treat rows far from a horizontal rule as the rule", ({
		expect,
	}) => {
		const editor = mountEditor([
			paragraph("one"),
			{ type: "horizontalRule" },
			paragraph("three"),
		])
		const p3 = q(editor, "p", 1)

		const result = findDraggableNodeAtCoords(editor, center(p3).x, center(p3).y)

		expect(result?.node.type.name).toBe("paragraph")
		expect(result?.node.textContent).toBe("three")
	})

	it("picks the horizontal rule nearest the cursor row out of several", ({
		expect,
	}) => {
		const editor = mountEditor([
			paragraph("one"),
			{ type: "horizontalRule" },
			paragraph("two"),
			{ type: "horizontalRule" },
			paragraph("three"),
		])
		const secondHr = q(editor, "hr", 1)
		const rect = secondHr.getBoundingClientRect()

		const result = findDraggableNodeAtCoords(
			editor,
			center(q(editor, "p", 0)).x,
			(rect.top + rect.bottom) / 2,
		)

		expect(result?.node.type.name).toBe("horizontalRule")
		expect(result?.dom).toBe(secondHr)
	})

	it("returns null for coordinates outside the viewport", ({ expect }) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")])

		expect(findDraggableNodeAtCoords(editor, 20, -500)).toBeNull()
	})

	it("finds nodes by row when the cursor is in the left margin", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), paragraph("two")], {
			marginLeft: 60,
		})
		const p2 = q(editor, "p", 1)
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			center(p2).y,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.textContent).toBe("two")
		expect(result?.dom).toBe(p2)
	})

	it("falls back to row detection when the hit test lands on the list container", ({
		expect,
	}) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("item one")),
				listItem(paragraph("item two")),
			),
		])
		const ul = q(editor, "ul")
		const li2 = q(editor, "li", 1)

		// the item marker is painted at the inner edge of the list padding
		// and answers the hit test for its <li>, so the probe needs room to
		// its left to reach the <ul> itself
		ul.style.paddingLeft = "80px"

		const x = ul.getBoundingClientRect().left + 8
		const y = center(li2).y

		expect(document.elementFromPoint(x + DRAG_HANDLE_APPROX_WIDTH_PX, y)).toBe(
			ul,
		)

		const result = findDraggableNodeAtCoords(editor, x, y)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(li2)
	})

	it("falls back to row detection when hovering the gap between list items", ({
		expect,
	}) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("item one")),
				listItem(paragraph("item two")),
			),
		])
		const li1 = q(editor, "li")
		const li2 = q(editor, "li", 1)

		// the gap between the two item boxes belongs to the <ul> itself, so
		// the hit test lands on the list container rather than an item
		const result = findDraggableNodeAtCoords(
			editor,
			center(li1).x,
			(li1.getBoundingClientRect().bottom + li2.getBoundingClientRect().top) /
				2,
		)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(li1)
	})

	it("prefers the deepest nested list item under the cursor", ({ expect }) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("parent"), bulletList(listItem(paragraph("child")))),
			),
		])
		const childP = q(editor, "li li p")
		const childLi = q(editor, "li li")

		const result = findDraggableNodeAtCoords(
			editor,
			center(childP).x,
			center(childP).y,
		)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(childLi)
	})

	it("returns the parent list item on its own text row", ({ expect }) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("parent"), bulletList(listItem(paragraph("child")))),
			),
		])
		const parentP = q(editor, "li > p")
		const parentLi = q(editor, "li")

		const result = findDraggableNodeAtCoords(
			editor,
			center(parentP).x,
			center(parentP).y,
		)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(parentLi)
	})

	it("ignores the trailing margin below a nested list", ({ expect }) => {
		const editor = mountEditor(
			[
				bulletList(
					listItem(
						paragraph("parent"),
						bulletList(listItem(paragraph("child"))),
					),
				),
			],
			{ marginLeft: 60 },
		)
		const parentLi = q(editor, "li")
		parentLi.style.paddingBottom = "40px"
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			parentLi.getBoundingClientRect().bottom - 15,
			{ leftMarginWidth: 50 },
		)

		expect(result).toBeNull()
	})

	it("returns the parent list item when hovering below its nested list", ({
		expect,
	}) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("parent"), bulletList(listItem(paragraph("child")))),
			),
		])
		const parentLi = q(editor, "li")
		parentLi.style.paddingBottom = "40px"
		const childLi = q(editor, "li li")

		const result = findDraggableNodeAtCoords(
			editor,
			center(q(editor, "li > p")).x,
			parentLi.getBoundingClientRect().bottom - 15,
		)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(parentLi)
		expect(result?.dom).not.toBe(childLi)
	})

	it("returns the parent list item when the row is in its padding above the content", ({
		expect,
	}) => {
		const editor = mountEditor(
			[
				bulletList(
					listItem(
						paragraph("parent"),
						bulletList(listItem(paragraph("child"))),
					),
				),
			],
			{ marginLeft: 60 },
		)
		const parentLi = q(editor, "li")
		parentLi.style.paddingTop = "20px"
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		// a row inside the padding: no child of the list item covers it, and
		// the nested list is far below it
		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			parentLi.getBoundingClientRect().top + 6,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(parentLi)
	})

	it("returns null when the list item containing the row opts out with ignore-self", ({
		expect,
	}) => {
		const editor = mountEditor([
			bulletList(
				listItem(paragraph("item one")),
				listItem(paragraph("item two")),
			),
		])
		const li1 = q(editor, "li")
		li1.classList.add("drag-handle-ignore-self")
		const p1 = q(editor, "li p")

		expect(
			findDraggableNodeAtCoords(editor, center(p1).x, center(p1).y),
		).toBeNull()
	})

	it("returns the blockquote for content nested inside it", ({ expect }) => {
		const editor = mountEditor([
			{ type: "blockquote", content: [paragraph("quoted")] },
		])
		const p = q(editor, "blockquote p")
		const bq = q(editor, "blockquote")

		const result = findDraggableNodeAtCoords(editor, center(p).x, center(p).y)

		expect(result?.node.type.name).toBe("blockquote")
		expect(result?.dom).toBe(bq)
	})

	it("returns the blockquote when scanning by row in the left margin", ({
		expect,
	}) => {
		const editor = mountEditor(
			[{ type: "blockquote", content: [paragraph("quoted")] }],
			{ marginLeft: 60 },
		)
		const p = q(editor, "blockquote p")
		const bq = q(editor, "blockquote")
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			center(p).y,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.type.name).toBe("blockquote")
		expect(result?.dom).toBe(bq)
	})

	// DRAG_DISABLED_EXCEPT_RULES lets a node type declare which descendant
	// types may still carry a handle of their own; blockquote's entry is
	// empty, so none of them do
	it("keeps the drag handle on a node that disables all of its descendants", ({
		expect,
	}) => {
		const editor = mountEditor([
			{ type: "blockquote", content: [paragraph("quoted")] },
		])
		const p = q(editor, "blockquote p")
		const bq = q(editor, "blockquote")

		expect(isDraggableNodeType("paragraph")).toBe(true)

		const result = findDraggableNodeAtCoords(editor, center(p).x, center(p).y)

		expect(result?.node.type.name).toBe("blockquote")
		expect(result?.dom).toBe(bq)
	})

	it("keeps the drag handle on a node that disables all of its descendants when scanning by row", ({
		expect,
	}) => {
		const editor = mountEditor(
			[{ type: "blockquote", content: [paragraph("quoted")] }],
			{ marginLeft: 60 },
		)
		const p = q(editor, "blockquote p")
		const bq = q(editor, "blockquote")
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		expect(isDraggableNodeType("paragraph")).toBe(true)

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			center(p).y,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.type.name).toBe("blockquote")
		expect(result?.dom).toBe(bq)
	})

	it("keeps the drag handle on the list item when its paragraph is not an allowed descendant", ({
		expect,
	}) => {
		const editor = mountEditor([bulletList(listItem(paragraph("item")))])
		const p = q(editor, "li p")
		const li = q(editor, "li")

		expect(isDraggableNodeType("paragraph")).toBe(true)

		const result = findDraggableNodeAtCoords(editor, center(p).x, center(p).y)

		expect(result?.node.type.name).toBe("listItem")
		expect(result?.dom).toBe(li)
	})

	it("keeps the drag handle on a node no ancestor type restricts", ({
		expect,
	}) => {
		const editor = mountEditor([
			{
				type: "heading",
				attrs: { level: 2 },
				content: [{ type: "text", text: "title" }],
			},
			paragraph("body"),
		])
		const h2 = q(editor, "h2")

		const result = findDraggableNodeAtCoords(editor, center(h2).x, center(h2).y)

		expect(result?.node.type.name).toBe("heading")
		expect(result?.dom).toBe(h2)
	})

	it("returns the task item for content inside it", ({ expect }) => {
		const editor = mountEditor([taskList(taskItem(paragraph("task")))])
		const p = q(editor, "li p")
		const li = q(editor, "li")

		const result = findDraggableNodeAtCoords(editor, center(p).x, center(p).y)

		expect(result?.node.type.name).toBe("taskItem")
		expect(result?.dom).toBe(li)
	})

	it("returns the nested task item rather than its parent", ({ expect }) => {
		const editor = mountEditor([
			taskList(
				taskItem(paragraph("outer"), taskList(taskItem(paragraph("inner")))),
			),
		])
		const innerP = q(editor, "li li p")
		const innerLi = q(editor, "li li")

		const result = findDraggableNodeAtCoords(
			editor,
			center(innerP).x,
			center(innerP).y,
		)

		expect(result?.node.type.name).toBe("taskItem")
		expect(result?.dom).toBe(innerLi)
	})

	// task items wrap their content in a <div>, so the outer item's bottom
	// margin is a row that none of its children cover while the nested items
	// still reach into it
	function nestedTaskEditor(marginLeft: number): Editor {
		return mountEditor(
			[
				taskList(
					taskItem(
						paragraph("outer"),
						taskList(
							taskItem(
								paragraph("middle"),
								taskList(taskItem(paragraph("inner"))),
							),
						),
					),
				),
			],
			{ marginLeft },
		)
	}

	it("returns the deepest nested task item for a row in the outer item's bottom margin", ({
		expect,
	}) => {
		const editor = nestedTaskEditor(60)
		const outerLi = q(editor, "li")
		const innerLi = q(editor, "li li li")
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			outerLi.getBoundingClientRect().bottom + 2,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.type.name).toBe("taskItem")
		expect(result?.dom).toBe(innerLi)
	})

	it("skips a nested task item whose own trailing margin covers the row", ({
		expect,
	}) => {
		const editor = nestedTaskEditor(60)
		const outerLi = q(editor, "li")
		const middleLi = q(editor, "li li")
		middleLi.style.paddingBottom = "20px"
		const editorLeft = editor.view.dom.getBoundingClientRect().left

		const result = findDraggableNodeAtCoords(
			editor,
			editorLeft - 30,
			outerLi.getBoundingClientRect().bottom + 2,
			{ leftMarginWidth: 50 },
		)

		expect(result?.node.type.name).toBe("taskItem")
		expect(result?.dom).toBe(outerLi)
	})

	it("resolves an atom node through its node view wrapper", ({ expect }) => {
		const editor = mountEditor([paragraph("one"), { type: "imageBlock" }])
		const wrapper = q(editor, "div[data-node-view-wrapper]")
		const span = q(editor, "div[data-node-view-wrapper] span")

		const result = findDraggableNodeAtCoords(
			editor,
			center(span).x,
			center(span).y,
		)

		expect(result?.node.type.name).toBe("imageBlock")
		expect(result?.dom).toBe(wrapper)
		expect(result?.pos).toBe(5)
	})

	it("returns null for an atom wrapper that opts out with ignore-self", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), { type: "imageBlock" }])
		const wrapper = q(editor, "div[data-node-view-wrapper]")
		wrapper.classList.add("drag-handle-ignore-self")
		const span = q(editor, "div[data-node-view-wrapper] span")

		expect(
			findDraggableNodeAtCoords(editor, center(span).x, center(span).y),
		).toBeNull()
	})

	it("returns the blockquote for an atom node nested inside it", ({
		expect,
	}) => {
		const editor = mountEditor([
			{ type: "blockquote", content: [{ type: "imageBlock" }] },
		])
		const bq = q(editor, "blockquote")
		const span = q(editor, "div[data-node-view-wrapper] span")

		const result = findDraggableNodeAtCoords(
			editor,
			center(span).x,
			center(span).y,
		)

		expect(result?.node.type.name).toBe("blockquote")
		expect(result?.dom).toBe(bq)
	})

	it("returns null when the view reports no position for the hovered element", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), { type: "imageBlock" }])
		const span = q(editor, "div[data-node-view-wrapper] span")
		const { x, y } = center(span)
		vi.spyOn(editor.view, "posAtDOM").mockReturnValue(-1)

		expect(findDraggableNodeAtCoords(editor, x, y)).toBeNull()
	})

	it("returns null when the view rejects the hovered element", ({ expect }) => {
		const editor = mountEditor([paragraph("one"), { type: "imageBlock" }])
		const span = q(editor, "div[data-node-view-wrapper] span")
		const { x, y } = center(span)
		vi.spyOn(editor.view, "posAtDOM").mockImplementation(() => {
			throw new RangeError("DOM position not inside the editor")
		})

		expect(findDraggableNodeAtCoords(editor, x, y)).toBeNull()
	})

	it("returns null when the reported position lies outside the document", ({
		expect,
	}) => {
		const editor = mountEditor([paragraph("one"), { type: "imageBlock" }])
		const span = q(editor, "div[data-node-view-wrapper] span")
		const { x, y } = center(span)
		vi.spyOn(editor.view, "posAtDOM").mockReturnValue(
			editor.state.doc.content.size + 100,
		)

		expect(findDraggableNodeAtCoords(editor, x, y)).toBeNull()
	})
})
