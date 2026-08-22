import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import IconPickerProvider from "./IconPickerProvider.vue"
import IconPicker from "./IconPicker.vue"
import { mountUnderTooltipProvider, t } from "~/components/test-helpers"

// the real list is virtualized off the rendered row heights, which
// happy-dom reports as zero — nothing would ever be drawn. The stand-in
// renders every row the picker hands it.
vi.mock("vue-virtual-scroller", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		RecycleScroller: defineComponent({
			name: "RecycleScroller",
			props: { items: { type: Array, required: true } },
			setup:
				(props, { slots }) =>
				() =>
					h(
						"div",
						(props.items as { id: string }[]).map((item) =>
							slots.default?.({ item }),
						),
					),
		}),
	}
})

// the provider installs the icon-picker context that its children reach
// through useIconPicker, so an IconPicker inside the slot is what opens
// it. Each icon in the list carries a tooltip, whose context the app
// installs once at page level.
function mountProvider() {
	return mountUnderTooltipProvider(IconPickerProvider, {
		slots: {
			default: () => h(IconPicker, { icon: "mingcute:tag-fill" }),
		},
	})
}

// the picker positions itself with an async floating-ui call, so a test
// that leaves it half-open trips over the teardown
async function openPicker(wrapper: Awaited<ReturnType<typeof mountProvider>>) {
	await wrapper.get("button").trigger("click")
	await flushPromises()
}

function popover(): HTMLElement | null {
	return document.body.querySelector(".z-dropdown")
}

function popoverOpen(): boolean {
	return popover()?.classList.contains("opacity-100") ?? false
}

function filterInput(): HTMLInputElement | null {
	return popover()?.querySelector("input") ?? null
}

async function typeFilter(value: string) {
	const input = filterInput()
	if (!input) {
		throw new Error("the icon picker is not open")
	}

	input.value = value
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await nextTick()
}

// the picker body is teleported into a shared <body> and vue keeps
// patching it while the provider is mounted, so these tests cannot
// interleave and each provider is taken down properly
describe("<IconPickerProvider>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("renders its children", async ({ expect }) => {
		const wrapper = await mountProvider()

		expect(wrapper.findComponent(IconPicker).exists()).toBe(true)
	})

	it("keeps the picker out of the way until it is asked for", async ({
		expect,
	}) => {
		await mountProvider()

		expect(popoverOpen()).toBe(false)
	})

	it("opens the picker for the child that asked", async ({ expect }) => {
		const wrapper = await mountProvider()

		await openPicker(wrapper)

		expect(popoverOpen()).toBe(true)
	})

	it("offers a filter over the icon list", async ({ expect }) => {
		const wrapper = await mountProvider()

		await openPicker(wrapper)

		expect(filterInput()?.placeholder).toBe(
			t("editor.icon-select-menu.input-placeholder"),
		)
	})

	it("says so when nothing matches the filter", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		await typeFilter("definitely-not-an-icon")

		expect(popover()?.textContent).toContain(
			t("editor.icon-select-menu.no-results"),
		)
	})

	it("keeps only the icons matching the filter", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)
		const total = popover()?.querySelectorAll("button").length ?? 0

		await typeFilter("tag")

		const filtered = popover()?.querySelectorAll("button").length ?? 0

		expect(filtered).toBeGreaterThan(0)
		expect(filtered).toBeLessThan(total)
	})

	it("hands the chosen icon to the child that asked", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)
		await typeFilter("tag 2")

		popover()?.querySelectorAll("button")[0]?.click()
		await nextTick()

		expect(wrapper.findComponent(IconPicker).emitted("select")).toEqual([
			["mingcute:tag-2-fill"],
		])
		expect(popoverOpen()).toBe(false)
	})

	it("closes the picker on escape", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		expect(popoverOpen()).toBe(false)
	})

	it("stays open on any other key", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		document.dispatchEvent(new KeyboardEvent("keydown", { key: "a" }))
		await nextTick()

		expect(popoverOpen()).toBe(true)
	})

	it("closes the picker on a click outside it", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popoverOpen()).toBe(false)
	})

	it("stays open while clicking inside it", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		popover()?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popoverOpen()).toBe(true)
	})

	it("stays open while clicking the trigger that opened it", async ({
		expect,
	}) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)

		wrapper
			.get("button")
			.element.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popoverOpen()).toBe(true)
	})

	it("clears the filter each time it opens", async ({ expect }) => {
		const wrapper = await mountProvider()
		await openPicker(wrapper)
		await typeFilter("tag")
		document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		await openPicker(wrapper)

		expect(filterInput()?.value).toBe("")
	})
})
