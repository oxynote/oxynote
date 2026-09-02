import { beforeEach, describe, it, vi } from "vitest"
import NotificationSidebar from "./NotificationSidebar.vue"
import {
	emitFrom,
	mountUnderSidebarProvider,
	seedPersistentState,
} from "./test-helpers"

// matchMedia is what useMediaQuery reads; happy-dom's own implementation
// always reports "not matching", so the mobile layout needs a stub
function stubViewport(matches: boolean) {
	vi.stubGlobal(
		"matchMedia",
		vi.fn((query: string) => ({
			matches: matches,
			media: query,
			onchange: null,
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			dispatchEvent: vi.fn(),
		})),
	)
}

function mountSidebar(open: boolean) {
	return mountUnderSidebarProvider(NotificationSidebar, {
		props: { modelValue: open },
		// the notification box owns its own queries and websocket
		// subscription; this component only positions it
		stubs: { NotificationBox: true },
	})
}

// the positioned panel is the box's own wrapper element
function panel(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	const element = wrapper.get("notification-box-stub").element.parentElement
	if (!element) {
		throw new Error("the notification box is not inside a panel")
	}

	return element
}

// the viewport stub is a global shared by every mount in the file
describe("<NotificationSidebar>", { concurrent: false }, () => {
	beforeEach(() => {
		// the sidebar provider reads its open state from this persisted
		// singleton, which every mount in the file shares
		seedPersistentState("sidebar-state", true)
	})

	it("collapses the column while the inbox is closed", async ({ expect }) => {
		stubViewport(false)

		const wrapper = await mountSidebar(false)

		expect(wrapper.html()).toContain("grid-cols-[0rem]")
	})

	it("opens the column when the inbox is open", async ({ expect }) => {
		stubViewport(false)

		const wrapper = await mountSidebar(true)

		expect(wrapper.html()).toContain("grid-cols-[20rem]")
	})

	it("slides the panel out of view while the inbox is closed", async ({
		expect,
	}) => {
		stubViewport(false)

		const wrapper = await mountSidebar(false)

		expect(panel(wrapper).className).toContain("-translate-x-full")
	})

	it("parks the panel against the left edge while the inbox is closed", async ({
		expect,
	}) => {
		stubViewport(false)

		const wrapper = await mountSidebar(false)

		expect(panel(wrapper).getAttribute("style")).toContain("left: 0px")
	})

	it("offsets the open panel by the sidebar's width", async ({ expect }) => {
		stubViewport(false)

		const wrapper = await mountSidebar(true)

		expect(panel(wrapper).getAttribute("style")).toContain("left: 224px")
	})

	it("keeps the inbox box mounted on wide viewports", async ({ expect }) => {
		stubViewport(false)

		const wrapper = await mountSidebar(false)

		expect(wrapper.find("notification-box-stub").exists()).toBe(true)
	})

	it("leaves the sheet closed on narrow viewports while the inbox is closed", async ({
		expect,
	}) => {
		stubViewport(true)

		await mountSidebar(false)

		expect(
			document.body.querySelector("[data-slot='sheet-content']"),
		).toBeNull()
	})

	it("opens a sheet on narrow viewports when the inbox is open", async ({
		expect,
	}) => {
		stubViewport(true)

		await mountSidebar(true)

		expect(
			document.body.querySelector("[data-slot='sheet-content']"),
		).not.toBeNull()
	})

	it("keeps the panel against the left edge while the sidebar is collapsed", async ({
		expect,
	}) => {
		stubViewport(false)
		seedPersistentState("sidebar-state", false)

		const wrapper = await mountSidebar(true)

		expect(panel(wrapper).getAttribute("style")).toContain("left: 0px")
	})

	it("closes the inbox when the sheet is dismissed", async ({ expect }) => {
		stubViewport(true)
		const wrapper = await mountSidebar(true)

		emitFrom(wrapper, "Sheet", "update:open", false)
		await nextTick()

		expect(
			wrapper.findComponent(NotificationSidebar).emitted("update:modelValue"),
		).toEqual([[false]])
	})

	it("closes the inbox when the box asks to be closed", async ({ expect }) => {
		stubViewport(true)
		const wrapper = await mountSidebar(true)

		emitFrom(wrapper, "NotificationBox", "close-notification-box")
		await nextTick()

		expect(
			wrapper.findComponent(NotificationSidebar).emitted("update:modelValue"),
		).toEqual([[false]])
	})

	it("passes a navigation from the inbox box on to its parent", async ({
		expect,
	}) => {
		stubViewport(false)
		const wrapper = await mountSidebar(true)

		emitFrom(wrapper, "NotificationBox", "notification-navigation")
		await nextTick()

		expect(
			wrapper
				.findComponent(NotificationSidebar)
				.emitted("notification-navigation"),
		).toHaveLength(1)
	})
})
