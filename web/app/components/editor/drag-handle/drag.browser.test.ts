import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import {
	BulletList,
	ListItem,
	OrderedList,
	TaskItem,
	TaskList,
} from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { Fragment, type Node as PMNode, Slice } from "@tiptap/pm/model"
import { NodeSelection } from "@tiptap/pm/state"
import { afterEach, describe, it, vi } from "vitest"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../blocks/node-names"
import { Drag, type DragHandleDragging } from "./drag"
import { GapDecorations, enableGapZones } from "./gap-decorations"
import { bulletList, taskList } from "./test-helpers"
import {
	MetricBlockStub,
	MetricGridStub,
	paragraph,
} from "~/components/editor/test-helpers"

// deterministic spacing so every Y-based decision (block midpoints, grid
// rows, gap zones) lands the same way on every run
const style = document.createElement("style")
style.textContent = `
	body { margin: 0; }
	.ProseMirror p, .ProseMirror h1 { margin: 8px 0; }
	.ProseMirror ul, .ProseMirror ol { margin: 8px 0; padding-left: 32px; }
	.ProseMirror li { margin: 4px 0; }
	.ProseMirror blockquote { margin: 8px 0 8px 16px; }
	[data-type="metric-grid"] {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		min-height: 24px;
		margin: 8px 0;
	}
	[data-type="metric-block"] {
		width: 100px;
		height: 40px;
	}
`
document.head.appendChild(style)

// the drag tests lay the metric blocks out, so the shared stand-ins
// pick up the DOM and the title the layout assertions read
const MetricGrid = MetricGridStub.extend({
	selectable: false,

	renderHTML() {
		return ["div", { "data-type": "metric-grid" }, 0]
	},
})

const MetricBlock = MetricBlockStub.extend({
	defining: true,
	selectable: false,

	addAttributes() {
		return { title: { default: "" } }
	},

	renderHTML({ HTMLAttributes }) {
		return ["div", { ...HTMLAttributes, "data-type": "metric-block" }]
	},
})

function listItem(text: string): JSONContent {
	return { type: "listItem", content: [paragraph(text)] }
}

function taskItem(text: string): JSONContent {
	return {
		type: "taskItem",
		attrs: { checked: false },
		content: [paragraph(text)],
	}
}

function nestedTaskItem(text: string, nested: JSONContent): JSONContent {
	return {
		type: "taskItem",
		attrs: { checked: false },
		content: [paragraph(text), nested],
	}
}

function metricGrid(...blocks: JSONContent[]): JSONContent {
	return { type: METRIC_GRID_NAME, content: blocks }
}

function metricBlock(title: string): JSONContent {
	return { type: METRIC_BLOCK_NAME, attrs: { title } }
}

const cleanups: (() => void)[] = []

function mountEditor(
	content: JSONContent[],
	opts: { onBeforeDrop?: () => void; width?: number; gaps?: boolean } = {},
): Editor {
	const container = document.createElement("div")
	container.style.width = `${opts.width ?? 400}px`
	container.style.marginTop = "40px"
	container.style.position = "relative"
	document.body.appendChild(container)

	const editor = new Editor({
		element: container,
		extensions: [
			Document,
			Paragraph,
			Text,
			Heading,
			Blockquote,
			BulletList,
			OrderedList,
			ListItem,
			TaskList,
			TaskItem.configure({ nested: true }),
			MetricGrid,
			MetricBlock,
			...(opts.gaps ? [GapDecorations] : []),
			Drag.configure({ onBeforeDrop: opts.onBeforeDrop }),
		],
		content: { type: "doc", content },
	})

	cleanups.push(() => {
		if (!editor.isDestroyed) {
			// dragend is the only reset for the module-level drag state,
			// which outlives the editor that produced it
			editor.view.dom.dispatchEvent(new DragEvent("dragend", { bubbles: true }))
			editor.destroy()
		}

		container.remove()
	})

	return editor
}

function cursorElem(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".pm-root-dropcursor")
}

function cursorVisible(): boolean {
	return cursorElem()?.style.display === "block"
}

function q(editor: Editor, selector: string, index = 0): HTMLElement {
	const el = editor.view.dom.querySelectorAll<HTMLElement>(selector)[index]

	if (!el) {
		throw new Error(`no element for ${selector}[${index}]`)
	}

	return el
}

// gap keys carry the parent node position, so tests match on the stable
// prefix of the key rather than spelling out the whole thing
function gapZone(keyPrefix: string): HTMLElement {
	const el = [...document.querySelectorAll<HTMLElement>("[data-gap-key]")].find(
		(candidate) =>
			candidate.getAttribute("data-gap-key")?.startsWith(keyPrefix),
	)

	if (!el) {
		throw new Error(`no gap zone for ${keyPrefix}`)
	}

	return el
}

// a gap zone only becomes hit-testable once a drag enables it; these
// cases need one that stayed enabled for content it cannot accept
function forceGapZone(keyPrefix: string): HTMLElement {
	const el = gapZone(keyPrefix)
	el.style.pointerEvents = "auto"

	return el
}

