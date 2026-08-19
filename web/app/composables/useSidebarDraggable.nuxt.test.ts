import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { effectScope, nextTick, ref } from "vue"
import type { EffectScope, Ref } from "vue"
import type { SidebarItem } from "~/components/sidebar"
import { useSidebarDraggable } from "./useSidebarDraggable"

interface CapturedDragOptions {
	onStart?: () => void
	onMove?: (pos: { x: number; y: number }) => void
	onEnd?: () => void
}

// the vueuse pointer/geometry primitives are stubbed out: happy-dom has no
// real layout, so the drag logic is driven through the captured callbacks
// and controllable refs instead of synthetic pointer events
const { useDraggableMock, useMouseInElementMock, useElementBoundingMock } =
	vi.hoisted(() => {
		return {
			useDraggableMock: vi.fn(
				(_el: unknown, _opts: CapturedDragOptions): any => ({
					x: { value: 0 },
					y: { value: 0 },
				}),
			),
			useMouseInElementMock: vi.fn((): any => ({
				isOutside: { value: true },
			})),
			useElementBoundingMock: vi.fn((): any => ({
				width: { value: 0 },
				height: { value: 0 },
			})),
		}
	})

mockNuxtImport("useDraggable", () => useDraggableMock)
mockNuxtImport("useMouseInElement", () => useMouseInElementMock)
mockNuxtImport("useElementBounding", () => useElementBoundingMock)

function sidebarItem(
	id: string,
	parentId: string | null,
): SidebarItem & { parentId: string | null } {
	return {
		id,
		name: id,
		partOfDocumentTree: true,
		active: false,
		draggable: true,
		children: [],
		parentId,
	}
}

function makeInstance(
	id: string,
	opts?: {
		parentId?: string | null
		insideWrapperOf?: { wrapper: HTMLElement }
	},
) {
	const dragOptions: CapturedDragOptions = {}
	const mouse: { isOutside: Ref<boolean> } = { isOutside: ref(true) }
	const topEdgeMouse: { isOutside: Ref<boolean> } = { isOutside: ref(true) }

	useDraggableMock.mockImplementationOnce(
		(_el: unknown, dragOpts: CapturedDragOptions) => {
			Object.assign(dragOptions, dragOpts)

			return { x: ref(11), y: ref(22) }
		},
	)
	// the composable asks for the item mouse first and the top edge second
	useMouseInElementMock
		.mockImplementationOnce(() => mouse)
		.mockImplementationOnce(() => topEdgeMouse)
	useElementBoundingMock.mockImplementationOnce(() => ({
		width: ref(100),
		height: ref(30),
	}))

	const elem = document.createElement("div")
	const wrapper = document.createElement("div")
	wrapper.appendChild(elem)

	// nesting the element inside another item's wrapper models a child item
	// in the sidebar tree
	if (opts?.insideWrapperOf) {
		opts.insideWrapperOf.wrapper.appendChild(elem)
	}

	const onConfirm = vi.fn()

	// each instance lives in its own effect scope so its watchers can be
	// disposed after the test — a leftover watcher would otherwise react to
	// the next test's drag through the shared store
	const scope = effectScope()
	activeScopes.push(scope)
	let api!: ReturnType<typeof useSidebarDraggable>
	scope.run(() => {
		api = useSidebarDraggable(
			elem,
			() => sidebarItem(id, opts?.parentId ?? null),
			document.createElement("div"),
			() => wrapper,
			onConfirm,
			{ minDistance: 5 },
		)
	})

	return { api, dragOptions, mouse, topEdgeMouse, onConfirm, wrapper }
}

const activeScopes: EffectScope[] = []

function startDrag(instance: ReturnType<typeof makeInstance>) {
	instance.dragOptions.onStart?.()
	instance.dragOptions.onMove?.({ x: 0, y: 0 })
	instance.dragOptions.onMove?.({ x: 0, y: 100 })
}

