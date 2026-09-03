import { beforeEach, describe, it } from "vitest"
import SidebarItem from "./SidebarItem.vue"
import SidebarNestedGroup from "./SidebarNestedGroup.vue"
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	type SidebarItem as Item,
} from "./sidebar"
import {
	emitFrom,
	emitFromNth,
	mountUnderSidebarProvider,
	seedPersistentState,
} from "./test-helpers"

function makeItem(id: string, overrides: Partial<Item> = {}): Item {
	return {
		id: id,
		name: id,
		url: `/acme/${id}`,
		acceptsChildren: true,
		icon: "lucide:file",
		active: false,
		draggable: true,
		actions: [],
		children: [],
		...overrides,
	}
}

function placeholder(): Item {
	return {
		id: SIDEBAR_ITEM_PLACEHOLDER_ID,
		name: "Add a page",
		url: null,
		acceptsChildren: false,
		icon: null,
		active: false,
		draggable: false,
		actions: [],
		children: [],
	}
}

async function mountGroup(items: Item[], itemId: string | null = null) {
	const wrapper = await mountUnderSidebarProvider(SidebarNestedGroup, {
		props: { modelValue: items, itemId: itemId },
	})

	return { wrapper, group: wrapper.findComponent(SidebarNestedGroup) }
}

function rowIds(wrapper: Awaited<ReturnType<typeof mountGroup>>["wrapper"]) {
	return wrapper
		.findAll("[data-item-id]")
		.map((el) => el.attributes("data-item-id"))
}

// the collapse state is a persisted singleton every mount in the file
// shares, and the options menus are teleported into the shared <body>
describe("<SidebarNestedGroup>", { concurrent: false }, () => {
	beforeEach(() => {
		seedPersistentState("sidebar-item-collapse", {})
	})

	it("renders nothing for an empty group", async ({ expect }) => {
		const { wrapper } = await mountGroup([])

		expect(rowIds(wrapper)).toEqual([])
	})

	it("renders one row per item, in order", async ({ expect }) => {
		const { wrapper } = await mountGroup([makeItem("a"), makeItem("b")])

		expect(rowIds(wrapper)).toEqual(["a", "b"])
	})

	it("records how many children each row has", async ({ expect }) => {
		const { wrapper } = await mountGroup([
			makeItem("a", { children: [makeItem("a1")] }),
		])

		expect(
			wrapper.get("[data-item-id='a']").attributes("data-item-children"),
		).toBe("1")
	})

	it("renders a nested group for an item's children", async ({ expect }) => {
		const { wrapper } = await mountGroup([
			makeItem("a", { children: [makeItem("a1")] }),
		])

		expect(rowIds(wrapper)).toEqual(["a", "a1"])
	})

	it("opens top-level items that have real children", async ({ expect }) => {
		const { wrapper } = await mountGroup([
			makeItem("a", { children: [makeItem("a1")] }),
		])

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(true)
	})

	it("leaves a top-level item whose only child is the placeholder closed", async ({
		expect,
	}) => {
		const { wrapper } = await mountGroup([
			makeItem("a", { children: [placeholder()] }),
		])

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(false)
	})

	it("leaves a childless top-level item closed", async ({ expect }) => {
		const { wrapper } = await mountGroup([makeItem("a", { children: [] })])

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(false)
	})

	it("leaves nested levels closed", async ({ expect }) => {
		const { wrapper } = await mountGroup(
			[makeItem("a", { children: [makeItem("a1")] })],
			"parent",
		)

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(false)
	})

	it("opens a closed row when its collapse toggle fires", async ({
		expect,
	}) => {
		const { wrapper } = await mountGroup([makeItem("a", { children: [] })])
		emitFrom(wrapper, "SidebarItem", "toggle-collapse")
		await nextTick()

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(true)
	})

	it("closes an open row when its collapse toggle fires again", async ({
		expect,
	}) => {
		const { wrapper } = await mountGroup([
			makeItem("a", { children: [makeItem("a1")] }),
		])
		emitFrom(wrapper, "SidebarItem", "toggle-collapse")
		await nextTick()

		expect(wrapper.findComponent(SidebarItem).props("open")).toBe(false)
	})

	it("makes a creation from a normal row a child of that row", async ({
		expect,
	}) => {
		const { wrapper, group } = await mountGroup([makeItem("a")])

		emitFrom(wrapper, "SidebarItem", "create")
		await nextTick()

		expect(group.emitted("create")).toEqual([[{ parentId: "a" }]])
	})

	it("makes a creation from a placeholder row a sibling in the same group", async ({
		expect,
	}) => {
		const { wrapper, group } = await mountGroup([placeholder()], "parent")

		emitFrom(wrapper, "SidebarItem", "create")
		await nextTick()

		expect(group.emitted("create")).toEqual([[{ parentId: "parent" }]])
	})

	it("reports a move with the row's id and the requested destination", async ({
		expect,
	}) => {
		const { wrapper, group } = await mountGroup([makeItem("a")])

		emitFrom(wrapper, "SidebarItem", "update-location", {
			parentId: "b",
			insertBeforeId: "c",
		})
		await nextTick()

		expect(group.emitted("update-location")).toEqual([
			[{ id: "a", parentId: "b", insertBeforeId: "c" }],
		])
	})

	it("passes a nested group's move straight through", async ({ expect }) => {
		const { wrapper, group } = await mountGroup([
			makeItem("a", { children: [makeItem("a1")] }),
		])
		emitFromNth(wrapper, "SidebarNestedGroup", 1, "update-location", {
			id: "a1",
			parentId: null,
			insertBeforeId: null,
		})
		await nextTick()

		expect(group.emitted("update-location")).toEqual([
			[{ id: "a1", parentId: null, insertBeforeId: null }],
		])
	})
})
