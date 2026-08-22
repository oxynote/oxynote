import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import BlockHandle from "./BlockHandle.vue"
import { DragHandle } from "./MainElement"
import CoreMenu from "./menu-options/CoreMenu.vue"
import { makeEditor } from "../test-helpers/node-view"
import {
	clearTeleportedOverlays,
	menuItem,
	renderedIconNames,
	stubViewportMatches,
	t,
} from "~/components/test-helpers"

const MENU_CLOSE_ANIMATION_MS = 150

function hook(blockId: string, score: number): DocumentHook {
	return { blockId: blockId, score: score } as unknown as DocumentHook
}

// the editor the handle reads the hovered node through
function makeHandleEditor(
	node: Record<string, unknown> | null = {
		attrs: { uid: "block-1" },
		type: { name: "paragraph" },
		nodeSize: 4,
		toJSON: () => ({ type: "paragraph", attrs: { uid: "block-1" } }),
	},
) {
	const nodeElement = document.createElement("div")

	return makeEditor({
		registerPlugin: vi.fn(),
		unregisterPlugin: vi.fn(),
		commands: {
			setMeta: vi.fn(),
			hasNodeComment: () => false,
			focus: vi.fn(),
		},
		state: {
			doc: {
				nodeAt: () => node,
				resolve: () => ({
					depth: 0,
					node: () => ({ type: { name: "doc" } }),
				}),
			},
			tr: { delete: () => ({ doc: { forEach: () => undefined } }) },
			schema: { nodes: {} },
		},
		view: { nodeDOM: () => nodeElement, dispatch: vi.fn() },
	})
}

function mountHandle(props: Record<string, unknown> = {}) {
	return mountSuspended(BlockHandle, {
		props: { editor: makeHandleEditor().editor, ...props },
	})
}

// the drag handle reports the block the pointer is over through the
// callback it was handed, not through a dom event
function hoverNode(
	wrapper: VueWrapper,
	pos: number,
	node: unknown = { type: { name: "paragraph" } },
) {
	const onNodeChange = wrapper
		.findComponent(DragHandle)
		.props("onNodeChange") as (data: {
		pos: number
		node: PMNode | null
	}) => void

	onNodeChange({ pos: pos, node: node as PMNode | null })
}

function handleRoot(wrapper: VueWrapper) {
	return wrapper.get(".z-drag-handle")
}

function dragOverlays(): NodeListOf<Element> {
	return document.body.querySelectorAll("[aria-hidden='true'].z-editor-overlay")
}

function grabbingCursor(): boolean {
	return document.documentElement.classList.contains("cursor-grabbing!")
}

async function openBlockMenu(wrapper: VueWrapper) {
	const triggers = wrapper.findAll("[data-slot='dropdown-menu-trigger']")
	const trigger = triggers[triggers.length - 1]
	await trigger?.trigger("pointerdown", { button: 0 })
	await trigger?.trigger("click")
	await nextTick()
}

