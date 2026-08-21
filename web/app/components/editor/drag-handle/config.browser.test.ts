import type { Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { afterEach, describe, it } from "vitest"
import { highlightOverlayByNodeType } from "./config"
import { FIGMA_BLOCK_NAME, IMAGE_BLOCK_NAME } from "../blocks/node-names"

type NodeName =
	| "doc"
	| "paragraph"
	| "listItem"
	| "horizontalRule"
	| typeof IMAGE_BLOCK_NAME
	| typeof FIGMA_BLOCK_NAME
	| "text"

// only the node types the overlay helper branches on
const schema = new Schema<NodeName>({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "text*" },
		listItem: { group: "block", content: "paragraph+" },
		horizontalRule: { group: "block", atom: true },
		[IMAGE_BLOCK_NAME]: { group: "block", atom: true },
		[FIGMA_BLOCK_NAME]: { group: "block", atom: true },
		text: { group: "inline" },
	},
})

// the helper reads nothing from the editor but state.doc, so a bare doc
// holder stands in
function editorWith(typeName: NodeName): Editor {
	const node: PMNode = schema.nodes[typeName].create()
	const doc = schema.nodes.doc.create(null, [node])

	return { state: { doc } } as unknown as Editor
}

const defaults = {
	extraLeft: 5,
	extraRight: 5,
	extraTop: 5,
	extraBottom: 5,
}

// the suite measures real boxes laid out in the shared page, so the
// tests cannot interleave
describe("highlightOverlayByNodeType", { concurrent: false }, () => {
	const cleanups: (() => void)[] = []

	afterEach(() => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}
	})

	function makeTarget(inner = ""): HTMLElement {
		const target = document.createElement("div")
		target.style.width = "300px"
		target.innerHTML = inner

		return target
	}

	// a 300px-wide parent wrapping a target of the same width, so every
	// fallback in the measurement chain has a distinct value to land on
	function mountTarget(inner = ""): HTMLElement {
		const parent = document.createElement("div")
		parent.style.width = "300px"

		const target = makeTarget(inner)
		parent.appendChild(target)
		document.body.appendChild(parent)

		cleanups.push(() => {
			parent.remove()
		})

		return target
	}

	it.for<{ name: string; typeName: NodeName }>([
		{ name: "a list item reaches past the marker", typeName: "listItem" },
		{ name: "a horizontal rule grows vertically", typeName: "horizontalRule" },
		{ name: "any other node keeps the default inset", typeName: "paragraph" },
	])("$name", ({ typeName }, { expect }) => {
		const expected: Record<string, typeof defaults> = {
			listItem: { ...defaults, extraLeft: 30 },
			horizontalRule: { ...defaults, extraTop: 8, extraBottom: 8 },
			paragraph: defaults,
		}

		expect(
			highlightOverlayByNodeType(editorWith(typeName), 0, mountTarget()),
		).toEqual(expected[typeName])
	})

	it("keeps the default inset when no node sits at the position", ({
		expect,
	}) => {
		const editor = editorWith("listItem")

		expect(
			highlightOverlayByNodeType(
				editor,
				editor.state.doc.content.size,
				mountTarget(),
			),
		).toEqual(defaults)
	})

	describe("image blocks", () => {
		it("extends the overlay to the width of a wider image", ({ expect }) => {
			const target = mountTarget(
				`<img alt="" style="width: 500px; max-width: none" />`,
			)

			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 205 })
		})

		it("finds an image nested below the block's first child", ({ expect }) => {
			const target = mountTarget(
				`<div><figure><img alt="" style="width: 500px; max-width: none" /></figure></div>`,
			)

			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 205 })
		})

		it("keeps the default inset when the block holds no image", ({
			expect,
		}) => {
			const target = mountTarget(`<span>no image here</span>`)

			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual(defaults)
		})

		it("falls back to layout widths when the boxes are scaled away", ({
			expect,
		}) => {
			const target = mountTarget(
				`<img alt="" style="width: 500px; max-width: none" />`,
			)
			target.style.transform = "scale(0)"

			expect(target.getBoundingClientRect().width).toBe(0)
			expect(target.offsetWidth).toBe(300)
			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 205 })
		})

		it("falls back to the parent width when the block is not rendered", ({
			expect,
		}) => {
			const target = mountTarget(
				`<img alt="" style="width: 500px; max-width: none" />`,
			)
			target.style.display = "none"

			expect(target.offsetWidth).toBe(0)
			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: -295 })
		})

		it("keeps the default inset when the block is detached from the page", ({
			expect,
		}) => {
			const target = makeTarget(
				`<img alt="" style="width: 500px; max-width: none" />`,
			)

			expect(target.parentElement).toBeNull()
			expect(
				highlightOverlayByNodeType(editorWith(IMAGE_BLOCK_NAME), 0, target),
			).toEqual(defaults)
		})
	})

	describe("figma blocks", () => {
		it("extends the overlay to the width of a wider embed", ({ expect }) => {
			const target = mountTarget(
				`<iframe title="figma" style="width: 500px; border: 0"></iframe>`,
			)

			expect(
				highlightOverlayByNodeType(editorWith(FIGMA_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 207 })
		})

		it("keeps the default inset when the block holds no embed", ({
			expect,
		}) => {
			const target = mountTarget(`<span>no embed here</span>`)

			expect(
				highlightOverlayByNodeType(editorWith(FIGMA_BLOCK_NAME), 0, target),
			).toEqual(defaults)
		})

		it("falls back to layout widths when the boxes are scaled away", ({
			expect,
		}) => {
			const target = mountTarget(
				`<iframe title="figma" style="width: 500px; border: 0"></iframe>`,
			)
			target.style.transform = "scale(0)"

			expect(target.getBoundingClientRect().width).toBe(0)
			expect(target.offsetWidth).toBe(300)
			expect(
				highlightOverlayByNodeType(editorWith(FIGMA_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 207 })
		})

		it("falls back to the parent width when the block is not rendered", ({
			expect,
		}) => {
			const target = mountTarget(
				`<iframe title="figma" style="width: 500px; border: 0"></iframe>`,
			)
			target.style.display = "none"

			expect(target.offsetWidth).toBe(0)
			expect(
				highlightOverlayByNodeType(editorWith(FIGMA_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: -293 })
		})

		it("keeps the default inset when the block is detached from the page", ({
			expect,
		}) => {
			const target = makeTarget(
				`<iframe title="figma" style="width: 500px; border: 0"></iframe>`,
			)

			expect(target.parentElement).toBeNull()
			expect(
				highlightOverlayByNodeType(editorWith(FIGMA_BLOCK_NAME), 0, target),
			).toEqual({ ...defaults, extraRight: 7 })
		})
	})
})
