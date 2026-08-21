import { computePosition } from "@floating-ui/dom"
import type { JSONContent } from "@tiptap/core"
import { Editor, Node as TiptapNode } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import { BulletList, ListItem, OrderedList } from "@tiptap/extension-list"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { NodeSelection, PluginKey } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import {
	prosemirrorJSONToYDoc,
	ySyncPlugin,
	ySyncPluginKey,
} from "@tiptap/y-tiptap"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import * as Y from "yjs"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { DragHandlePlugin } from "./handle-plugin"
import type { DragHandleDragging } from "./drag"
import { disableGapZones, enableGapZones } from "./gap-decorations"
import { findDraggableNodeAtCoords } from "./node-detection.js"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../blocks/node-names"
import { CLONE_IGNORE_ATTR } from "~/utils/object"
import { paragraph } from "~/components/editor/test-helpers"

vi.mock("./gap-decorations", () => ({
	enableGapZones: vi.fn(),
	disableGapZones: vi.fn(),
}))

vi.mock("./node-detection.js", () => ({
	findDraggableNodeAtCoords: vi.fn(),
}))

vi.mock("@floating-ui/dom", () => ({
	computePosition: vi.fn(),
}))

function must<T>(value: T | null | undefined, what: string): T {
	if (value === null || value === undefined) {
		throw new Error(`the test needs ${what}`)
	}

	return value
}

// stands in for a node view that paints into a canvas and hangs a
// decorator widget off itself: the drag image cloner treats both
// specially
const CanvasBlock = TiptapNode.create({
	name: "canvasBlock",
	group: "block",
	atom: true,

	renderHTML() {
		return [
			"div",
			{ "data-node-view-wrapper": "" },
			["canvas", { width: "20", height: "20", "data-origin": "source" }],
			["span", { [CLONE_IGNORE_ATTR]: "true" }, "decorator"],
		]
	},
})

// grouped as a block so the tests can also place one outside its grid,
// which is where the wrapper lookup finds nothing
const MetricBlock = TiptapNode.create({
	name: METRIC_BLOCK_NAME,
	group: "block",
	content: "inline*",

	renderHTML() {
		return ["div", { "data-metric-block": "" }, 0]
	},
})

const MetricGrid = TiptapNode.create({
	name: METRIC_GRID_NAME,
	group: "block",
	content: `${METRIC_BLOCK_NAME}+`,

	renderHTML() {
		return ["div", { "data-metric-grid": "" }, 0]
	},
})

// the plugin broadcasts drags by node uid, so the paragraphs it drags
// need one
const UidParagraph = Paragraph.extend({
	addAttributes() {
		return { uid: { default: null } }
	},
})

// same reason as MetricBlock: a list item has to be placeable outside
// any list for the parent-list lookup to come up empty
const BlockListItem = ListItem.extend({ group: "block" })

const extensions = [
	Document,
	UidParagraph,
	Text,
	BulletList,
	OrderedList,
	BlockListItem,
	MetricBlock,
	MetricGrid,
	CanvasBlock,
]

function fakeProvider(otherDraggingUid?: string) {
	const states = new Map<number, Record<string, unknown>>([
		[1, { draggingNodeUid: null }],
	])

	if (otherDraggingUid) {
		states.set(2, { draggingNodeUid: otherDraggingUid })
	}

	const setLocalStateField = vi.fn()
	const provider = {
		awareness: {
			clientID: 1,
			getStates: () => states,
			setLocalStateField,
		},
	} as unknown as HocuspocusProvider

	return { provider, setLocalStateField }
}

type DOMHandlers = NonNullable<Plugin["props"]["handleDOMEvents"]>

function domHandler<K extends keyof DOMHandlers>(plugin: Plugin, name: K) {
	const handler = plugin.props.handleDOMEvents?.[name]

	if (!handler) {
		throw new Error(`the plugin registers no ${String(name)} handler`)
	}

	return handler
}

function draggingOf(view: EditorView): DragHandleDragging | null {
	return view.dragging
}

// three ticks is more than the single `.then` the reposition chain
// schedules, and costs nothing when the queue is already empty
async function flushMicrotasks() {
	await Promise.resolve()
	await Promise.resolve()
	await Promise.resolve()
}

