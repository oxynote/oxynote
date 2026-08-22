import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { defineComponent, h } from "vue"
import SidebarItem from "./SidebarItem.vue"
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	type SidebarItem as Item,
} from "./sidebar"
import {
	at,
	menuItem,
	mountUnderSidebarProvider,
	renderedIconNames,
	t,
} from "./test-helpers"

interface CapturedDragOptions {
	disabled?: MaybeRefOrGetter<boolean>
	onStart?: () => void
	onMove?: (pos: { x: number; y: number }) => void
	onEnd?: () => void
}

// happy-dom has no layout, so the vueuse pointer/geometry primitives the
// drag rides on are stubbed and driven directly — the same treatment
// app/composables/useSidebarDraggable.nuxt.test.ts gives them one layer
// down. Everything between them and the component stays real.
const {
	capturedDrags,
	capturedMice,
	useDraggableMock,
	useMouseInElementMock,
	useElementBoundingMock,
} = vi.hoisted(() => {
	const capturedDrags: CapturedDragOptions[] = []
	const capturedMice: { isOutside: Ref<boolean> }[] = []

	return {
		capturedDrags,
		capturedMice,
		useDraggableMock: vi.fn((_el: unknown, opts: CapturedDragOptions): any => {
			capturedDrags.push(opts)

			return { x: ref(0), y: ref(0) }
		}),
		useMouseInElementMock: vi.fn((): any => {
			// real refs: the composable watches these, and a plain object
			// would leave every watcher and computed behind them inert
			const mouse = { isOutside: ref(true) }
			capturedMice.push(mouse)

			return mouse
		}),
		useElementBoundingMock: vi.fn((): any => ({
			width: ref(0),
			height: ref(0),
		})),
	}
})

mockNuxtImport("useDraggable", () => useDraggableMock)
mockNuxtImport("useMouseInElement", () => useMouseInElementMock)
mockNuxtImport("useElementBounding", () => useElementBoundingMock)

function makeItem(overrides: Partial<Item> = {}): Item {
	return {
		id: "doc-1",
		name: "Runbook",
		url: "/acme/runbook-doc-1",
		partOfDocumentTree: true,
		icon: "lucide:file",
		active: false,
		draggable: true,
		children: [],
		...overrides,
	}
}

async function mountItem(item: Item, props: Record<string, unknown> = {}) {
	const wrapper = await mountUnderSidebarProvider(SidebarItem, {
		props: { parentId: null, item: item, wrapper: null, ...props },
	})

	return { wrapper, item: wrapper.findComponent(SidebarItem) }
}