// the instances coordinate through a shared pinia store and shared
// module-level mocks (mockNuxtImport singletons), so the tests cannot
// interleave — each test ends the drag it starts
describe("useSidebarDraggable", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		useDraggableMock.mockReset()
		useMouseInElementMock.mockReset()
		useElementBoundingMock.mockReset()
	})

	afterEach(() => {
		activeScopes.forEach((scope) => {
			scope.stop()
		})
		activeScopes.length = 0
	})

	it("starts inactive", ({ expect }) => {
		const item = makeInstance("a")

		expect(item.api.isDraggingGlobal.value).toBe(false)
		expect(item.api.isDraggingSelf.value).toBe(false)
		expect(item.api.draggedGhostStyle.value).toEqual({})
	})

	it("activates the drag after the minimum distance", ({ expect }) => {
		const item = makeInstance("a")

		startDrag(item)

		expect(item.api.isDraggingGlobal.value).toBe(true)
		expect(item.api.isDraggingSelf.value).toBe(true)
		expect(item.api.draggedGhostStyle.value).toEqual({
			top: "22px",
			left: "11px",
			width: "100px",
			height: "30px",
		})

		item.dragOptions.onEnd?.()

		expect(item.api.isDraggingGlobal.value).toBe(false)
	})

	it("ignores movement below the minimum distance", ({ expect }) => {
		const item = makeInstance("a")

		item.dragOptions.onMove?.({ x: 0, y: 0 })
		item.dragOptions.onMove?.({ x: 0, y: 3 })

		expect(item.api.isDraggingGlobal.value).toBe(false)
	})

	it("confirms a drop into the hovered item", async ({ expect }) => {
		const dragged = makeInstance("a")
		const target = makeInstance("b", { parentId: "root" })
		startDrag(dragged)

		target.mouse.isOutside.value = false
		await nextTick()

		expect(target.api.isItemDraggedOn.value).toBe(true)

		dragged.dragOptions.onEnd?.()

		expect(dragged.onConfirm).toHaveBeenCalledExactlyOnceWith({
			targetParentId: "b",
			targetInsertBeforeId: null,
		})
		expect(dragged.api.isDraggingGlobal.value).toBe(false)
	})

	it("confirms a drop before the item when its top edge is hovered", async ({
		expect,
	}) => {
		const dragged = makeInstance("a")
		const target = makeInstance("b", { parentId: "root" })
		startDrag(dragged)

		target.topEdgeMouse.isOutside.value = false
		await nextTick()

		expect(target.api.isTopEdgeDraggedOn.value).toBe(true)
		expect(target.api.isItemDraggedOn.value).toBe(false)

		dragged.dragOptions.onEnd?.()

		expect(dragged.onConfirm).toHaveBeenCalledExactlyOnceWith({
			targetParentId: "root",
			targetInsertBeforeId: "b",
		})
	})

	it("does not target the dragged item itself", async ({ expect }) => {
		const item = makeInstance("a")
		startDrag(item)

		item.mouse.isOutside.value = false
		await nextTick()

		expect(item.api.isItemDraggedOn.value).toBe(false)

		item.dragOptions.onEnd?.()

		expect(item.onConfirm).toHaveBeenCalledTimes(0)
	})

	it("does not target a descendant of the dragged item", async ({ expect }) => {
		const dragged = makeInstance("a")
		const child = makeInstance("a-child", {
			parentId: "a",
			insideWrapperOf: dragged,
		})
		startDrag(dragged)

		child.mouse.isOutside.value = false
		await nextTick()

		expect(child.api.isItemDraggedOn.value).toBe(false)

		dragged.dragOptions.onEnd?.()

		expect(dragged.onConfirm).toHaveBeenCalledTimes(0)
	})

	it("ends without a confirm when nothing was targeted", ({ expect }) => {
		const item = makeInstance("a")
		startDrag(item)

		item.dragOptions.onEnd?.()

		expect(item.onConfirm).toHaveBeenCalledTimes(0)
		expect(item.api.isDraggingGlobal.value).toBe(false)
	})
})