// the suite drives real drag events against editors mounted in the
// shared page and asserts on module-level mocks, so it cannot interleave
describe("DragHandlePlugin", { concurrent: false }, () => {
	const cleanups: (() => void)[] = []

	beforeEach(() => {
		vi.mocked(enableGapZones).mockReset()
		vi.mocked(disableGapZones).mockReset()
		vi.mocked(findDraggableNodeAtCoords).mockReset()
		vi.mocked(computePosition).mockReset()
		vi.mocked(computePosition).mockResolvedValue({
			x: 100,
			y: 200,
			placement: "left-start",
			strategy: "absolute",
			middlewareData: {},
		})

		vi.useFakeTimers()
	})

	afterEach(() => {
		// the drag image cleanup listener lives on document, so a drop has
		// to land even for tests that never dropped anything
		document.dispatchEvent(new Event("drop"))
		vi.useRealTimers()

		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}
	})

	function mountEditor(content: JSONContent[] = [paragraph("one")]): Editor {
		const container = document.createElement("div")
		document.body.appendChild(container)

		const editor = new Editor({
			element: container,
			extensions,
			content: { type: "doc", content },
		})

		cleanups.push(() => {
			editor.destroy()
			container.remove()
		})

		return editor
	}

	interface SetupOptions {
		content?: JSONContent[]
		editor?: Editor
		provider?: HocuspocusProvider
		locked?: boolean
		pluginKey?: PluginKey | string
		getReferencedVirtualElement?: () => { getBoundingClientRect: () => DOMRect }
		register?: boolean
	}

	function setup(opts: SetupOptions = {}) {
		const editor = opts.editor ?? mountEditor(opts.content)
		const element = document.createElement("div")
		const onNodeChange = vi.fn()
		const onElementDragStart = vi.fn()
		const onElementDragEnd = vi.fn()
		const onDragCancel = vi.fn()

		const handle = DragHandlePlugin({
			editor,
			element,
			pluginKey: opts.pluginKey ?? new PluginKey("dragHandleTest"),
			provider: opts.provider,
			locked: opts.locked,
			getReferencedVirtualElement: opts.getReferencedVirtualElement,
			onNodeChange,
			onElementDragStart,
			onElementDragEnd,
			onDragCancel,
		})

		if (opts.register !== false) {
			editor.registerPlugin(handle.plugin)
		}

		return {
			editor,
			element,
			handle,
			onNodeChange,
			onElementDragStart,
			onElementDragEnd,
			onDragCancel,
		}
	}

	function fireMouseMove(editor: Editor, plugin: Plugin, x: number, y: number) {
		return domHandler(plugin, "mousemove").call(
			plugin,
			editor.view,
			new MouseEvent("mousemove", { clientX: x, clientY: y }),
		)
	}

	// drives the plugin's own mousemove path so the internal "current
	// node" bookkeeping ends up exactly where a real hover leaves it
	function hover(
		editor: Editor,
		plugin: Plugin,
		pos: number,
		depth = 1,
	): PMNode {
		const node = must(editor.state.doc.nodeAt(pos), `a node at ${pos}`)
		const dom = must(
			editor.view.nodeDOM(pos),
			`node dom at ${pos}`,
		) as HTMLElement

		vi.mocked(findDraggableNodeAtCoords).mockReturnValue({
			node,
			pos,
			depth,
			dom,
		})
		fireMouseMove(editor, plugin, 5, 7)
		vi.advanceTimersByTime(20)

		return node
	}

	function startDrag(element: HTMLElement, withDataTransfer = true) {
		const dataTransfer = withDataTransfer ? new DataTransfer() : null
		const setDragImage = dataTransfer
			? vi.spyOn(dataTransfer, "setDragImage")
			: null
		const clearData = dataTransfer ? vi.spyOn(dataTransfer, "clearData") : null
		const event = new DragEvent("dragstart", {
			dataTransfer,
			cancelable: true,
			bubbles: true,
		})

		element.dispatchEvent(event)

		return {
			event,
			setDragImage,
			clearData,
			dragImage: () =>
				must(setDragImage?.mock.calls[0]?.[0], "a drag image") as HTMLElement,
		}
	}

	function endDrag(element: HTMLElement, clientX = 0, clientY = 0) {
		const event = new DragEvent("dragend", { clientX, clientY, bubbles: true })
		element.dispatchEvent(event)

		return event
	}

	describe("initial state", () => {
		it("hides the handle inside a detached wrapper before installation", ({
			expect,
		}) => {
			const { element } = setup({ register: false })

			expect(element.style.visibility).toBe("hidden")
			expect(element.style.pointerEvents).toBe("none")
			expect(element.parentElement?.isConnected).toBe(false)
		})

		it("mounts the wrapper beside the editor and enables the handle", ({
			expect,
		}) => {
			const { editor, element } = setup()
			const wrapper = must(element.parentElement, "the handle wrapper")

			expect(wrapper.parentElement).toBe(editor.view.dom.parentElement)
			expect(wrapper.style.position).toBe("absolute")
			expect(wrapper.style.pointerEvents).toBe("none")
			expect(wrapper.style.top).toBe("0px")
			expect(wrapper.style.left).toBe("0px")
			expect(element.style.pointerEvents).toBe("auto")
			expect(element.style.visibility).toBe("hidden")
			expect(element.draggable).toBe(true)
		})

		it("starts with a zero depth and no remote change recorded", ({
			expect,
		}) => {
			const { editor, handle } = setup()

			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: false,
			})
		})

		it("marks the handle non-draggable for a read-only editor", ({
			expect,
		}) => {
			const editor = mountEditor()
			editor.setEditable(false)

			const { element } = setup({ editor })

			expect(element.draggable).toBe(false)
		})

		it("accepts a plugin key given as a string", ({ expect }) => {
			const { editor, handle } = setup({ pluginKey: "stringDragHandle" })

			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: false,
			})
		})

		it("falls back to the shared plugin key when none is given", ({
			expect,
		}) => {
			const editor = mountEditor()
			const element = document.createElement("div")
			const handle = DragHandlePlugin({ editor, element })

			editor.registerPlugin(handle.plugin)

			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: false,
			})
		})
	})

	describe("unbind", () => {
		it("stops reacting to drag events on the handle", ({ expect }) => {
			const { editor, element, handle, onElementDragStart } = setup()
			hover(editor, handle.plugin, 0)

			handle.unbind()
			startDrag(element)

			expect(onElementDragStart).toHaveBeenCalledTimes(0)
			expect(draggingOf(editor.view)).toBeNull()
		})

		it("drops a pending hover frame", ({ expect }) => {
			const { editor, handle } = setup()
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue(null)
			fireMouseMove(editor, handle.plugin, 1, 1)

			handle.unbind()
			vi.advanceTimersByTime(50)

			expect(findDraggableNodeAtCoords).toHaveBeenCalledTimes(0)
		})

		it("is safe to call when no hover frame is pending", ({ expect }) => {
			const { handle } = setup()

			expect(() => {
				handle.unbind()
			}).not.toThrow()
		})
	})

	describe("dragstart", () => {
		it("ignores a drag started before any node is tracked", ({ expect }) => {
			const { editor, element, onElementDragStart } = setup()

			startDrag(element)

			expect(onElementDragStart).toHaveBeenCalledTimes(0)
			expect(enableGapZones).toHaveBeenCalledTimes(0)
			expect(draggingOf(editor.view)).toBeNull()
		})

		it("ignores the drag when no node sits at the tracked position", ({
			expect,
		}) => {
			const { editor, element, handle, onElementDragStart } = setup()
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue({
				node: must(editor.state.doc.firstChild, "a first child"),
				pos: editor.state.doc.content.size,
				depth: 1,
				dom: editor.view.dom,
			})
			fireMouseMove(editor, handle.plugin, 5, 7)
			vi.advanceTimersByTime(20)

			startDrag(element)

			expect(onElementDragStart).toHaveBeenCalledTimes(0)
			expect(enableGapZones).toHaveBeenCalledTimes(0)
			expect(draggingOf(editor.view)).toBeNull()
		})

		it("refuses the drag while another user drags the same node", ({
			expect,
		}) => {
			const { provider, setLocalStateField } = fakeProvider("u1")
			const { editor, element, handle, onDragCancel, onElementDragStart } =
				setup({ content: [paragraph("one", { uid: "u1" })], provider })
			hover(editor, handle.plugin, 0)

			const { event } = startDrag(element)

			expect(event.defaultPrevented).toBe(true)
			expect(onDragCancel).toHaveBeenCalledTimes(1)
			expect(onElementDragStart).toHaveBeenCalledTimes(0)
			expect(enableGapZones).toHaveBeenCalledTimes(0)
			expect(draggingOf(editor.view)).toBeNull()
			expect(setLocalStateField).toHaveBeenCalledTimes(0)
		})

		it("allows the drag while another user drags a different node", ({
			expect,
		}) => {
			const { provider, setLocalStateField } = fakeProvider("other")
			const { editor, element, handle, onElementDragStart } = setup({
				content: [paragraph("one", { uid: "u1" })],
				provider,
			})
			hover(editor, handle.plugin, 0)

			const { event } = startDrag(element)

			expect(event.defaultPrevented).toBe(false)
			expect(onElementDragStart).toHaveBeenCalledTimes(1)
			expect(setLocalStateField).toHaveBeenCalledWith("draggingNodeUid", "u1")
		})

		it("drags the hovered node alone and selects it", ({ expect }) => {
			const { provider, setLocalStateField } = fakeProvider()
			const { editor, element, handle, onElementDragStart } = setup({
				content: [
					paragraph("one", { uid: "u1" }),
					paragraph("two", { uid: "u2" }),
				],
				provider,
			})
			const node = hover(editor, handle.plugin, 0)

			const { event, clearData, setDragImage } = startDrag(element)
			const dragging = draggingOf(editor.view)

			expect(onElementDragStart).toHaveBeenCalledWith(event)
			expect(clearData).toHaveBeenCalledTimes(1)
			expect(setDragImage).toHaveBeenCalledTimes(1)
			expect(dragging?.move).toBe(true)
			expect(dragging?.parentListType).toBeNull()
			expect(dragging?.sourceWrapperBounds).toBeNull()
			expect(dragging?.slice.content.childCount).toBe(1)
			expect(dragging?.slice.content.firstChild?.textContent).toBe("one")
			expect(editor.state.selection).toBeInstanceOf(NodeSelection)
			expect(editor.state.selection.from).toBe(0)
			expect(enableGapZones).toHaveBeenCalledWith(editor, node, 0)
			expect(setLocalStateField).toHaveBeenCalledWith("draggingNodeUid", "u1")
		})

		it("disables pointer events on the handle once the drag is under way", ({
			expect,
		}) => {
			const { editor, element, handle } = setup()
			hover(editor, handle.plugin, 0)

			startDrag(element)
			vi.advanceTimersByTime(1)

			expect(element.style.pointerEvents).toBe("none")
		})

		it("skips the drag image when the event carries no data transfer", ({
			expect,
		}) => {
			const { editor, element, handle, onElementDragStart } = setup()
			const node = hover(editor, handle.plugin, 0)

			startDrag(element, false)

			expect(onElementDragStart).toHaveBeenCalledTimes(1)
			expect(draggingOf(editor.view)).toBeNull()
			expect(enableGapZones).toHaveBeenCalledWith(editor, node, 0)
		})

		it("leaves awareness untouched for a node without a uid", ({ expect }) => {
			const { provider, setLocalStateField } = fakeProvider()
			const { editor, element, handle } = setup({
				content: [paragraph("one")],
				provider,
			})
			hover(editor, handle.plugin, 0)

			startDrag(element)

			expect(draggingOf(editor.view)?.move).toBe(true)
			expect(setLocalStateField).toHaveBeenCalledTimes(0)
		})

		it.for([
			{ name: "bullet list", listType: "bulletList" },
			{ name: "ordered list", listType: "orderedList" },
		])(
			"records the parent $name type when dragging a list item",
			({ listType }, { expect }) => {
				const { editor, element, handle } = setup({
					content: [
						{
							type: listType,
							content: [{ type: "listItem", content: [paragraph("item")] }],
						},
					],
				})
				hover(editor, handle.plugin, 1)

				startDrag(element)

				expect(draggingOf(editor.view)?.parentListType).toBe(listType)
			},
		)

		it("records no parent list type for a list item outside a list", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [{ type: "listItem", content: [paragraph("item")] }],
			})
			hover(editor, handle.plugin, 0)

			startDrag(element)

			expect(draggingOf(editor.view)?.parentListType).toBeNull()
		})

		it("records the source wrapper bounds when dragging a wrapped node", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [
					{
						type: METRIC_GRID_NAME,
						content: [
							{
								type: METRIC_BLOCK_NAME,
								content: [{ type: "text", text: "m" }],
							},
						],
					},
				],
			})
			hover(editor, handle.plugin, 1)

			startDrag(element)

			expect(draggingOf(editor.view)?.sourceWrapperBounds).toEqual({
				start: 0,
				end: 5,
			})
		})

		it("records no wrapper bounds for a wrapped node outside its wrapper", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [
					{ type: METRIC_BLOCK_NAME, content: [{ type: "text", text: "m" }] },
				],
			})
			hover(editor, handle.plugin, 0)

			startDrag(element)

			expect(draggingOf(editor.view)?.sourceWrapperBounds).toBeNull()
		})

		it("drags the whole selection when the hovered node is part of it", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [paragraph("one"), paragraph("two")],
			})
			hover(editor, handle.plugin, 0)
			editor.commands.setTextSelection({ from: 1, to: 9 })

			startDrag(element)
			const dragging = draggingOf(editor.view)

			expect(dragging?.slice.content.childCount).toBe(2)
			expect(dragging?.slice.content.child(1).textContent).toBe("two")
			expect(editor.state.selection.from).toBe(0)
			expect(editor.state.selection.to).toBe(10)
		})

		it("drags the hovered node alone when the selection covers other nodes", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [paragraph("one"), paragraph("two"), paragraph("three")],
			})
			hover(editor, handle.plugin, 0)
			editor.commands.setTextSelection({ from: 6, to: 14 })

			startDrag(element)
			const dragging = draggingOf(editor.view)

			expect(dragging?.slice.content.childCount).toBe(1)
			expect(dragging?.slice.content.firstChild?.textContent).toBe("one")
		})

		it("removes the drag image once the drop lands", ({ expect }) => {
			const { editor, element, handle } = setup()
			hover(editor, handle.plugin, 0)
			const { dragImage } = startDrag(element)
			const image = dragImage()

			expect(image.parentElement).toBe(document.body)
			expect(image.style.top).toBe("-10000px")

			document.dispatchEvent(new Event("drop"))

			expect(image.parentElement).toBeNull()
		})

		it("copies the painted canvas pixels into the drag image", ({ expect }) => {
			const { editor, element, handle } = setup({
				content: [{ type: "canvasBlock" }],
			})
			const source = must(
				editor.view.dom.querySelector("canvas"),
				"a source canvas",
			)
			const sourceCtx = must(source.getContext("2d"), "a source context")
			sourceCtx.fillStyle = "#ff0000"
			sourceCtx.fillRect(0, 0, 20, 20)
			hover(editor, handle.plugin, 0)

			const { dragImage } = startDrag(element)
			const clone = must(dragImage().querySelector("canvas"), "a cloned canvas")
			const cloneCtx = must(clone.getContext("2d"), "a cloned context")

			expect(clone.getAttribute("data-origin")).toBeNull()
			expect(clone.width).toBe(20)
			expect(clone.style.cssText).not.toBe("")
			expect([...cloneCtx.getImageData(5, 5, 1, 1).data]).toEqual([
				255, 0, 0, 255,
			])
		})

		it("keeps the cloned canvas when its pixels cannot be copied", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [{ type: "canvasBlock" }],
			})
			vi.spyOn(
				CanvasRenderingContext2D.prototype,
				"drawImage",
			).mockImplementation(() => {
				throw new Error("tainted canvas")
			})
			hover(editor, handle.plugin, 0)

			const { dragImage } = startDrag(element)
			const clone = must(dragImage().querySelector("canvas"), "a cloned canvas")

			expect(clone.getAttribute("data-origin")).toBe("source")
			expect(clone.style.cssText).not.toBe("")
		})

		it("still replaces the canvas when no drawing context is available", ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [{ type: "canvasBlock" }],
			})
			vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
				() => null,
			)
			hover(editor, handle.plugin, 0)

			const { dragImage } = startDrag(element)
			const clone = must(dragImage().querySelector("canvas"), "a cloned canvas")

			expect(clone.getAttribute("data-origin")).toBeNull()
			expect(clone.width).toBe(20)
		})

		it("strips decorator widgets from the drag image", ({ expect }) => {
			const { editor, element, handle } = setup({
				content: [{ type: "canvasBlock" }],
			})
			hover(editor, handle.plugin, 0)

			const { dragImage } = startDrag(element)
			const image = dragImage()

			expect(
				editor.view.dom.querySelector(`[${CLONE_IGNORE_ATTR}]`),
			).not.toBeNull()
			expect(image.querySelector(`[${CLONE_IGNORE_ATTR}]`)).toBeNull()
		})

		it("copies the computed styles of every cloned element", ({ expect }) => {
			const { editor, element, handle } = setup()
			hover(editor, handle.plugin, 0)

			const { dragImage } = startDrag(element)
			const clone = must(
				dragImage().firstElementChild,
				"a cloned paragraph",
			) as HTMLElement

			expect(clone.tagName).toBe("P")
			expect(clone.style.cssText).toContain("display:")
		})
	})

	describe("dragend", () => {
		it("re-enables the handle and clears the broadcast drag", ({ expect }) => {
			const { provider, setLocalStateField } = fakeProvider()
			const { editor, element, handle, onElementDragEnd } = setup({
				content: [paragraph("one", { uid: "u1" })],
				provider,
			})
			hover(editor, handle.plugin, 0)
			startDrag(element)
			vi.advanceTimersByTime(1)

			const event = endDrag(element)

			expect(onElementDragEnd).toHaveBeenCalledWith(event)
			expect(disableGapZones).toHaveBeenCalledTimes(1)
			expect(setLocalStateField).toHaveBeenLastCalledWith(
				"draggingNodeUid",
				null,
			)
			expect(element.style.pointerEvents).toBe("auto")
		})

		it("leaves awareness alone when no drag was broadcast", ({ expect }) => {
			const { provider, setLocalStateField } = fakeProvider()
			const { editor, element, handle } = setup({
				content: [paragraph("one")],
				provider,
			})
			hover(editor, handle.plugin, 0)
			startDrag(element)

			endDrag(element)

			expect(setLocalStateField).toHaveBeenCalledTimes(0)
		})

		it("hides the handle when the drop leaves no node under the cursor", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue(null)

			endDrag(element)
			vi.advanceTimersByTime(20)

			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
		})

		it("re-targets the handle at the node under the cursor after the drop", async ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup({
				content: [paragraph("one"), paragraph("two")],
			})
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()
			vi.mocked(computePosition).mockClear()
			const node = must(editor.state.doc.nodeAt(5), "the second paragraph")
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue({
				node,
				pos: 5,
				depth: 2,
				dom: must(editor.view.nodeDOM(5), "its dom") as HTMLElement,
			})

			endDrag(element, 11, 13)
			vi.advanceTimersByTime(20)
			await flushMicrotasks()

			expect(findDraggableNodeAtCoords).toHaveBeenLastCalledWith(editor, 11, 13)
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node,
				pos: 5,
				depth: 2,
			})
			expect(computePosition).toHaveBeenCalledTimes(1)
			expect(element.style.visibility).toBe("")
			expect(element.style.pointerEvents).toBe("auto")
		})
	})

	describe("state", () => {
		it("locks the handle when a transaction carries the lock meta", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			editor.view.dispatch(editor.state.tr.setMeta("lockDragHandle", true))

			fireMouseMove(editor, handle.plugin, 5, 7)
			vi.advanceTimersByTime(20)

			expect(findDraggableNodeAtCoords).toHaveBeenCalledTimes(0)
			expect(onNodeChange).toHaveBeenCalledTimes(0)
			expect(element.style.visibility).toBe("hidden")
		})

		it("unlocks the handle when the lock meta turns off", ({ expect }) => {
			const { editor, handle, onNodeChange } = setup({ locked: true })

			editor.view.dispatch(editor.state.tr.setMeta("lockDragHandle", false))
			hover(editor, handle.plugin, 0)

			expect(onNodeChange).toHaveBeenCalledTimes(1)
		})

		it("hides the handle and forgets the node on the hide meta", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			editor.view.dispatch(editor.state.tr.setMeta("hideDragHandle", true))

			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: false,
			})
		})

		it("releases the lock when the handle is hidden", ({ expect }) => {
			const { editor, handle, onNodeChange } = setup({ locked: true })
			editor.view.dispatch(editor.state.tr.setMeta("hideDragHandle", true))
			onNodeChange.mockClear()

			hover(editor, handle.plugin, 0)

			expect(onNodeChange).toHaveBeenCalledTimes(1)
		})

		it("records a remote transaction while no node is tracked", ({
			expect,
		}) => {
			const { editor, handle } = setup()
			const tr = editor.state.tr.insertText("!", 1)
			tr.setMeta(ySyncPluginKey, { isChangeOrigin: true })

			editor.view.dispatch(tr)

			expect(handle.plugin.getState(editor.state)?.lastWasRemote).toBe(true)
		})

		it("hides the handle when a remote change cannot resolve the tracked node", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()
			const tr = editor.state.tr.insertText("!", 1)
			tr.setMeta(ySyncPluginKey, { isChangeOrigin: true })

			editor.view.dispatch(tr)

			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: true,
			})
		})

		it("hides the handle when a local change deletes the tracked node", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup({
				content: [paragraph("one"), paragraph("two"), paragraph("three")],
			})
			hover(editor, handle.plugin, 5)
			onNodeChange.mockClear()

			editor.view.dispatch(editor.state.tr.delete(5, 10))

			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
			expect(handle.plugin.getState(editor.state)).toEqual({
				depth: 0,
				lastWasRemote: false,
			})
		})

		it("follows the tracked node through a local change above it", async ({
			expect,
		}) => {
			const { editor, handle } = setup({
				content: [paragraph("one"), paragraph("two")],
			})
			hover(editor, handle.plugin, 5)
			const nodeDOM = vi.spyOn(editor.view, "nodeDOM")

			editor.view.dispatch(editor.state.tr.insertText("XY", 1))
			await flushMicrotasks()

			expect(nodeDOM).toHaveBeenCalledWith(7)
		})

		it("keeps the tracked position through a local change below it", async ({
			expect,
		}) => {
			const { editor, handle } = setup({
				content: [paragraph("one"), paragraph("two")],
			})
			hover(editor, handle.plugin, 0)
			const nodeDOM = vi.spyOn(editor.view, "nodeDOM")

			editor.view.dispatch(editor.state.tr.insertText("XY", 7))
			await flushMicrotasks()

			expect(nodeDOM).toHaveBeenCalledWith(0)
		})
	})

	describe("view", () => {
		it("repositions the handle after a local document change", async ({
			expect,
		}) => {
			const { editor, handle } = setup()
			hover(editor, handle.plugin, 0)
			vi.mocked(computePosition).mockClear()

			editor.view.dispatch(editor.state.tr.insertText("!", 2))
			await flushMicrotasks()

			expect(computePosition).toHaveBeenCalledTimes(1)
		})

		it("does not reposition while the handle is locked", async ({ expect }) => {
			const { editor, element, handle } = setup()
			hover(editor, handle.plugin, 0)
			editor.view.dispatch(editor.state.tr.setMeta("lockDragHandle", true))
			vi.mocked(computePosition).mockClear()

			editor.view.dispatch(editor.state.tr.insertText("!", 2))
			await flushMicrotasks()

			expect(computePosition).toHaveBeenCalledTimes(0)
			expect(element.draggable).toBe(false)
		})

		it("does not reposition when the document is unchanged", async ({
			expect,
		}) => {
			const { editor, handle } = setup()
			hover(editor, handle.plugin, 0)
			vi.mocked(computePosition).mockClear()

			editor.commands.setTextSelection(2)
			await flushMicrotasks()

			expect(computePosition).toHaveBeenCalledTimes(0)
		})

		it("does not reposition while no node is tracked", async ({ expect }) => {
			const { editor } = setup()
			vi.mocked(computePosition).mockClear()

			editor.view.dispatch(editor.state.tr.insertText("!", 2))
			await flushMicrotasks()

			expect(computePosition).toHaveBeenCalledTimes(0)
		})

		it("does not reposition when the tracked position holds a text node", async ({
			expect,
		}) => {
			const { editor, handle } = setup()
			hover(editor, handle.plugin, 1)
			vi.mocked(computePosition).mockClear()

			editor.view.dispatch(editor.state.tr.insertText("!", 2))
			await flushMicrotasks()

			expect(computePosition).toHaveBeenCalledTimes(0)
		})

		it("clears the broadcast drag and unmounts the wrapper when destroyed", ({
			expect,
		}) => {
			const { provider, setLocalStateField } = fakeProvider()
			const pluginKey = new PluginKey("dragHandleDestroy")
			const { editor, element, handle } = setup({
				content: [paragraph("one", { uid: "u1" })],
				provider,
				pluginKey,
			})
			hover(editor, handle.plugin, 0)
			startDrag(element)
			const wrapper = must(element.parentElement, "the handle wrapper")

			editor.unregisterPlugin(pluginKey)

			expect(setLocalStateField).toHaveBeenLastCalledWith(
				"draggingNodeUid",
				null,
			)
			expect(wrapper.parentElement).toBeNull()
		})

		it("drops a pending hover frame when destroyed", ({ expect }) => {
			const pluginKey = new PluginKey("dragHandleDestroyFrame")
			const { editor, handle } = setup({ pluginKey })
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue(null)
			fireMouseMove(editor, handle.plugin, 1, 1)

			editor.unregisterPlugin(pluginKey)
			vi.advanceTimersByTime(50)

			expect(findDraggableNodeAtCoords).toHaveBeenCalledTimes(0)
		})
	})

	describe("repositioning", () => {
		it("offsets the handle by the node type's configured offsets", async ({
			expect,
		}) => {
			const { editor, element, handle } = setup({
				content: [
					{
						type: "bulletList",
						content: [{ type: "listItem", content: [paragraph("item")] }],
					},
				],
			})

			hover(editor, handle.plugin, 1)
			await flushMicrotasks()

			expect(element.style.position).toBe("absolute")
			expect(element.style.left).toBe("75px")
			expect(element.style.top).toBe("199px")
		})

		it("measures the hovered node's dom by default", ({ expect }) => {
			const { editor, handle } = setup()
			hover(editor, handle.plugin, 0)
			const reference = must(
				vi.mocked(computePosition).mock.calls[0]?.[0],
				"a reference element",
			)
			const dom = must(
				editor.view.nodeDOM(0),
				"the paragraph dom",
			) as HTMLElement

			expect(reference.getBoundingClientRect().width).toBe(
				dom.getBoundingClientRect().width,
			)
			expect(reference.getBoundingClientRect().top).toBe(
				dom.getBoundingClientRect().top,
			)
		})

		it("measures the referenced virtual element when one is provided", ({
			expect,
		}) => {
			const rect = new DOMRect(1, 2, 3, 4)
			const virtualElement = { getBoundingClientRect: () => rect }
			const { editor, handle } = setup({
				getReferencedVirtualElement: () => virtualElement,
			})

			hover(editor, handle.plugin, 0)

			expect(vi.mocked(computePosition).mock.calls[0]?.[0]).toBe(virtualElement)
		})
	})

	describe("keydown", () => {
		it("hides the handle while the editor has focus", ({ expect }) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()
			vi.spyOn(editor.view, "hasFocus").mockReturnValue(true)

			const result = domHandler(handle.plugin, "keydown").call(
				handle.plugin,
				editor.view,
				new KeyboardEvent("keydown", { key: "a" }),
			)

			expect(result).toBe(false)
			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
		})

		it("leaves the handle visible while the editor is unfocused", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()
			vi.spyOn(editor.view, "hasFocus").mockReturnValue(false)

			const result = domHandler(handle.plugin, "keydown").call(
				handle.plugin,
				editor.view,
				new KeyboardEvent("keydown", { key: "a" }),
			)

			expect(result).toBe(false)
			expect(element.style.visibility).toBe("")
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})

		it("does nothing while the handle is locked", ({ expect }) => {
			const { editor, handle, onNodeChange } = setup({ locked: true })
			const hasFocus = vi.spyOn(editor.view, "hasFocus")

			const result = domHandler(handle.plugin, "keydown").call(
				handle.plugin,
				editor.view,
				new KeyboardEvent("keydown", { key: "a" }),
			)

			expect(result).toBe(false)
			expect(hasFocus).toHaveBeenCalledTimes(0)
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})
	})

	describe("mouseleave", () => {
		it("hides the handle when the pointer leaves for an outside element", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			editor.view.dom.dispatchEvent(
				new MouseEvent("mouseleave", { relatedTarget: document.body }),
			)

			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node: null,
				pos: -1,
				depth: 0,
			})
		})

		it("keeps the handle when the pointer moves onto the handle itself", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			editor.view.dom.dispatchEvent(
				new MouseEvent("mouseleave", { relatedTarget: element }),
			)

			expect(element.style.visibility).toBe("")
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})

		it("ignores a mouseleave that never reached an element", ({ expect }) => {
			const { editor, element, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			const result = domHandler(handle.plugin, "mouseleave").call(
				handle.plugin,
				editor.view,
				new MouseEvent("mouseleave", { relatedTarget: document.body }),
			)

			expect(result).toBe(false)
			expect(element.style.visibility).toBe("")
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})

		it("does nothing while the handle is locked", ({ expect }) => {
			const { editor, element, handle, onNodeChange } = setup({ locked: true })
			const result = domHandler(handle.plugin, "mouseleave").call(
				handle.plugin,
				editor.view,
				new MouseEvent("mouseleave", { relatedTarget: document.body }),
			)

			expect(result).toBe(false)
			expect(element.style.visibility).toBe("hidden")
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})
	})

	describe("mousemove", () => {
		it("shows and positions the handle for the hovered node", async ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			const node = hover(editor, handle.plugin, 0)
			await flushMicrotasks()

			expect(onNodeChange).toHaveBeenCalledWith({
				editor,
				node,
				pos: 0,
				depth: 1,
			})
			expect(element.style.visibility).toBe("")
			expect(element.style.pointerEvents).toBe("auto")
			expect(computePosition).toHaveBeenCalledTimes(1)
		})

		it("coalesces several moves into one detection", ({ expect }) => {
			const { editor, handle } = setup()
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue({
				node: must(editor.state.doc.nodeAt(0), "the paragraph"),
				pos: 0,
				depth: 1,
				dom: must(editor.view.nodeDOM(0), "its dom") as HTMLElement,
			})

			fireMouseMove(editor, handle.plugin, 1, 2)
			fireMouseMove(editor, handle.plugin, 3, 4)
			vi.advanceTimersByTime(20)

			expect(findDraggableNodeAtCoords).toHaveBeenCalledTimes(1)
			expect(findDraggableNodeAtCoords).toHaveBeenCalledWith(editor, 3, 4)
		})

		it("does nothing when no draggable node is under the cursor", ({
			expect,
		}) => {
			const { editor, element, handle, onNodeChange } = setup()
			vi.mocked(findDraggableNodeAtCoords).mockReturnValue(null)

			fireMouseMove(editor, handle.plugin, 1, 2)
			vi.advanceTimersByTime(20)

			expect(onNodeChange).toHaveBeenCalledTimes(0)
			expect(element.style.visibility).toBe("hidden")
		})

		it("does not re-notify while the pointer stays on the same node", ({
			expect,
		}) => {
			const { editor, handle, onNodeChange } = setup()
			hover(editor, handle.plugin, 0)

			hover(editor, handle.plugin, 0)

			expect(onNodeChange).toHaveBeenCalledTimes(1)
		})

		it("does nothing while the handle is locked", ({ expect }) => {
			const { editor, handle, onNodeChange } = setup({ locked: true })

			const result = fireMouseMove(editor, handle.plugin, 1, 2)
			vi.advanceTimersByTime(20)

			expect(result).toBe(false)
			expect(findDraggableNodeAtCoords).toHaveBeenCalledTimes(0)
			expect(onNodeChange).toHaveBeenCalledTimes(0)
		})
	})

	describe("when bound to a collaborative document", () => {
		function setupCollab(content: JSONContent[]) {
			const editor = mountEditor(content)
			const ydoc = prosemirrorJSONToYDoc(
				editor.schema,
				editor.getJSON(),
				"default",
			)
			const syncPlugin = ySyncPlugin(ydoc.getXmlFragment("default")) as Plugin
			editor.registerPlugin(syncPlugin)

			return {
				...setup({ editor, provider: fakeProvider().provider }),
				ydoc,
			}
		}

		// a change authored by another peer: a foreign transaction origin
		// is what makes the sync plugin treat it as remote
		function insertRemoteParagraph(ydoc: Y.Doc) {
			ydoc.transact(() => {
				const inserted = new Y.XmlElement("paragraph")
				inserted.insert(0, [new Y.XmlText("zero")])
				ydoc.getXmlFragment("default").insert(0, [inserted])
			}, "remote-peer")
		}

		function appendRemoteParagraph(ydoc: Y.Doc) {
			ydoc.transact(() => {
				const inserted = new Y.XmlElement("paragraph")
				inserted.insert(0, [new Y.XmlText("last")])
				const fragment = ydoc.getXmlFragment("default")
				fragment.insert(fragment.length, [inserted])
			}, "remote-peer")
		}

		function posOfParagraph(editor: Editor, text: string): number {
			let found = -1

			editor.state.doc.forEach((node, offset) => {
				if (node.textContent === text) {
					found = offset
				}
			})

			return found
		}

		it("follows the tracked node through a remote insertion", async ({
			expect,
		}) => {
			const { editor, handle, ydoc } = setupCollab([
				paragraph("one"),
				paragraph("two"),
			])
			hover(editor, handle.plugin, 5)
			vi.mocked(computePosition).mockClear()

			insertRemoteParagraph(ydoc)
			await flushMicrotasks()

			expect(handle.plugin.getState(editor.state)?.lastWasRemote).toBe(true)
			expect(computePosition).toHaveBeenCalledTimes(0)

			const nodeDOM = vi.spyOn(editor.view, "nodeDOM")
			editor.view.dispatch(editor.state.tr.insertText("!", 1))
			await flushMicrotasks()

			expect(nodeDOM).toHaveBeenCalledWith(posOfParagraph(editor, "two"))
		})

		it("keeps the tracked position when a remote change lands below it", async ({
			expect,
		}) => {
			const { editor, handle, ydoc } = setupCollab([
				paragraph("one"),
				paragraph("two"),
			])
			hover(editor, handle.plugin, 5)

			appendRemoteParagraph(ydoc)
			await flushMicrotasks()

			const nodeDOM = vi.spyOn(editor.view, "nodeDOM")
			editor.view.dispatch(
				editor.state.tr.insertText("!", editor.state.doc.content.size - 1),
			)
			await flushMicrotasks()

			expect(nodeDOM).toHaveBeenCalledWith(5)
		})

		it("keeps the handle on the first node through a remote change below it", ({
			expect,
		}) => {
			const { editor, element, handle, ydoc, onNodeChange } = setupCollab([
				paragraph("one"),
				paragraph("two"),
			])
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			appendRemoteParagraph(ydoc)

			expect(element.style.visibility).toBe("")
			expect(onNodeChange).not.toHaveBeenCalled()
			expect(handle.plugin.getState(editor.state)?.lastWasRemote).toBe(true)
		})

		// yjs maps the very start of the document to the fragment start
		// rather than to the node that happens to sit there, so a peer
		// inserting above it leaves the tracked position untouched
		it("keeps the handle at the document start when a remote insertion lands above it", ({
			expect,
		}) => {
			const { editor, element, handle, ydoc, onNodeChange } = setupCollab([
				paragraph("one"),
				paragraph("two"),
			])
			hover(editor, handle.plugin, 0)
			onNodeChange.mockClear()

			insertRemoteParagraph(ydoc)

			expect(element.style.visibility).toBe("")
			expect(onNodeChange).not.toHaveBeenCalled()
		})

		it("cancels a drag that never produced a drag image", ({ expect }) => {
			const { editor, element, handle, ydoc, onDragCancel, onElementDragEnd } =
				setupCollab([
					paragraph("one", { uid: "u1" }),
					paragraph("two", { uid: "u2" }),
				])
			hover(editor, handle.plugin, 5)
			startDrag(element, false)

			insertRemoteParagraph(ydoc)

			expect(editor.view.dragging).toBeNull()
			expect(onDragCancel).toHaveBeenCalledTimes(1)
			expect(onElementDragEnd).toHaveBeenCalledTimes(1)
		})

		it("cancels an in-flight drag when a remote change moves the node", ({
			expect,
		}) => {
			const { editor, element, handle, ydoc, onDragCancel, onElementDragEnd } =
				setupCollab([
					paragraph("one", { uid: "u1" }),
					paragraph("two", { uid: "u2" }),
				])
			hover(editor, handle.plugin, 5)
			startDrag(element)
			vi.mocked(disableGapZones).mockClear()

			insertRemoteParagraph(ydoc)

			expect(editor.view.dragging).toBeNull()
			expect(onDragCancel).toHaveBeenCalledTimes(1)
			expect(disableGapZones).toHaveBeenCalled()
			expect(onElementDragEnd).toHaveBeenCalledTimes(1)
		})
	})
})