interface Point {
	x: number
	y: number
}

function centerOf(el: Element): Point {
	const rect = el.getBoundingClientRect()

	return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
}

function topOf(el: Element): Point {
	const rect = el.getBoundingClientRect()

	return { x: rect.left + rect.width / 2, y: rect.top + 2 }
}

function bottomOf(el: Element): Point {
	const rect = el.getBoundingClientRect()

	return { x: rect.left + rect.width / 2, y: rect.bottom - 2 }
}

// position of the doc's nth top-level child
function childPos(editor: Editor, index: number): number {
	let pos = 0

	for (let i = 0; i < index; i++) {
		pos += editor.state.doc.child(i).nodeSize
	}

	return pos
}

function posOfText(editor: Editor, typeName: string, text: string): number {
	const matches: number[] = []

	editor.state.doc.descendants((node, pos) => {
		if (node.type.name === typeName && node.textContent === text) {
			matches.push(pos)
		}

		return true
	})

	const [first] = matches

	if (first === undefined) {
		throw new Error(`no ${typeName} node containing "${text}"`)
	}

	return first
}

function posOfMetricBlock(editor: Editor, title: string): number {
	const matches: number[] = []

	editor.state.doc.descendants((node, pos) => {
		if (node.attrs.title === title) {
			matches.push(pos)
		}

		return true
	})

	const [first] = matches

	if (first === undefined) {
		throw new Error(`no metric block titled "${title}"`)
	}

	return first
}

function makeNode(editor: Editor, typeName: string): PMNode {
	const type = editor.schema.nodes[typeName]
	const created = type?.createAndFill()

	if (!created) {
		throw new Error(`the test schema cannot build a "${typeName}"`)
	}

	return created
}

// the metric block titles of every grid in the doc, in document order
function gridShape(editor: Editor): string[][] {
	const grids: string[][] = []

	editor.state.doc.descendants((node) => {
		if (node.type.name === METRIC_GRID_NAME) {
			grids.push(node.children.map((child) => child.attrs.title as string))
			return false
		}

		return true
	})

	return grids
}

// mirrors handleDragData in handle-plugin.ts: select the dragged node and
// put a single-node slice plus the drag metadata on view.dragging
function startDrag(
	editor: Editor,
	nodePos: number,
	extra: Partial<DragHandleDragging> = {},
) {
	const { view } = editor
	const node = view.state.doc.nodeAt(nodePos)

	if (!node) {
		throw new Error(`no node at ${nodePos}`)
	}

	view.dispatch(
		view.state.tr.setSelection(NodeSelection.create(view.state.doc, nodePos)),
	)

	view.dragging = {
		slice: new Slice(Fragment.from(node), 0, 0),
		move: true,
		parentListType: null,
		sourceWrapperBounds: null,
		...extra,
	}
}

function dragEvent(type: string, point: Point, transfer = false): DragEvent {
	return new DragEvent(type, {
		bubbles: true,
		cancelable: true,
		clientX: point.x,
		clientY: point.y,
		dataTransfer: transfer ? new DataTransfer() : null,
	})
}

function dragOver(editor: Editor, point: Point): DragEvent {
	const event = dragEvent("dragover", point)
	editor.view.dom.dispatchEvent(event)

	return event
}

// a dragover that never reaches prosemirror's own DOM tree, the case the
// document-level handlers exist for
function globalDragOver(point: Point): DragEvent {
	const event = dragEvent("dragover", point)
	document.body.dispatchEvent(event)

	return event
}

// a real drop, so prosemirror's own drop handler clears view.dragging
// before the plugin's handleDrop runs, exactly as it does in the browser
function drop(editor: Editor, point: Point) {
	editor.view.dom.dispatchEvent(dragEvent("drop", point, true))
}

function globalDrop(point: Point) {
	document.body.dispatchEvent(dragEvent("drop", point, true))
}

// invokes handleDrop the way prosemirror does, but without its fallback
// drop behaviour, so a refused drop leaves the document untouched
function callHandleDrop(
	editor: Editor,
	point: Point,
	opts: { slice?: Slice; moved?: boolean } = {},
): boolean {
	const dragging = editor.view.dragging
	editor.view.dragging = null

	const slice = opts.slice ?? dragging?.slice ?? Slice.empty
	const moved = opts.moved ?? dragging !== null

	editor.view.someProp("transformPasted", (f) => f(slice, editor.view, false))

	return !!editor.view.someProp("handleDrop", (f) =>
		f(editor.view, dragEvent("drop", point, true), slice, moved),
	)
}

function nextFrames(): Promise<void> {
	return new Promise((resolve) => {
		requestAnimationFrame(() => {
			requestAnimationFrame(() => {
				resolve()
			})
		})
	})
}

// gap widgets position themselves in a frame of their own, and only
// become hit-testable once the drag enables them
async function settleGapZones(editor: Editor, draggedPos: number) {
	await nextFrames()

	const node = editor.state.doc.nodeAt(draggedPos)

	if (!node) {
		throw new Error(`no node at ${draggedPos}`)
	}

	enableGapZones(editor, node, draggedPos)
}

