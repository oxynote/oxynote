import { Editor, type JSONContent } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import { BulletList, ListItem } from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { afterEach, beforeEach, describe, it, onTestFinished, vi } from "vitest"
import { Drag } from "./drag"
import { GapDecorations } from "./gap-decorations"
import { DragHandlePlugin } from "./handle-plugin"
import { startTouchDrag } from "./touch-drag"
import {
	METRIC_BLOCK_NAME,
	METRIC_GRID_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../blocks/node-names"
import {
	bulletList,
	SplitDocumentationLeftSideStub,
	SplitDocumentationRightSideStub,
	SplitDocumentationStub,
} from "./test-helpers"
import {
	MetricBlockStub,
	MetricGridStub,
	paragraph,
	touchPointer,
} from "~/components/editor/test-helpers"

// where the finger first held the block in the unit cases
const HELD_AT = { x: 60, y: 120 }
// how far a nested list sits inside its parent
const INDENT = 32
// a list item whose text starts at the block's left edge, in an editor
// that starts left of it
const LIST_ITEM = { textLeft: 20, editorLeft: 10 }
// metric blocks too wide to share a row, the way a phone stacks them
const MetricGrid = MetricGridStub.extend({
	renderHTML() {
		return ["div", { "data-type": "metric-grid" }, 0]
	},
})
const MetricBlock = MetricBlockStub.extend({
	addAttributes() {
		return { title: { default: "" } }
	},

	renderHTML({ HTMLAttributes }) {
		return ["div", { ...HTMLAttributes, "data-type": "metric-block" }]
	},
})
// fake timers, the page's scroll and the elements placed on it are shared
describe("startTouchDrag", { concurrent: false }, () => {
	beforeEach(() => {
		vi.useFakeTimers({
			toFake: [
				"setTimeout",
				"clearTimeout",
				"requestAnimationFrame",
				"cancelAnimationFrame",
				"Date",
			],
		})
	})

	afterEach(() => {
		// run the pending frame before the real timers drop it. New gap
		// zones wait for one shared frame, and a dropped one leaves the
		// zones of every later test unplaced.
		vi.advanceTimersToNextFrame()
		vi.useRealTimers()
	})

	it("starts a list item's drag at its left edge, level with the handle", ({
		expect,
	}) => {
		const { handle, block, preview } = mountBlock()
		const onDragStart = vi.fn()
		handle.addEventListener("dragstart", onDragStart)

		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		expect.soft(drag).not.toBeNull()
		expect.soft(onDragStart).toHaveBeenCalledTimes(1)
		expect
			.soft(onDragStart.mock.calls[0]?.[0])
			.toMatchObject({ clientX: 24, clientY: 112, bubbles: true })
		expect
			.soft((onDragStart.mock.calls[0]?.[0] as DragEvent).dataTransfer)
			.toBeInstanceOf(DataTransfer)
	})

	it("returns null without copying the block when a listener refuses the drag", ({
		expect,
	}) => {
		const { handle, block, preview } = mountBlock()
		handle.addEventListener("dragstart", (e) => {
			e.preventDefault()
		})
		const copyBlock = vi.fn(() => preview)

		const drag = startTouchDrag(handle, block, copyBlock, HELD_AT, LIST_ITEM)

		expect.soft(drag).toBeNull()
		expect.soft(copyBlock).not.toHaveBeenCalled()
		expect.soft(preview.isConnected).toBe(false)
	})

	it("starts a list item's drag at its text, clear of a checkbox", ({
		expect,
	}) => {
		const { handle, block, preview } = mountBlock()
		const onDragStart = vi.fn()
		handle.addEventListener("dragstart", onDragStart)

		startTouchDrag(handle, block, () => preview, HELD_AT, {
			textLeft: 50,
			editorLeft: 10,
		})

		expect(onDragStart).toHaveBeenCalledExactlyOnceWith(
			expect.objectContaining({ clientX: 54, clientY: 112 }),
		)
	})

	it("moves a list item's preview as far as the finger moves", ({ expect }) => {
		const { handle, block, preview } = mountBlock()
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		drag?.move(touchPointer("pointermove", 80, 300))

		expect.soft(preview.isConnected).toBe(true)
		expect.soft(preview.style.position).toBe("fixed")
		expect.soft(preview.style.left).toBe("40px")
		expect.soft(preview.style.width).toBe("200px")
		expect.soft(preview.style.top).toBe("280px")
	})

	it("points each move at a list item's left edge and first line, moved as far as the finger", ({
		expect,
	}) => {
		const { handle, block, preview, target } = mountBlock()
		const onDragOver = vi.fn()
		target.addEventListener("dragover", onDragOver)
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		drag?.move(touchPointer("pointermove", 96, 418))

		expect(onDragOver).toHaveBeenCalledExactlyOnceWith(
			expect.objectContaining({ clientX: 60, clientY: 410 }),
		)
	})

	it("keeps a list item's drag point inside the editor's left edge", ({
		expect,
	}) => {
		const { handle, block, preview } = mountBlock()
		const onDragOver = vi.fn()
		document.addEventListener("dragover", onDragOver)
		onTestFinished(() => {
			document.removeEventListener("dragover", onDragOver)
		})
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		drag?.move(touchPointer("pointermove", 0, 120))

		expect(onDragOver).toHaveBeenCalledExactlyOnceWith(
			expect.objectContaining({ clientX: 14, clientY: 112 }),
		)
	})

	it("keeps a list item's preview from widening the page", ({ expect }) => {
		const { handle, block, preview } = mountBlock()
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		drag?.move(touchPointer("pointermove", 5000, 120))

		expect(preview.style.left).toBe(
			`${document.documentElement.clientWidth - 200}px`,
		)
	})

	it("drops any other block where the finger is", ({ expect }) => {
		const { handle, block, preview, target } = mountBlock()
		const onDragStart = vi.fn()
		const onDragOver = vi.fn()
		handle.addEventListener("dragstart", onDragStart)
		target.addEventListener("dragover", onDragOver)
		const drag = startTouchDrag(handle, block, () => preview, HELD_AT, null)

		drag?.move(touchPointer("pointermove", 96, 418))

		expect
			.soft(onDragStart)
			.toHaveBeenCalledExactlyOnceWith(
				expect.objectContaining({ clientX: 60, clientY: 120 }),
			)
		expect
			.soft(onDragOver)
			.toHaveBeenCalledExactlyOnceWith(
				expect.objectContaining({ clientX: 96, clientY: 418 }),
			)
	})

	it("reaches the block under the preview even where the copy keeps its own pointer events", ({
		expect,
	}) => {
		const { handle, block, preview, target } = mountBlock()
		const copied = document.createElement("div")
		Object.assign(copied.style, { height: "40px", pointerEvents: "auto" })
		preview.append(copied)
		const onDragOver = vi.fn()
		target.addEventListener("dragover", onDragOver)
		const drag = startTouchDrag(handle, block, () => preview, HELD_AT, null)

		drag?.move(touchPointer("pointermove", 96, 418))

		expect(onDragOver).toHaveBeenCalledOnce()
	})

	it("keeps any other block's preview in its column", ({ expect }) => {
		const { handle, block, preview } = mountBlock()
		const drag = startTouchDrag(handle, block, () => preview, HELD_AT, null)

		drag?.move(touchPointer("pointermove", 200, 300))

		expect.soft(preview.style.left).toBe("20px")
		expect.soft(preview.style.top).toBe("280px")
	})

	it("drops where the moved handle is and then ends the drag", ({ expect }) => {
		const { handle, block, preview, target } = mountBlock()
		const events: string[] = []
		target.addEventListener("drop", (e) => events.push(`${e.type}@target`))
		handle.addEventListener("dragend", (e) => events.push(`${e.type}@handle`))
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)

		drag?.drop(touchPointer("pointermove", 96, 418))

		expect.soft(events).toEqual(["drop@target", "dragend@handle"])
		expect.soft(preview.isConnected).toBe(false)
	})

	it("leaves its target and ends the drag without a drop when cancelled", ({
		expect,
	}) => {
		const { handle, block, preview, target } = mountBlock()
		const events: string[] = []

		for (const type of ["drop", "dragleave"]) {
			target.addEventListener(type, (e) => events.push(`${e.type}@target`))
		}

		handle.addEventListener("dragend", (e) => events.push(`${e.type}@handle`))
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)
		drag?.move(touchPointer("pointermove", 96, 418))

		drag?.cancel()

		expect.soft(events).toEqual(["dragleave@target", "dragend@handle"])
		expect.soft(preview.isConnected).toBe(false)
	})

	it.for([
		{ name: "top", input: () => 2, expected: -12 },
		{ name: "bottom", input: () => window.innerHeight - 2, expected: 12 },
	])(
		"scrolls the page while the finger is at the $name edge",
		({ input, expected }, { expect }) => {
			const scrollBy = vi.spyOn(window, "scrollBy").mockReturnValue()
			const { handle, block, preview } = mountBlock()
			const drag = startTouchDrag(
				handle,
				block,
				() => preview,
				HELD_AT,
				LIST_ITEM,
			)
			drag?.move(touchPointer("pointermove", 60, input()))

			vi.advanceTimersToNextFrame()
			vi.advanceTimersToNextFrame()

			expect(scrollBy.mock.calls).toEqual([
				[0, expected],
				[0, expected],
			])
		},
	)

	it("leaves the page still while the finger is away from the edges", ({
		expect,
	}) => {
		const scrollBy = vi.spyOn(window, "scrollBy").mockReturnValue()
		const { handle, block, preview } = mountBlock()
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)
		drag?.move(touchPointer("pointermove", 60, 300))

		vi.advanceTimersToNextFrame()

		expect(scrollBy).not.toHaveBeenCalled()
	})

	it("stops scrolling the page once the drag ends", ({ expect }) => {
		const scrollBy = vi.spyOn(window, "scrollBy").mockReturnValue()
		const { handle, block, preview } = mountBlock()
		const drag = startTouchDrag(
			handle,
			block,
			() => preview,
			HELD_AT,
			LIST_ITEM,
		)
		drag?.move(touchPointer("pointermove", 60, 2))

		drag?.cancel()
		vi.advanceTimersToNextFrame()

		expect(scrollBy).not.toHaveBeenCalled()
	})

	describe("with the drag handle plugin", { concurrent: false }, () => {
		it("moves a held block to where the finger lifts", async ({ expect }) => {
			const editor = mountEditor()
			const [first, , third] = [...editor.view.dom.querySelectorAll("p")]
			if (!first || !third) {
				throw new Error("the editor renders three paragraphs")
			}

			const from = centerOf(first)
			const to = { x: from.x, y: third.getBoundingClientRect().bottom - 2 }
			first.dispatchEvent(touchPointer("pointerdown", from.x, from.y))
			await vi.advanceTimersByTimeAsync(300)
			document.dispatchEvent(touchPointer("pointermove", to.x, to.y))

			document.dispatchEvent(touchPointer("pointerup", to.x, to.y))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("three"), paragraph("one"))`,
			)
		})

		it("moves another block after the first touch drag", async ({ expect }) => {
			const editor = mountEditor()
			await touchDragParagraph(editor, 0, 2)

			await touchDragParagraph(editor, 0, 2)

			expect
				.soft(editor.state.doc.toString())
				.toBe(`doc(paragraph("three"), paragraph("one"), paragraph("two"))`)
			expect.soft(editor.view.dom.style.pointerEvents).toBe("")
		})

		it.for([
			{
				name: "keeps a nested list item in its list when the finger moves straight down",
				input: 0,
				expected: `doc(bulletList(listItem(paragraph("one"), bulletList(listItem(paragraph("b")), listItem(paragraph("a")))), listItem(paragraph("two"))))`,
			},
			{
				name: "moves a nested list item out a level when the finger also slides left by one indent",
				input: -INDENT,
				expected: `doc(bulletList(listItem(paragraph("one"), bulletList(listItem(paragraph("b")))), listItem(paragraph("a")), listItem(paragraph("two"))))`,
			},
		])("$name", async ({ input, expected }, { expect }) => {
			const editor = mountEditor([nestedList()])
			const [, held, b] = [...editor.view.dom.querySelectorAll("p")]
			if (!held || !b) {
				throw new Error("the editor renders the list paragraphs")
			}

			const start = centerOf(held)

			// the gap below "b" takes a drop into the nested list and one
			// into the outer list, told apart by how far left the drag is
			await touchDragTo(held, start, {
				x: start.x + input,
				y: b.getBoundingClientRect().bottom + 6,
			})

			expect(editor.state.doc.toString()).toBe(expected)
		})

		it("moves a nested list item up out of its list through the gap above its parent", async ({
			expect,
		}) => {
			const editor = mountEditor([nestedList()])
			const [one, , held] = [...editor.view.dom.querySelectorAll("p")]
			if (!one || !held) {
				throw new Error("the editor renders the list paragraphs")
			}

			const start = centerOf(held)

			// only a gap takes a drop outside the list the item came from, and
			// the preview lies over the drag point all the way there
			await touchDragTo(held, start, {
				x: start.x,
				y: one.getBoundingClientRect().top - 4,
			})

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b")), listItem(paragraph("one"), bulletList(listItem(paragraph("a")))), listItem(paragraph("two"))))`,
			)
		})

		it.for([
			{
				name: "moves a stacked metric block below the next one",
				input: { held: 0, landing: "bottom" },
				expected: ["b", "a"],
			},
			{
				name: "moves a stacked metric block above the one before it",
				input: { held: 1, landing: "top" },
				expected: ["b", "a"],
			},
		])("$name", async ({ input, expected }, { expect }) => {
			const editor = mountEditor([
				{
					type: METRIC_GRID_NAME,
					content: ["a", "b"].map((title) => ({
						type: METRIC_BLOCK_NAME,
						attrs: { title },
					})),
				},
			])
			const blocks = [
				...editor.view.dom.querySelectorAll('[data-type="metric-block"]'),
			]
			const held = blocks[input.held]
			const other = blocks[1 - input.held]
			if (!held || !other) {
				throw new Error("the editor renders both metric blocks")
			}

			const start = centerOf(held)
			const target = other.getBoundingClientRect()

			await touchDragTo(held, start, {
				x: start.x,
				y: input.landing === "bottom" ? target.bottom - 10 : target.top + 10,
			})

			expect(
				editor.state.doc.firstChild?.children.map(
					(block) => block.attrs.title as string,
				),
			).toEqual(expected)
		})

		it.for([
			{
				name: "moves a metric block in a split documentation's right side onto the lower half of the next one",
				input: { held: 0, landing: 0.75 },
				expected: ["b", "a"],
			},
			{
				name: "moves a metric block in a split documentation's right side onto the upper half of the one before",
				input: { held: 1, landing: 0.25 },
				expected: ["b", "a"],
			},
		])("$name", async ({ input, expected }, { expect }) => {
			const editor = mountEditor([
				{
					type: SPLIT_DOCUMENTATION_NAME,
					content: [
						{
							type: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
							content: [paragraph("left")],
						},
						{
							type: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
							content: ["a", "b"].map((title) => ({
								type: METRIC_BLOCK_NAME,
								attrs: { title },
							})),
						},
					],
				},
			])
			const blocks = [
				...editor.view.dom.querySelectorAll('[data-type="metric-block"]'),
			]
			const held = blocks[input.held]
			const other = blocks[1 - input.held]
			if (!held || !other) {
				throw new Error("the editor renders both metric blocks")
			}

			const start = centerOf(held)
			const target = other.getBoundingClientRect()

			await touchDragTo(held, start, {
				x: start.x,
				y: target.top + target.height * input.landing,
			})

			expect(
				editor.state.doc.firstChild?.lastChild?.children.map(
					(block) => block.attrs.title as string,
				),
			).toEqual(expected)
		})
	})
})

