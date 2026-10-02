import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Editor } from "@tiptap/core"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import BlockHookDiffMarkers from "./BlockHookDiffMarkers.vue"
import HookDiffMarker from "./HookDiffMarker.vue"
import { makeHook } from "./test-helpers"

const ROOT_RECT = new DOMRect(0, 0, 800, 600)

interface FakeBlock {
	uid: string
	rect: DOMRect
	type?: string
}

// the root's own rect is the frame every marker is placed in, and happy-dom
// lays nothing out, so it is fixed here
describe("<BlockHookDiffMarkers>", { concurrent: false }, () => {
	beforeEach(() => {
		vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
			ROOT_RECT,
		)
	})

	it("places a marker by each block whose hooks changed", async ({
		expect,
	}) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
			{ uid: "block-2", rect: new DOMRect(50, 160, 600, 40) },
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [
				urlHook("block-1", "https://new.test"),
				urlHook("block-2", "https://same.test"),
			],
			targetBranchHooks: [
				urlHook("block-1", "https://old.test"),
				urlHook("block-2", "https://same.test"),
			],
		})
		const markers = wrapper.findAllComponents(HookDiffMarker)

		expect(markers).toHaveLength(1)
		expect(markers[0]?.props()).toEqual({
			count: { removed: 1, added: 1 },
			hookStatus: "fresh",
			lifted: false,
		})
		// a paragraph's handle sits one pixel above the block's top
		expect(markers[0]?.attributes("style")).toBe("top: 99px; right: 750px;")
		// the gap to the block centres the marker under the drag dots
		expect(markers[0]?.classes()).toEqual(
			expect.arrayContaining(["pr-0.5", "lg:pr-1"]),
		)
	})

	it("leaves the page's own hooks to the title", async ({ expect }) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook(null, "https://new.test")],
		})

		expect(wrapper.findAllComponents(HookDiffMarker)).toHaveLength(0)
	})

	it("skips a block the document does not hold", async ({ expect }) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook("block-gone", "https://new.test")],
		})

		expect(wrapper.findAllComponents(HookDiffMarker)).toHaveLength(0)
	})

	it("lifts the marker of the block the handle shows on", async ({
		expect,
	}) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook("block-1", "https://new.test")],
			hoveredBlockId: "block-1",
		})

		expect(wrapper.getComponent(HookDiffMarker).props("lifted")).toBe(true)
	})

	it.for([
		{ name: "stale for a triggered hook", score: "0", expected: "stale" },
		{ name: "neutral for removed hooks alone", score: null, expected: null },
	])("colours the hook icon $name", async ({ score, expected }, { expect }) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
		])
		const triggered = { ...urlHook("block-1", "https://new.test"), score: "0" }

		const wrapper = await mountMarkers(
			editor,
			score === null
				? { targetBranchHooks: [urlHook("block-1", "https://old.test")] }
				: { activeBranchHooks: [triggered] },
		)

		expect(wrapper.getComponent(HookDiffMarker).props("hookStatus")).toBe(
			expected,
		)
	})

	it("centres the marker on a block whose handle is centred", async ({
		expect,
	}) => {
		const { editor } = fakeEditor([
			{
				uid: "block-1",
				rect: new DOMRect(50, 100, 600, 40),
				type: "horizontalRule",
			},
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook("block-1", "https://new.test")],
		})

		// the middle of the block less half of a 1.375rem handle at 16px
		expect(wrapper.getComponent(HookDiffMarker).attributes("style")).toBe(
			"top: 109px; right: 750px;",
		)
	})

	it("places the marker on whole pixels", async ({ expect }) => {
		const { editor } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50.4, 100.6, 600, 40) },
		])

		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook("block-1", "https://new.test")],
		})

		expect(wrapper.getComponent(HookDiffMarker).attributes("style")).toBe(
			"top: 100px; right: 750px;",
		)
	})

	it("follows its block when the document changes", async ({ expect }) => {
		const { editor, blocks, emitTransaction } = fakeEditor([
			{ uid: "block-1", rect: new DOMRect(50, 100, 600, 40) },
		])
		const wrapper = await mountMarkers(editor, {
			activeBranchHooks: [urlHook("block-1", "https://new.test")],
		})

		blocks.set("block-1", new DOMRect(50, 300, 600, 40))
		emitTransaction()
		await nextTick()

		expect(wrapper.getComponent(HookDiffMarker).attributes("style")).toBe(
			"top: 299px; right: 750px;",
		)
	})

	it("stops following the document once unmounted", async ({ expect }) => {
		const { editor, off } = fakeEditor([])
		const wrapper = await mountMarkers(editor, {})

		wrapper.unmount()

		expect(off).toHaveBeenCalledTimes(1)
		expect(off).toHaveBeenCalledWith("transaction", expect.any(Function))
	})
})

function urlHook(blockId: string | null, url: string): DocumentHook {
	return makeHook({
		id: `${blockId ?? "page"}-${url}`,
		blockId: blockId,
		settings: { url: url },
	})
}

// fakeEditor stands in for the diff editor: its blocks sit at fixed rects
// that a test can move, and a transaction is emitted by hand
function fakeEditor(fakeBlocks: FakeBlock[]) {
	const blocks = new Map(fakeBlocks.map((block) => [block.uid, block.rect]))
	const nodes = fakeBlocks.map((block, index) => ({
		pos: index * 10,
		node: {
			attrs: { uid: block.uid },
			type: { name: block.type ?? "paragraph" },
		},
	}))
	const handlers: (() => void)[] = []
	const off = vi.fn()

	const editor = {
		on: (_event: string, handler: () => void) => {
			handlers.push(handler)
		},
		off: off,
		state: {
			doc: {
				descendants: (fn: (node: unknown, pos: number) => void) => {
					nodes.forEach((entry) => {
						fn(entry.node, entry.pos)
					})
				},
				nodeAt: (pos: number) =>
					nodes.find((entry) => entry.pos === pos)?.node ?? null,
			},
		},
		view: {
			dom: document.createElement("div"),
			nodeDOM: (pos: number) => {
				const uid = nodes.find((entry) => entry.pos === pos)?.node.attrs.uid
				const rect = uid ? blocks.get(uid) : undefined
				if (!rect) {
					return null
				}

				const el = document.createElement("div")
				el.getBoundingClientRect = () => rect

				return el
			},
		},
	} as unknown as Editor

	return {
		editor,
		blocks,
		off,
		emitTransaction: () => {
			handlers.forEach((handler) => {
				handler()
			})
		},
	}
}

function mountMarkers(
	editor: Editor,
	props: Record<string, unknown>,
): Promise<VueWrapper> {
	return mountSuspended(BlockHookDiffMarkers, {
		props: {
			editor: editor,
			activeBranchHooks: [],
			targetBranchHooks: [],
			hoveredBlockId: null,
			...props,
		},
	})
}