// the suites drive real editors in the one shared page and share the
// module level drag state of drag.ts, so they cannot interleave
describe("Drag", { concurrent: false }, () => {
	afterEach(() => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}
	})

	describe("drop cursor lifecycle", () => {
		it("mounts a hidden drop cursor next to the editor", ({ expect }) => {
			const editor = mountEditor([paragraph("one")])

			const cursor = cursorElem()

			expect(cursor).not.toBeNull()
			expect(cursor?.style.display).toBe("none")
			expect(cursor?.parentElement).toBe(editor.view.dom.parentElement)
		})

		it("removes the drop cursor when the editor is destroyed", ({ expect }) => {
			const editor = mountEditor([paragraph("one")])

			editor.destroy()

			expect(cursorElem()).toBeNull()
		})

		it("ignores dragover while nothing is being dragged", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])

			dragOver(editor, topOf(q(editor, "p", 1)))

			expect(cursorVisible()).toBe(false)
		})

		it("shows the cursor for external content seen by transformPasted", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			const pasted = new Slice(
				Fragment.from(makeNode(editor, "paragraph")),
				0,
				0,
			)

			editor.view.someProp("transformPasted", (f) =>
				f(pasted, editor.view, false),
			)
			dragOver(editor, topOf(q(editor, "p", 1)))

			expect(cursorVisible()).toBe(true)
		})

		it("hides the cursor on dragend", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			dragOver(editor, bottomOf(q(editor, "p", 1)))
			expect(cursorVisible()).toBe(true)

			editor.view.dom.dispatchEvent(new DragEvent("dragend", { bubbles: true }))

			expect(cursorVisible()).toBe(false)
		})

		it("hides the cursor when the drag leaves the editor", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			dragOver(editor, bottomOf(q(editor, "p", 1)))

			editor.view.dom.dispatchEvent(
				new DragEvent("dragleave", { bubbles: true, relatedTarget: null }),
			)

			expect(cursorVisible()).toBe(false)
		})

		it("keeps the cursor when the drag moves between elements inside the editor", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			dragOver(editor, bottomOf(q(editor, "p", 1)))

			editor.view.dom.dispatchEvent(
				new DragEvent("dragleave", {
					bubbles: true,
					relatedTarget: q(editor, "p", 1),
				}),
			)

			expect(cursorVisible()).toBe(true)
		})

		it("hides the cursor on mouseup anywhere in the document", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			dragOver(editor, bottomOf(q(editor, "p", 1)))

			document.body.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }))

			expect(cursorVisible()).toBe(false)
		})
	})

	describe("block drop positions", () => {
		it("moves the dragged block above the hovered block", ({ expect }) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, childPos(editor, 2))

			dragOver(editor, topOf(q(editor, "p", 0)))
			expect(cursorVisible()).toBe(true)

			drop(editor, topOf(q(editor, "p", 0)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("three"), paragraph("one"), paragraph("two"))`,
			)
		})

		it("moves the dragged block below the hovered block", ({ expect }) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, 0)

			dragOver(editor, bottomOf(q(editor, "p", 2)))
			drop(editor, bottomOf(q(editor, "p", 2)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("three"), paragraph("one"))`,
			)
		})

		it("keeps the cursor hidden over the dragged block itself", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			dragOver(editor, centerOf(q(editor, "p", 0)))

			expect(cursorVisible()).toBe(false)
		})

		it("refuses a drop onto the dragged block itself", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			const point = topOf(q(editor, "p", 0))

			expect(callHandleDrop(editor, point)).toBe(false)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("hides the cursor over an element that opts out of drops", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			q(editor, "p", 1).classList.add("drag-handle-ignore-self")

			dragOver(editor, centerOf(q(editor, "p", 1)))

			expect(cursorVisible()).toBe(false)
		})

		it("reports no drop position over an element that opts out of drops", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			q(editor, "p", 1).classList.add("drag-handle-ignore-self")

			expect(callHandleDrop(editor, centerOf(q(editor, "p", 1)))).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("hides the cursor when the point resolves to no document position", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			vi.spyOn(editor.view, "posAtCoords").mockReturnValue(null)

			dragOver(editor, bottomOf(q(editor, "p", 1)))

			expect(cursorVisible()).toBe(false)
		})

		it("drops at the document start when the point resolves to the root", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, childPos(editor, 1))
			vi.spyOn(editor.view, "posAtCoords").mockReturnValue({
				pos: 0,
				inside: -1,
			})

			const point = topOf(q(editor, "p", 0))
			dragOver(editor, point)
			expect(cursorVisible()).toBe(true)

			expect(callHandleDrop(editor, point)).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("one"))`,
			)
		})

		it("treats a blockquote as one unit when hovering its inner paragraph", ({
			expect,
		}) => {
			const editor = mountEditor([
				{ type: "blockquote", content: [paragraph("quoted")] },
				paragraph("one"),
			])
			startDrag(editor, childPos(editor, 1))

			const point = topOf(q(editor, "blockquote p"))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), blockquote(paragraph("quoted")))`,
			)
		})

		it("falls back to position distance when the target has no DOM element", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, 0)
			vi.spyOn(editor.view, "nodeDOM").mockReturnValue(null)

			const point = bottomOf(q(editor, "p", 2))
			dragOver(editor, point)

			expect(callHandleDrop(editor, point)).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("three"), paragraph("one"))`,
			)
		})

		it("places the cursor after the last block when hovering below it", ({
			expect,
		}) => {
			const editor = mountEditor([
				{
					type: "heading",
					attrs: { level: 1 },
					content: [{ type: "text", text: "title" }],
				},
				paragraph("one"),
			])
			startDrag(editor, 0)

			dragOver(editor, bottomOf(q(editor, "p", 0)))

			const editorTop = editor.view.dom.getBoundingClientRect().top
			const lastBottom = q(editor, "p", 0).getBoundingClientRect().bottom

			expect(cursorVisible()).toBe(true)
			expect(cursorElem()?.style.height).toBe("0.1875rem")
			// positioned near the bottom edge of the last block, minus the
			// type specific offset
			expect(
				Math.abs(
					parseFloat(cursorElem()?.style.top ?? "0") - (lastBottom - editorTop),
				),
			).toBeLessThan(24)
		})

		it("treats a list as one unit when dragging a plain block over it", ({
			expect,
		}) => {
			const editor = mountEditor([
				bulletList(listItem("a"), listItem("b")),
				paragraph("one"),
			])
			startDrag(editor, childPos(editor, 1))

			const point = topOf(q(editor, "li", 0))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), bulletList(listItem(paragraph("a")), listItem(paragraph("b"))))`,
			)
		})
	})

	describe("list drops", () => {
		it("reorders items inside their own list", ({ expect }) => {
			const editor = mountEditor([
				bulletList(listItem("a"), listItem("b"), listItem("c")),
			])
			startDrag(editor, posOfText(editor, "listItem", "c"), {
				parentListType: "bulletList",
			})

			const point = topOf(q(editor, "li", 0))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("c")), listItem(paragraph("a")), listItem(paragraph("b"))))`,
			)
		})

		it("refuses to drop a list item outside its list without a gap zone", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				bulletList(listItem("a"), listItem("b")),
			])
			startDrag(editor, posOfText(editor, "listItem", "a"), {
				parentListType: "bulletList",
			})

			dragOver(editor, topOf(q(editor, "p", 0)))

			expect(cursorVisible()).toBe(false)
		})

		it("drops through a gap zone between list items", async ({ expect }) => {
			const editor = mountEditor(
				[bulletList(listItem("a"), listItem("b"), listItem("c"))],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "c")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			const point = centerOf(gapZone("type-bulletList-pos-0:before:idx-5"))
			dragOver(editor, point)
			expect(cursorVisible()).toBe(true)

			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("a")), listItem(paragraph("c")), listItem(paragraph("b"))))`,
			)
		})

		it("attaches to the end of a same-type list when hovering the next list", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[bulletList(listItem("a"), listItem("b")), taskList(taskItem("t"))],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "a")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			// the gap before the task item sits inside a list of a different
			// type, so the whole task list is treated as atomic
			const taskListPos = posOfText(editor, "taskList", "t")
			const gap = gapZone(`type-taskList-pos-${taskListPos}:before:idx-0`)
			const point = {
				x: centerOf(gap).x,
				y: gap.getBoundingClientRect().top + 1,
			}
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b")), listItem(paragraph("a"))), taskList(taskItem(paragraph("t"))))`,
			)
		})

		it("wraps a list item dropped after a list of a different type", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[bulletList(listItem("a"), listItem("b")), taskList(taskItem("t"))],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "a")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			const gap = gapZone("doc:after:")
			const point = centerOf(gap)
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b"))), taskList(taskItem(paragraph("t"))), bulletList(listItem(paragraph("a"))))`,
			)
		})

		it("wraps an externally dropped list item in a bullet list", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			const item = makeNode(editor, "listItem")

			expect(
				callHandleDrop(editor, topOf(q(editor, "p", 0)), {
					slice: new Slice(Fragment.from(item), 0, 0),
					moved: false,
				}),
			).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph)), paragraph("one"), paragraph("two"))`,
			)
		})

		it("wraps an externally dropped task item in a task list", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			const item = makeNode(editor, "taskItem")

			expect(
				callHandleDrop(editor, topOf(q(editor, "p", 0)), {
					slice: new Slice(Fragment.from(item), 0, 0),
					moved: false,
				}),
			).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(taskList(taskItem(paragraph)), paragraph("one"), paragraph("two"))`,
			)
		})

		it("refuses a drop when the drag's list type is not in the schema", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, posOfText(editor, "paragraph", "one"), {
				parentListType: "nonexistentList",
			})
			const item = makeNode(editor, "listItem")
			const point = bottomOf(q(editor, "p", 1))

			dragOver(editor, point)

			expect(
				callHandleDrop(editor, point, {
					slice: new Slice(Fragment.from(item), 0, 0),
				}),
			).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("refuses a drop of mixed list item and block content", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			const item = makeNode(editor, "listItem")
			const block = makeNode(editor, "paragraph")

			expect(
				callHandleDrop(editor, topOf(q(editor, "p", 0)), {
					slice: new Slice(Fragment.fromArray([item, block]), 0, 0),
					moved: false,
				}),
			).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("attaches to the start of the same-type list that follows the gap", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[bulletList(listItem("a"), listItem("b")), paragraph("one")],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "b")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			const gap = gapZone("doc:before:idx-0")
			const point = {
				x: centerOf(gap).x,
				y: gap.getBoundingClientRect().top + 1,
			}
			expect(document.elementFromPoint(point.x, point.y)).toBe(gap)

			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b")), listItem(paragraph("a"))), paragraph("one"))`,
			)
		})

		it("refuses a list item over a gap zone that cannot hold one", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[metricGrid(metricBlock("a"), metricBlock("b")), paragraph("one")],
				{ gaps: true },
			)
			const item = makeNode(editor, "listItem")
			editor.view.dragging = {
				slice: new Slice(Fragment.from(item), 0, 0),
				move: false,
				parentListType: "bulletList",
			} as DragHandleDragging
			await nextFrames()

			// the gap between the two blocks, the only one of the grid's
			// vertical gaps that is not off the left edge of the page
			const gap = forceGapZone("type-metricGrid-pos-0:before:idx-1")
			const point = centerOf(gap)
			expect(document.elementFromPoint(point.x, point.y)).toBe(gap)

			dragOver(editor, point)

			expect(cursorVisible()).toBe(false)
		})

		it("drops past a foreign list when a gap below its midpoint is hovered", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					bulletList(listItem("a"), listItem("b")),
					taskList(nestedTaskItem("t", bulletList(listItem("c")))),
				],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "a")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			const nestedListPos = posOfText(editor, "bulletList", "c")
			const point = centerOf(gapZone(`type-bulletList-pos-${nestedListPos}:`))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b"))), taskList(taskItem(paragraph("t"), bulletList(listItem(paragraph("c"))))), bulletList(listItem(paragraph("a"))))`,
			)
		})

		it("measures the atomic list by position when it has no DOM element", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					bulletList(listItem("a"), listItem("b")),
					taskList(nestedTaskItem("t", bulletList(listItem("c")))),
				],
				{ gaps: true },
			)
			const draggedPos = posOfText(editor, "listItem", "a")
			startDrag(editor, draggedPos, { parentListType: "bulletList" })
			await settleGapZones(editor, draggedPos)

			const nestedListPos = posOfText(editor, "bulletList", "c")
			const point = centerOf(gapZone(`type-bulletList-pos-${nestedListPos}:`))
			vi.spyOn(editor.view, "nodeDOM").mockReturnValue(null)

			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(bulletList(listItem(paragraph("b")), listItem(paragraph("a"))), taskList(taskItem(paragraph("t"), bulletList(listItem(paragraph("c"))))))`,
			)
		})
	})

	describe("metric grid drops", () => {
		it("shows a vertical cursor inside the grid and reorders blocks", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			startDrag(editor, posOfMetricBlock(editor, "c"))

			const secondBlock = q(editor, '[data-type="metric-block"]', 1)
			const rect = secondBlock.getBoundingClientRect()
			const point = {
				x: rect.left + rect.width * 0.25,
				y: centerOf(secondBlock).y,
			}
			dragOver(editor, point)

			expect(cursorVisible()).toBe(true)
			expect(cursorElem()?.style.width).toBe("0.1875rem")
			expect(parseFloat(cursorElem()?.style.height ?? "0")).toBeGreaterThan(0)

			drop(editor, point)

			expect(gridShape(editor)).toEqual([["a", "c", "b"]])
		})

		it("inserts before the first block when hovering the left margin", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			startDrag(editor, posOfMetricBlock(editor, "c"))

			const grid = q(editor, '[data-type="metric-grid"]')
			grid.style.marginLeft = "80px"
			const gridRect = grid.getBoundingClientRect()
			const point = { x: gridRect.left - 20, y: gridRect.top + 10 }
			dragOver(editor, point)
			drop(editor, point)

			expect(gridShape(editor)).toEqual([["c", "a", "b"]])
		})

		it("inserts after the last block when hovering past its right edge", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			startDrag(editor, posOfMetricBlock(editor, "a"))

			const last = q(editor, '[data-type="metric-block"]', 2)
			const rect = last.getBoundingClientRect()
			const point = { x: rect.right + 20, y: centerOf(last).y }
			dragOver(editor, point)
			drop(editor, point)

			expect(gridShape(editor)).toEqual([["b", "c", "a"]])
		})

		it("inserts between blocks when hovering the gap between them", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			startDrag(editor, posOfMetricBlock(editor, "a"))

			const second = q(editor, '[data-type="metric-block"]', 1)
			const third = q(editor, '[data-type="metric-block"]', 2)
			const point = {
				x:
					(second.getBoundingClientRect().right +
						third.getBoundingClientRect().left) /
					2,
				y: centerOf(second).y,
			}
			dragOver(editor, point)
			drop(editor, point)

			expect(gridShape(editor)).toEqual([["b", "a", "c"]])
		})

		it("refuses the positions immediately around the dragged block", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			startDrag(editor, posOfMetricBlock(editor, "b"), {
				sourceWrapperBounds: {
					start: 0,
					end: editor.state.doc.child(0).nodeSize,
				},
			})

			const second = q(editor, '[data-type="metric-block"]', 1)
			const rect = second.getBoundingClientRect()

			dragOver(editor, { x: rect.left + 5, y: centerOf(second).y })
			expect(cursorVisible()).toBe(false)

			dragOver(editor, { x: rect.right - 5, y: centerOf(second).y })
			expect(cursorVisible()).toBe(false)
		})

		it("hides the cursor at the boundaries of the source grid", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				metricGrid(metricBlock("a"), metricBlock("b")),
			])
			const gridPos = childPos(editor, 1)
			startDrag(editor, posOfMetricBlock(editor, "a"), {
				sourceWrapperBounds: {
					start: gridPos,
					end: gridPos + editor.state.doc.child(1).nodeSize,
				},
			})

			dragOver(editor, bottomOf(q(editor, "p", 0)))

			expect(cursorVisible()).toBe(false)
		})

		it("wraps a block dropped outside a grid and drops the emptied grid", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[paragraph("one"), paragraph("two"), metricGrid(metricBlock("a"))],
				{ gaps: true },
			)
			const gridPos = childPos(editor, 2)
			const draggedPos = posOfMetricBlock(editor, "a")
			startDrag(editor, draggedPos, {
				sourceWrapperBounds: {
					start: gridPos,
					end: gridPos + editor.state.doc.child(2).nodeSize,
				},
			})
			await settleGapZones(editor, draggedPos)

			const point = centerOf(gapZone("doc:before:idx-0"))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(metricGrid(metricBlock), paragraph("one"), paragraph("two"))`,
			)
			expect(gridShape(editor)).toEqual([["a"]])
		})

		it("drops into an empty grid", ({ expect }) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b")),
				metricGrid(),
			])
			startDrag(editor, posOfMetricBlock(editor, "b"))

			const emptyGrid = q(editor, '[data-type="metric-grid"]', 1)
			const point = centerOf(emptyGrid)
			dragOver(editor, point)
			drop(editor, point)

			expect(gridShape(editor)).toEqual([["a"], ["b"]])
		})

		it("keeps the drop on the hovered row of a wrapped grid", ({ expect }) => {
			const editor = mountEditor(
				[
					metricGrid(
						metricBlock("a"),
						metricBlock("b"),
						metricBlock("c"),
						metricBlock("d"),
					),
				],
				{ width: 240 },
			)
			startDrag(editor, posOfMetricBlock(editor, "a"))

			const third = q(editor, '[data-type="metric-block"]', 2)
			const fourth = q(editor, '[data-type="metric-block"]', 3)
			expect(third.getBoundingClientRect().top).toBeGreaterThan(
				q(editor, '[data-type="metric-block"]', 1).getBoundingClientRect()
					.bottom,
			)

			const point = {
				x:
					(third.getBoundingClientRect().right +
						fourth.getBoundingClientRect().left) /
					2,
				y: centerOf(third).y,
			}
			dragOver(editor, point)
			drop(editor, point)

			expect(gridShape(editor)).toEqual([["b", "c", "a", "d"]])
		})

		it("falls back to the closest row when hovering the gap between rows", ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					metricGrid(
						metricBlock("a"),
						metricBlock("b"),
						metricBlock("c"),
						metricBlock("d"),
					),
				],
				{ width: 240 },
			)
			startDrag(editor, posOfMetricBlock(editor, "d"))

			const second = q(editor, '[data-type="metric-block"]', 1)
			const third = q(editor, '[data-type="metric-block"]', 2)
			const point = {
				x: centerOf(second).x,
				y:
					(second.getBoundingClientRect().bottom +
						third.getBoundingClientRect().top) /
					2,
			}
			dragOver(editor, point)
			expect(cursorVisible()).toBe(true)

			drop(editor, point)

			expect(gridShape(editor)).toEqual([["a", "b", "d", "c"]])
		})

		it("shows the row-end cursor at a row boundary", async ({ expect }) => {
			const editor = mountEditor(
				[
					metricGrid(
						metricBlock("a"),
						metricBlock("b"),
						metricBlock("c"),
						metricBlock("d"),
					),
				],
				{ width: 240, gaps: true },
			)
			const draggedPos = posOfMetricBlock(editor, "d")
			startDrag(editor, draggedPos)
			await settleGapZones(editor, draggedPos)

			const second = q(editor, '[data-type="metric-block"]', 1)
			const point = {
				x: second.getBoundingClientRect().right + 4,
				y: centerOf(second).y,
			}
			dragOver(editor, point)

			expect(cursorVisible()).toBe(true)
			const cursorTop = parseFloat(cursorElem()?.style.top ?? "0")
			const editorTop = editor.view.dom.getBoundingClientRect().top
			expect(
				Math.abs(cursorTop - (second.getBoundingClientRect().top - editorTop)),
			).toBeLessThan(2)
		})

		it("ignores an empty drag slice", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			editor.view.dragging = { slice: Slice.empty, move: true }

			const point = bottomOf(q(editor, "p", 1))
			dragOver(editor, point)
			expect(cursorVisible()).toBe(true)

			expect(callHandleDrop(editor, point, { moved: false })).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("skips grids that have no DOM element", ({ expect }) => {
			const editor = mountEditor([
				paragraph("one"),
				metricGrid(metricBlock("a"), metricBlock("b")),
			])
			startDrag(editor, posOfMetricBlock(editor, "a"))
			const block = q(editor, '[data-type="metric-block"]', 1)
			const point = centerOf(block)
			vi.spyOn(editor.view, "nodeDOM").mockReturnValue(null)

			dragOver(editor, point)

			expect(cursorVisible()).toBe(false)
		})

		it("refuses a drop at a source wrapper boundary", ({ expect }) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c")),
			])
			const secondBlockPos = posOfMetricBlock(editor, "b")
			startDrag(editor, posOfMetricBlock(editor, "c"), {
				sourceWrapperBounds: { start: secondBlockPos, end: secondBlockPos },
			})

			const second = q(editor, '[data-type="metric-block"]', 1)
			const point = {
				x: second.getBoundingClientRect().left + 5,
				y: centerOf(second).y,
			}
			dragOver(editor, point)
			expect(cursorVisible()).toBe(false)

			expect(callHandleDrop(editor, point)).toBe(true)
			expect(gridShape(editor)).toEqual([["a", "b", "c"]])
		})

		it("positions the row-end cursor without gap zones", ({ expect }) => {
			const editor = mountEditor(
				[
					metricGrid(
						metricBlock("a"),
						metricBlock("b"),
						metricBlock("c"),
						metricBlock("d"),
					),
				],
				{ width: 240 },
			)
			startDrag(editor, posOfMetricBlock(editor, "d"))

			const second = q(editor, '[data-type="metric-block"]', 1)
			const point = {
				x: second.getBoundingClientRect().right + 4,
				y: centerOf(second).y,
			}
			dragOver(editor, point)

			const editorRect = editor.view.dom.getBoundingClientRect()
			const expectedLeft =
				second.getBoundingClientRect().right + 4.5 - editorRect.left

			expect(cursorVisible()).toBe(true)
			expect(parseFloat(cursorElem()?.style.left ?? "0")).toBeCloseTo(
				expectedLeft,
				1,
			)
		})

		it("falls back to the editor edge when the grid children have no DOM", ({
			expect,
		}) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a"), metricBlock("b")),
				paragraph("one"),
			])
			editor.commands.setTextSelection(editor.state.doc.content.size - 1)
			const block = makeNode(editor, "metricBlock")
			editor.view.dragging = {
				slice: new Slice(Fragment.from(block), 0, 0),
				move: false,
			}

			const nodeDOM = editor.view.nodeDOM.bind(editor.view)
			vi.spyOn(editor.view, "nodeDOM").mockImplementation((pos) =>
				pos === 0 ? nodeDOM(pos) : null,
			)

			dragOver(editor, centerOf(q(editor, '[data-type="metric-grid"]')))

			expect(cursorVisible()).toBe(true)
			expect(cursorElem()?.style.height).toBe("0px")
			expect(cursorElem()?.style.left).toBe("0px")
		})

		it("refuses a metric block over a gap zone that cannot hold one", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[paragraph("one"), taskList(taskItem("t"), taskItem("u"))],
				{ gaps: true },
			)
			const block = makeNode(editor, "metricBlock")
			editor.view.dragging = {
				slice: new Slice(Fragment.from(block), 0, 0),
				move: false,
			}
			await nextFrames()

			const gap = forceGapZone("type-taskList-pos-")
			const point = centerOf(gap)
			expect(document.elementFromPoint(point.x, point.y)).toBe(gap)

			dragOver(editor, point)

			expect(cursorVisible()).toBe(false)
		})

		it("centres the vertical cursor on the gap zone between two blocks", async ({
			expect,
		}) => {
			const editor = mountEditor(
				[metricGrid(metricBlock("a"), metricBlock("b"), metricBlock("c"))],
				{ gaps: true },
			)
			const draggedPos = posOfMetricBlock(editor, "c")
			startDrag(editor, draggedPos)
			await settleGapZones(editor, draggedPos)

			const first = q(editor, '[data-type="metric-block"]', 0)
			const rect = first.getBoundingClientRect()
			dragOver(editor, { x: rect.right - 5, y: centerOf(first).y })

			const gapRect = gapZone(
				"type-metricGrid-pos-0:before:idx-1",
			).getBoundingClientRect()
			const editorLeft = editor.view.dom.getBoundingClientRect().left

			expect(cursorVisible()).toBe(true)
			expect(parseFloat(cursorElem()?.style.left ?? "0")).toBeCloseTo(
				gapRect.left + gapRect.width / 2 - 1.5 - editorLeft,
				1,
			)
		})

		it("drops into an empty grid nested in another block", ({ expect }) => {
			const editor = mountEditor([
				metricGrid(metricBlock("a")),
				{ type: "blockquote", content: [metricGrid()] },
			])
			startDrag(editor, posOfMetricBlock(editor, "a"))

			const point = centerOf(q(editor, 'blockquote [data-type="metric-grid"]'))
			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.toString()).toBe(
				`doc(blockquote(metricGrid(metricBlock), metricGrid))`,
			)
		})
	})

	describe("drop guards", () => {
		it("notifies onBeforeDrop before dispatching the drop", ({ expect }) => {
			const onBeforeDrop = vi.fn(() => {
				expect(editor.state.doc.child(0).textContent).toBe("one")
			})
			const editor = mountEditor([paragraph("one"), paragraph("two")], {
				onBeforeDrop,
			})
			startDrag(editor, 0)

			const point = bottomOf(q(editor, "p", 1))
			dragOver(editor, point)
			drop(editor, point)

			expect(onBeforeDrop).toHaveBeenCalledTimes(1)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("one"))`,
			)
		})

		it("ignores a second drop from the same drag", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			const point = bottomOf(q(editor, "p", 1))
			dragOver(editor, point)
			expect(callHandleDrop(editor, point)).toBe(true)
			expect(callHandleDrop(editor, point, { moved: false })).toBe(false)

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("one"))`,
			)
		})
	})

	describe("document level drag handlers", () => {
		it("shows the cursor for a dragover that misses the editor", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			const event = globalDragOver(bottomOf(q(editor, "p", 1)))

			expect(cursorVisible()).toBe(true)
			expect(event.defaultPrevented).toBe(true)
		})

		it("leaves the dragover alone when there is no valid position", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			const event = globalDragOver(centerOf(q(editor, "p", 0)))

			expect(cursorVisible()).toBe(false)
			expect(event.defaultPrevented).toBe(false)
		})

		it("inserts the node on a drop that misses the editor", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			globalDragOver(bottomOf(q(editor, "p", 1)))
			globalDrop(bottomOf(q(editor, "p", 1)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("two"), paragraph("one"))`,
			)
		})

		it("ignores a drop with no cursor shown", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)

			globalDrop(bottomOf(q(editor, "p", 1)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("ignores a drop once the drag slice is gone", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			globalDragOver(bottomOf(q(editor, "p", 1)))

			editor.view.dragging = null
			globalDrop(bottomOf(q(editor, "p", 1)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("ignores a drop next to the source wrapper", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			globalDragOver(bottomOf(q(editor, "p", 1)))

			const end = editor.state.doc.content.size
			editor.view.dragging = {
				...editor.view.dragging,
				sourceWrapperBounds: { start: end, end },
			} as DragHandleDragging
			globalDrop(bottomOf(q(editor, "p", 1)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("ignores a drop the position can no longer accept", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			globalDragOver(bottomOf(q(editor, "p", 1)))

			const item = makeNode(editor, "listItem")
			editor.view.dragging = {
				slice: new Slice(Fragment.from(item), 0, 0),
				move: true,
				parentListType: "nonexistentList",
			} as DragHandleDragging
			globalDrop(bottomOf(q(editor, "p", 1)))

			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})
	})

	describe("cursor updates on document changes", () => {
		it("keeps the cursor in place across an unrelated update", ({ expect }) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			dragOver(editor, bottomOf(q(editor, "p", 1)))
			const before = cursorElem()?.style.top

			editor.view.dragging = null
			editor.view.dispatch(editor.state.tr.insertText("!", 3))

			expect(cursorVisible()).toBe(true)
			expect(cursorElem()?.style.top).toBe(before)
		})

		it("hides the cursor when an update invalidates the position", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, 0)
			dragOver(editor, topOf(q(editor, "p", 2)))
			expect(cursorVisible()).toBe(true)

			editor.view.dispatch(
				editor.state.tr.setSelection(
					NodeSelection.create(editor.state.doc, childPos(editor, 2)),
				),
			)

			expect(cursorVisible()).toBe(false)
		})

		it("hides the cursor when the hovered element stops accepting drops", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, 0)
			dragOver(editor, topOf(q(editor, "p", 2)))
			expect(cursorVisible()).toBe(true)

			q(editor, "p", 2).classList.add("drag-handle-ignore-self")
			editor.view.dispatch(editor.state.tr.setMeta("refresh", true))

			expect(cursorVisible()).toBe(false)
		})

		it("hides the cursor when an update lands it next to the source wrapper", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				paragraph("two"),
				paragraph("three"),
			])
			startDrag(editor, 0)
			dragOver(editor, topOf(q(editor, "p", 2)))
			expect(cursorVisible()).toBe(true)

			const boundary = childPos(editor, 2)
			editor.view.dragging = {
				...editor.view.dragging,
				sourceWrapperBounds: { start: boundary, end: boundary },
			} as DragHandleDragging
			editor.view.dispatch(editor.state.tr.setMeta("refresh", true))

			expect(cursorVisible()).toBe(false)
		})
	})
})