// the dropdown body is teleported into the shared <body>, and the geometry
// mocks and the drag store behind them are module-level singletons every
// mount in the file shares
describe("<SidebarItem>", { concurrent: false }, () => {
	beforeEach(() => {
		// rows mounted by an earlier test can outlive it, and a pointer left
		// over one of them keeps its watchers writing to the app-wide drag
		// store — park every pointer before the next mount starts
		capturedMice.forEach((mouse) => {
			mouse.isOutside.value = true
		})
		capturedDrags.length = 0
		capturedMice.length = 0
	})

	// a drag left in flight keeps its dragged row and drop target in the
	// app-wide store, where the next test's rows would read it
	afterEach(() => {
		capturedDrags.forEach((drag) => drag.onEnd?.())
	})

	it("links a normal item at its url", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem())

		expect(wrapper.get("a").attributes("href")).toBe("/acme/runbook-doc-1")
		expect(wrapper.text()).toContain("Runbook")
	})

	it("renders an item with no url as a plain clickable row", async ({
		expect,
	}) => {
		const { wrapper } = await mountItem(makeItem({ url: null }))

		expect(wrapper.find("a").exists()).toBe(false)
		expect(wrapper.text()).toContain("Runbook")
	})

	it("runs an urlless item's own click handler", async ({ expect }) => {
		const onClick = vi.fn()
		const { wrapper } = await mountItem(makeItem({ url: null, onClick }))

		await wrapper.get("[data-slot='sidebar-menu-button']").trigger("click")

		expect(onClick).toHaveBeenCalledTimes(1)
	})

	it("blanks the href of an optimistically inserted item", async ({
		expect,
	}) => {
		const { wrapper } = await mountItem(
			makeItem({ localOptimisticInsert: true }),
		)

		expect(wrapper.get("a").attributes("href")).toBeUndefined()
		expect(wrapper.get("a").attributes("data-optimistic-insert")).toBe("")
	})

	it("shows the item's icon", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem({ icon: "lucide:book" }))

		expect(renderedIconNames(wrapper)).toContain("lucide:book")
	})

	it("shows the unread count when the item carries one", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem({ url: null, count: 3 }))

		expect(wrapper.get("[data-slot='sidebar-menu-button']").text()).toBe(
			"Runbook3",
		)
	})

	it("omits the count badge when the count is zero", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem({ url: null, count: 0 }))

		expect(wrapper.get("[data-slot='sidebar-menu-button']").text()).toBe(
			"Runbook",
		)
	})

	it("marks the active item", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem(), { active: true })

		expect(
			wrapper
				.get("[data-slot='sidebar-menu-button']")
				.attributes("data-active"),
		).toBe("true")
	})

	it("asks to toggle the collapse when the chevron is pressed", async ({
		expect,
	}) => {
		const { wrapper, item } = await mountItem(makeItem())

		await at(wrapper.findAll("[data-sidebar='menu-action']"), 1).trigger(
			"click",
		)

		expect(item.emitted("toggle-collapse")).toHaveLength(1)
	})

	it("turns the chevron once the item is open", async ({ expect }) => {
		const { wrapper } = await mountItem(makeItem(), { open: true })

		expect(
			at(wrapper.findAll("[data-sidebar='menu-action']"), 1)
				.get(".iconify")
				.classes(),
		).toContain("rotate-90")
	})

	it("asks to create a sub page when the placeholder row is pressed", async ({
		expect,
	}) => {
		const { wrapper, item } = await mountItem(
			makeItem({ id: SIDEBAR_ITEM_PLACEHOLDER_ID, url: null, icon: null }),
		)

		await wrapper.get("[data-slot='sidebar-menu-button']").trigger("click")

		expect(item.emitted("create")).toHaveLength(1)
	})

	it("styles the placeholder row as a placeholder", async ({ expect }) => {
		const { wrapper } = await mountItem(
			makeItem({ id: SIDEBAR_ITEM_PLACEHOLDER_ID, url: null, icon: null }),
		)

		expect(
			wrapper
				.get("[data-slot='sidebar-menu-button']")
				.attributes("data-placeholder-variant"),
		).toBe("true")
		expect(renderedIconNames(wrapper)).toContain("lucide:plus")
	})

	it("hides the options menu on the placeholder row", async ({ expect }) => {
		const { wrapper } = await mountItem(
			makeItem({ id: SIDEBAR_ITEM_PLACEHOLDER_ID, url: null, icon: null }),
		)

		expect(wrapper.find("[data-slot='dropdown-menu-trigger']").exists()).toBe(
			false,
		)
	})

	it("hides the options menu on items outside the document tree", async ({
		expect,
	}) => {
		const { wrapper } = await mountItem(
			makeItem({ partOfDocumentTree: false, url: null }),
		)

		expect(wrapper.find("[data-slot='dropdown-menu-trigger']").exists()).toBe(
			false,
		)
	})

	it.for([
		{
			label: "sidebar.item-dropdown-menu-buttons.duplicate-page",
			event: "duplicate",
		},
		{
			label: "sidebar.item-dropdown-menu-buttons.add-sub-page",
			event: "create",
		},
		{
			label: "sidebar.item-dropdown-menu-buttons.delete-page",
			event: "delete",
		},
	] as const)(
		"asks to $event the page from the options menu",
		async ({ label, event }, { expect }) => {
			const { wrapper, item } = await mountItem(makeItem())
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem(t(label)).click()
			await nextTick()

			expect(item.emitted(event)).toHaveLength(1)
		},
	)

	it("does not duplicate itself as a drag ghost while at rest", async ({
		expect,
	}) => {
		const { wrapper } = await mountItem(makeItem())

		expect(wrapper.findAllComponents(SidebarItem)).toHaveLength(1)
	})

	it.for([
		{ draggable: true, expected: false },
		{ draggable: false, expected: true },
	])(
		"leaves dragging $expected for an item with draggable $draggable",
		async ({ draggable, expected }, { expect }) => {
			await mountItem(makeItem({ draggable: draggable }))

			expect(toValue(at(capturedDrags, 0).disabled)).toBe(expected)
		},
	)

	it("keeps its ghost hidden until the drag passes the threshold", async ({
		expect,
	}) => {
		await mountDraggableItem()

		nudge(0)
		await nextTick()

		expect(document.body.querySelector("[data-ghost]")).toBeNull()
		endDrag()
	})

	it("shows a ghost of itself once the drag passes the threshold", async ({
		expect,
	}) => {
		await mountDraggableItem()

		await startDrag()
		await nextTick()

		expect(document.body.querySelector("[data-ghost]")).not.toBeNull()
		endDrag()
	})

	it("drops its ghost again once the drag ends", async ({ expect }) => {
		await mountDraggableItem()
		await startDrag()
		await nextTick()

		endDrag()
		await nextTick()

		expect(document.body.querySelector("[data-ghost]")).toBeNull()
	})

	it("asks to be moved into the row it was dropped on", async ({ expect }) => {
		const dragged = await mountPair()
		await startDrag()

		await hover(TARGET_ROW)
		endDrag()

		expect(dragged.emitted("update-location")).toEqual([
			[{ parentId: "b", insertBeforeId: null }],
		])
	})

	it("asks to be moved above the row whose top edge it was dropped on", async ({
		expect,
	}) => {
		const dragged = await mountPair()
		await startDrag()

		await hover(TARGET_ROW_TOP_EDGE)
		endDrag()

		expect(dragged.emitted("update-location")).toEqual([
			[{ parentId: "root", insertBeforeId: "b" }],
		])
	})

	it("asks for nothing when the drag ends away from every row", async ({
		expect,
	}) => {
		const dragged = await mountPair()
		await startDrag()

		endDrag()

		expect(dragged.emitted("update-location")).toBeUndefined()
	})
})