// a block at a known place, its handle in the gutter beside it, its
// preview, and an element further down the page to drag over
function mountBlock() {
	const block = place(20, 100, 200, 40)
	const target = place(20, 400, 200, 40)
	const handle = place(0, 100, 16, 24)
	const preview = document.createElement("div")
	onTestFinished(() => {
		preview.remove()
	})

	return { handle, block, preview, target }
}

function place(left: number, top: number, width: number, height: number) {
	const element = document.createElement("div")
	Object.assign(element.style, {
		position: "fixed",
		left: `${left}px`,
		top: `${top}px`,
		width: `${width}px`,
		height: `${height}px`,
	})
	document.body.append(element)
	onTestFinished(() => {
		element.remove()
	})

	return element
}

function mountEditor(
	content = [paragraph("one"), paragraph("two"), paragraph("three")],
) {
	const style = document.createElement("style")
	style.textContent = `
		.ProseMirror ul { margin: 0; padding-left: ${INDENT}px; }
		.ProseMirror li > p { margin: 0; }
		[data-type="metric-grid"] { display: flex; flex-wrap: wrap; gap: 12px; }
		[data-type="metric-block"] { width: 300px; height: 120px; }
	`
	document.head.append(style)
	const container = document.createElement("div")
	container.style.width = "400px"
	container.style.marginTop = "80px"
	container.style.position = "relative"
	document.body.append(container)
	const editor = new Editor({
		element: container,
		extensions: [
			Document,
			Paragraph,
			Text,
			BulletList,
			ListItem,
			MetricGrid,
			MetricBlock,
			SplitDocumentationStub,
			SplitDocumentationLeftSideStub,
			SplitDocumentationRightSideStub,
			GapDecorations,
			Drag,
		],
		content: { type: "doc", content },
	})
	const handle = document.createElement("div")
	handle.textContent = "handle"
	editor.registerPlugin(
		DragHandlePlugin({ editor, element: handle, onNodeLongPress: vi.fn() })
			.plugin,
	)
	onTestFinished(() => {
		editor.destroy()
		container.remove()
		style.remove()
	})

	return editor
}

