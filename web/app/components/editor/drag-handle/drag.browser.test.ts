import type { JSONContent } from "@tiptap/core"
import { Editor } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import { ContentDocument } from "../blocks/document"
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
import {
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../blocks/node-names"
import { Drag, type DragHandleDragging } from "./drag"
import { GapDecorations, enableGapZones } from "./gap-decorations"
import {
	blockElement,
	blockGrid,
	bulletList,
	GRID_STYLES,
	GridLayout,
	metricBlock,
	pointIn,
	posOfMetricBlock,
	RenderedBlockGrid,
	SplitDocumentationLeftSideStub,
	SplitDocumentationRightSideStub,
	SplitDocumentationStub,
	storedCells,
	taskList,
	TitledMetricBlock,
} from "./test-helpers"
import { paragraph } from "~/components/editor/test-helpers"

// deterministic spacing so every Y-based decision (block midpoints, grid
// rows, gap zones) lands the same way on every run
const style = document.createElement("style")
style.textContent = `
	body { margin: 0; }
	.ProseMirror p, .ProseMirror h1 { margin: 8px 0; }
	.ProseMirror ul, .ProseMirror ol { margin: 8px 0; padding-left: 32px; }
	.ProseMirror li { margin: 4px 0; }
	.ProseMirror blockquote { margin: 8px 0 8px 16px; }
	${GRID_STYLES}
`
document.head.appendChild(style)

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

function splitDocumentation(...rightSide: JSONContent[]): JSONContent {
	return {
		type: SPLIT_DOCUMENTATION_NAME,
		content: [
			{
				type: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
				content: [paragraph("left")],
			},
			{ type: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, content: rightSide },
		],
	}
}

// prosemirror copies while the copy key is held. That key is alt on a Mac
// and ctrl elsewhere.
const COPY_KEYS = { altKey: true, ctrlKey: true }

const cleanups: (() => void)[] = []

function mountEditor(
	content: JSONContent[],
	opts: {
		onBeforeDrop?: () => void
		width?: number
		gaps?: boolean
		wideGrid?: boolean
		narrowGrid?: boolean
	} = {},
): Editor {
	const container = document.createElement("div")
	container.className = opts.wideGrid
		? "wide-grid"
		: opts.narrowGrid
			? "narrow-grid"
			: ""
	container.style.width = `${opts.width ?? 400}px`
	container.style.marginTop = "40px"
	container.style.position = "relative"
	document.body.appendChild(container)

	const editor = new Editor({
		element: container,
		extensions: [
			ContentDocument,
			Paragraph,
			Text,
			Heading,
			Blockquote,
			BulletList,
			OrderedList,
			ListItem,
			TaskList,
			TaskItem.configure({ nested: true }),
			RenderedBlockGrid,
			TitledMetricBlock,
			GridLayout,
			SplitDocumentationStub,
			SplitDocumentationLeftSideStub,
			SplitDocumentationRightSideStub,
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

function makeNode(editor: Editor, typeName: string): PMNode {
	const type = editor.schema.nodes[typeName]
	const created = type?.createAndFill()

	if (!created) {
		throw new Error(`the test schema cannot build a "${typeName}"`)
	}

	return created
}

// the line through the middle of the drop cursor, in viewport pixels
function lineOf() {
	const box = cursorElem()?.getBoundingClientRect()

	if (!box) {
		throw new Error("no drop cursor")
	}

	return box.width < box.height
		? { x: box.left + box.width / 2, top: box.top, bottom: box.bottom }
		: { y: box.top + box.height / 2, left: box.left, right: box.right }
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

function dragEvent(
	type: string,
	point: Point,
	transfer = false,
	keys: { altKey?: boolean; ctrlKey?: boolean } = {},
): DragEvent {
	return new DragEvent(type, {
		bubbles: true,
		cancelable: true,
		clientX: point.x,
		clientY: point.y,
		dataTransfer: transfer ? new DataTransfer() : null,
		...keys,
	})
}

function dragOver(
	editor: Editor,
	point: Point,
	keys: { altKey?: boolean; ctrlKey?: boolean } = {},
): DragEvent {
	const event = dragEvent("dragover", point, false, keys)
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

		it("takes a drop onto the dragged block itself without moving it", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			const point = topOf(q(editor, "p", 0))

			expect(callHandleDrop(editor, point)).toBe(true)
			expect(editor.state.doc.toString()).toBe(
				`doc(paragraph("one"), paragraph("two"))`,
			)
		})

		it("leaves the document and the focus alone when a block drops onto itself", ({
			expect,
		}) => {
			const editor = mountEditor([paragraph("one"), paragraph("two")])
			startDrag(editor, 0)
			const onTransaction = vi.fn()
			editor.on("transaction", onTransaction)

			drop(editor, centerOf(q(editor, "p", 0)))

			expect.soft(onTransaction).not.toHaveBeenCalled()
			expect.soft(editor.view.hasFocus()).toBe(false)
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
				[splitDocumentation(metricBlock("a"), metricBlock("b"))],
				{ gaps: true },
			)
			const item = makeNode(editor, "listItem")
			editor.view.dragging = {
				slice: new Slice(Fragment.from(item), 0, 0),
				move: false,
				parentListType: "bulletList",
			} as DragHandleDragging
			await nextFrames()

			// a split documentation's right side holds no lists
			const gap = forceGapZone("type-splitDocumentationRightSide-pos-")
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

	describe("split documentation right side drops", () => {
		it.for([
			{
				name: "drops before a metric block from its upper half",
				input: { dragged: "b", over: 0, y: 0.25 },
				expected: ["b", "a"],
			},
			{
				name: "drops after a metric block from its lower half",
				input: { dragged: "a", over: 1, y: 0.75 },
				expected: ["b", "a"],
			},
		])("$name", ({ input, expected }, { expect }) => {
			const editor = mountEditor([
				splitDocumentation(metricBlock("a"), metricBlock("b")),
			])
			startDrag(editor, posOfMetricBlock(editor, input.dragged))
			const target = q(editor, '[data-type="metric-block"]', input.over)
			const rect = target.getBoundingClientRect()
			const point = {
				x: rect.left + rect.width / 2,
				y: rect.top + rect.height * input.y,
			}
			dragOver(editor, point)

			drop(editor, point)

			expect(rightSideTitles(editor)).toEqual(expected)
		})

		it("drops a block from outside after the whole split documentation", ({
			expect,
		}) => {
			const editor = mountEditor([
				paragraph("one"),
				splitDocumentation(metricBlock("a"), metricBlock("b")),
			])
			startDrag(editor, 0)
			const target = q(editor, '[data-type="metric-block"]', 1)
			const rect = target.getBoundingClientRect()
			const point = { x: rect.left + rect.width / 2, y: rect.bottom - 2 }
			dragOver(editor, point)

			drop(editor, point)

			expect(editor.state.doc.lastChild?.textContent).toBe("one")
		})
	})

	describe("grid drops", () => {
		it("shows the line of a grid drop and moves the block there on drop", ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					blockGrid(metricBlock("a"), metricBlock("b")),
					blockGrid(metricBlock("x")),
				],
				{ wideGrid: true },
			)
			startDrag(editor, posOfMetricBlock(editor, "x"))
			const box = blockElement(editor, "b").getBoundingClientRect()
			const point = pointIn(blockElement(editor, "b"), 0.1, 0.5)

			dragOver(editor, point)

			expect(lineOf()).toEqual({
				x: box.left - 2,
				top: box.top,
				bottom: box.bottom,
			})

			drop(editor, point)

			// the dragged block left its grid empty, so that grid is deleted
			expect(editor.state.doc.childCount).toBe(1)
			expect(storedCells(editor)).toEqual([
				"a 0,0 12x1",
				"b 12,1 12x1",
				"x 12,0 12x1",
			])
		})

		it("copies a block it does not move", ({ expect }) => {
			const editor = mountEditor(
				[blockGrid(metricBlock("a"), metricBlock("b"))],
				{
					wideGrid: true,
				},
			)
			startDrag(editor, posOfMetricBlock(editor, "a"), { move: false })
			const point = pointIn(blockElement(editor, "b"), 0.5, 0.9)

			dragOver(editor, point, COPY_KEYS)
			editor.view.dom.dispatchEvent(dragEvent("drop", point, true, COPY_KEYS))

			expect(storedCells(editor)).toEqual([
				"a 0,0 12x1",
				"b 12,0 12x1",
				"a 12,1 12x1",
			])
		})

		it("shows where a copy lands over the dragged block while the copy key is held", ({
			expect,
		}) => {
			const editor = mountEditor(
				[blockGrid(metricBlock("a"), metricBlock("b"))],
				{ wideGrid: true },
			)
			startDrag(editor, posOfMetricBlock(editor, "a"))
			const box = blockElement(editor, "a").getBoundingClientRect()

			dragOver(editor, pointIn(blockElement(editor, "a"), 0.5, 0.9), COPY_KEYS)

			expect(lineOf()).toEqual({
				y: box.bottom + 2,
				left: box.left,
				right: box.right,
			})
		})

		it("hides the cursor and leaves the grid alone where a drop would change nothing", ({
			expect,
		}) => {
			const editor = mountEditor(
				[blockGrid(metricBlock("a"), metricBlock("b"))],
				{ wideGrid: true },
			)
			startDrag(editor, posOfMetricBlock(editor, "a"))
			const point = pointIn(blockElement(editor, "a"), 0.5, 0.5)

			dragOver(editor, point)

			expect(cursorVisible()).toBe(false)

			drop(editor, point)

			expect(storedCells(editor)).toEqual([
				"a null,null 12x1",
				"b null,null 12x1",
			])
		})

		it("lets a grid block dropped outside a grid start a new grid at its first cell", ({
			expect,
		}) => {
			const editor = mountEditor(
				[
					blockGrid(
						metricBlock("a", { widePosX: 0, widePosY: 0 }),
						metricBlock("b", { widePosX: 12, widePosY: 0 }),
					),
					splitDocumentation(metricBlock("c")),
				],
				{ wideGrid: true },
			)
			// this is a copy from outside the editor, so no source container
			// blocks the drop
			editor.view.dragging = {
				slice: new Slice(
					Fragment.from(editor.state.doc.child(0).child(1)),
					0,
					0,
				),
				move: false,
			}
			const point = pointIn(blockElement(editor, "c"), 0.5, 0.75)

			dragOver(editor, point)
			drop(editor, point)

			expect(editor.state.doc.childCount).toBe(3)
			expect(storedCells(editor, 2)).toEqual(["b null,null 12x1"])
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

function rightSideTitles(editor: Editor): string[] {
	const titles: string[] = []

	editor.state.doc.descendants((node) => {
		if (node.type.name === METRIC_BLOCK_NAME) {
			titles.push(node.attrs.title as string)
		}

		return true
	})

	return titles
}