// the composable asks for the item's mouse first and its top edge second,
// so a two-row mount fills the captured list in this order
const DRAGGED_ROW = 0
const TARGET_ROW = 2
const TARGET_ROW_TOP_EDGE = 3

function mountDraggableItem() {
	// a real wrapper element: the drag store records it, and descendant
	// checks against it decide which rows may be dropped on
	return mountItem(makeItem({ id: "a" }), {
		wrapper: document.createElement("div"),
	})
}

// two rows under one provider — a drop target has to be a different row,
// and rows coordinate through an app-wide store
async function mountPair() {
	// the props are built once: a fresh item object per render would give
	// both rows a new identity on every store change and take the captured
	// pointer stubs down with them
	const rows = [
		{
			key: "a",
			parentId: null,
			item: makeItem({ id: "a" }),
			wrapper: document.createElement("div"),
		},
		{
			key: "b",
			parentId: "root",
			item: makeItem({ id: "b" }),
			wrapper: document.createElement("div"),
		},
	]
	const Pair = defineComponent({
		setup() {
			return () => rows.map((row) => h(SidebarItem, row))
		},
	})
	const wrapper = await mountUnderSidebarProvider(Pair, {})

	return at(wrapper.findAllComponents(SidebarItem), DRAGGED_ROW)
}

function nudge(distance: number) {
	const drag = at(capturedDrags, DRAGGED_ROW)
	drag.onStart?.()
	drag.onMove?.({ x: 0, y: 0 })
	drag.onMove?.({ x: 0, y: distance })
}

// the composable only treats a pointer move as a drag past minDistance,
// and the row it started from has to register as dragged before another
// row can become a target
async function startDrag() {
	nudge(100)
	await nextTick()
}

function endDrag() {
	at(capturedDrags, DRAGGED_ROW).onEnd?.()
}

async function hover(mouseIndex: number) {
	at(capturedMice, mouseIndex).isOutside.value = false
	await nextTick()
}