function listItem(...content: JSONContent[]): JSONContent {
	return { type: "listItem", content }
}

// "one" with "a" and "b" nested under it, then "two"
function nestedList(): JSONContent {
	return bulletList(
		listItem(
			paragraph("one"),
			bulletList(listItem(paragraph("a")), listItem(paragraph("b"))),
		),
		listItem(paragraph("two")),
	)
}

// holds the element where the finger lands, then drags to the end point
// and lifts there
async function touchDragTo(
	held: Element,
	start: { x: number; y: number },
	end: { x: number; y: number },
) {
	held.dispatchEvent(touchPointer("pointerdown", start.x, start.y))
	await vi.advanceTimersByTimeAsync(300)
	document.dispatchEvent(touchPointer("pointermove", end.x, end.y))
	document.dispatchEvent(touchPointer("pointerup", end.x, end.y))
}

function centerOf(element: Element) {
	const rect = element.getBoundingClientRect()

	return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
}

// holds the paragraph at one index and drags it to the bottom of the
// paragraph at another, through the touch events a finger sends
async function touchDragParagraph(editor: Editor, from: number, to: number) {
	const paragraphs = [...editor.view.dom.querySelectorAll("p")]
	const held = paragraphs[from]
	const target = paragraphs[to]
	if (!held || !target) {
		throw new Error("the editor renders the paragraphs to drag")
	}

	const start = centerOf(held)
	const end = { x: start.x, y: target.getBoundingClientRect().bottom - 2 }
	held.dispatchEvent(touchPointer("pointerdown", start.x, start.y))
	await vi.advanceTimersByTimeAsync(300)
	document.dispatchEvent(touchPointer("pointermove", end.x, end.y))
	document.dispatchEvent(touchPointer("pointerup", end.x, end.y))
	await vi.advanceTimersByTimeAsync(800)
}