// the editable flag is a shared cookie state, the editor store, the
// viewport stub, the document element's classes and the teleported menu
// bodies are all shared, so these tests cannot interleave
describe("<BlockHandle>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		stubViewportMatches(true)
		useEditorMeta().setEditable(true)
		useEditorMeta().updateLock(false)
		useEditorStore().setReviewableDiffActive(false)
		document.documentElement.className = ""
	})

	it("offers a hook handle and a block handle on a wide screen", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()

		expect(wrapper.findAll("[data-slot='dropdown-menu-trigger']")).toHaveLength(
			2,
		)
		expect(wrapper.text()).toContain(t("editor.hook-handle.screen-reader-hint"))
		expect(wrapper.text()).toContain(t("editor.drag-handle.screen-reader-hint"))
	})

	it("folds the hook handle away on a narrow screen", async ({ expect }) => {
		stubViewportMatches(false)

		const wrapper = await mountHandle()

		expect(wrapper.findAll("[data-slot='dropdown-menu-trigger']")).toHaveLength(
			1,
		)
	})

	it("offers no hook handle in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountHandle()

		expect(wrapper.findAll("[data-slot='dropdown-menu-trigger']")).toHaveLength(
			1,
		)
	})

	it("leaves the hook icon unmarked for a block with no hooks", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()

		expect(
			wrapper.get(".i-mingcute\\:leaf-line").attributes("data-hook-status"),
		).toBeUndefined()
	})

	it("marks the hook icon fresh for a block with healthy hooks", async ({
		expect,
	}) => {
		const wrapper = await mountHandle({
			documentHooks: [hook("block-1", 1)],
		})
		hoverNode(wrapper, 10)
		await nextTick()

		expect(
			wrapper.get(".i-mingcute\\:leaf-line").attributes("data-hook-status"),
		).toBe("fresh")
	})

	it("marks the hook icon stale when a hook has run out", async ({
		expect,
	}) => {
		const wrapper = await mountHandle({
			documentHooks: [hook("block-1", 1), hook("block-1", 0)],
		})
		hoverNode(wrapper, 10)
		await nextTick()

		expect(
			wrapper.get(".i-mingcute\\:leaf-line").attributes("data-hook-status"),
		).toBe("stale")
	})

	it("ignores hooks belonging to other blocks", async ({ expect }) => {
		const wrapper = await mountHandle({
			documentHooks: [hook("block-2", 0)],
		})
		hoverNode(wrapper, 10)
		await nextTick()

		expect(
			wrapper.get(".i-mingcute\\:leaf-line").attributes("data-hook-status"),
		).toBeUndefined()
	})

	it("hands the hovered block to the block menu", async ({ expect }) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()

		await openBlockMenu(wrapper)

		expect(wrapper.findComponent(CoreMenu).props("hovered")).toEqual(
			expect.objectContaining({ nodePos: 10, nodeId: "block-1" }),
		)
	})

	it("forgets the hovered block when the pointer leaves the document", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()
		await openBlockMenu(wrapper)

		hoverNode(wrapper, 10, null)
		await nextTick()

		expect(wrapper.findComponent(CoreMenu).props("hovered")).toBeNull()
	})

	it("forgets a block that is no longer in the document", async ({
		expect,
	}) => {
		const wrapper = await mountHandle({
			editor: makeHandleEditor(null).editor,
		})
		hoverNode(wrapper, 10)
		await nextTick()

		await openBlockMenu(wrapper)

		expect(wrapper.findComponent(CoreMenu).props("hovered")).toBeNull()
	})

	it("highlights the block while the pointer is on the handle", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()

		await handleRoot(wrapper).trigger("mouseenter")

		expect(dragOverlays()).toHaveLength(1)
	})

	it("drops the highlight when the pointer leaves the handle", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()
		await handleRoot(wrapper).trigger("mouseenter")

		await handleRoot(wrapper).trigger("mouseleave")

		expect(dragOverlays()).toHaveLength(0)
	})

	it("highlights nothing while no block is hovered", async ({ expect }) => {
		const wrapper = await mountHandle()

		await handleRoot(wrapper).trigger("mouseenter")

		expect(dragOverlays()).toHaveLength(0)
	})

	it("locks the editor while the block menu is open", async ({ expect }) => {
		const wrapper = await mountHandle()

		await openBlockMenu(wrapper)

		expect(useEditorMeta().isLocked.value).toBe(true)
	})

	it("unlocks the editor once the menu has finished closing", async ({
		expect,
	}) => {
		const wrapper = await mountHandle()
		await openBlockMenu(wrapper)
		vi.useFakeTimers()

		await openBlockMenu(wrapper)
		await vi.advanceTimersByTimeAsync(MENU_CLOSE_ANIMATION_MS)

		expect(useEditorMeta().isLocked.value).toBe(false)
	})

	it("keeps the block menu's own actions reachable", async ({ expect }) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()

		await openBlockMenu(wrapper)

		expect(
			menuItem(t("editor.drag-handle.options.default.duplicate-block")),
		).toBeDefined()
	})

	it("folds the hook options into the block menu on a narrow screen", async ({
		expect,
	}) => {
		stubViewportMatches(false)
		const wrapper = await mountHandle()

		await openBlockMenu(wrapper)

		expect(document.body.textContent).toContain(t("editor.hooks.add-new"))
	})

	it("shows a grabbing cursor while a block is dragged", async ({ expect }) => {
		const wrapper = await mountHandle()

		await handleRoot(wrapper).trigger("dragstart")

		expect(grabbingCursor()).toBe(true)
	})

	it("puts the cursor back once the drag ends", async ({ expect }) => {
		const wrapper = await mountHandle()
		await handleRoot(wrapper).trigger("dragstart")

		await handleRoot(wrapper).trigger("dragend")

		expect(grabbingCursor()).toBe(false)
	})

	it("refuses to start a drag in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountHandle()
		const event = new Event("dragstart", { cancelable: true, bubbles: true })

		handleRoot(wrapper).element.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(true)
		expect(grabbingCursor()).toBe(false)
	})

	it("passes a comment request from the block menu on", async ({ expect }) => {
		const wrapper = await mountHandle()
		hoverNode(wrapper, 10)
		await nextTick()
		await openBlockMenu(wrapper)

		menuItem(t("editor.drag-handle.options.add-node-comment")).click()
		await nextTick()

		expect(wrapper.emitted("add-node-comment")).toEqual([[10]])
	})

	it("shows the icons for both handles", async ({ expect }) => {
		const wrapper = await mountHandle()

		expect(renderedIconNames(wrapper)).toEqual([
			"mingcute:leaf-line",
			"mingcute:dots-line",
		])
	})
})
