import { beforeEach, describe, it } from "vitest"
import SidebarTagVisibility from "./SidebarTagVisibility.vue"
import {
	clearTeleportedOverlays,
	menuItem,
	mountUnderSidebarProvider,
	t,
} from "./test-helpers"

function makeTag(id: string, tagName: string, hidden: boolean, color: string) {
	return { id: id, tagName: tagName, color: color, hidden: hidden }
}

function mountControl(tags: ReturnType<typeof makeTag>[]) {
	return mountUnderSidebarProvider(SidebarTagVisibility, { props: { tags } })
}

async function openControl(tags: ReturnType<typeof makeTag>[]) {
	const wrapper = await mountControl(tags)
	await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

	return { wrapper, control: wrapper.findComponent(SidebarTagVisibility) }
}

// the menu is teleported into the shared <body> and reka-ui reuses its
// generated ids across mounts, so a leftover would answer the next lookup
describe("<SidebarTagVisibility>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("names the control for screen readers", async ({ expect }) => {
		const wrapper = await mountControl([])

		expect(wrapper.text()).toContain(
			t("sidebar.sections.tags.visibility-action-title"),
		)
	})

	it("lists every tag, hidden ones included", async ({ expect }) => {
		await openControl([
			makeTag("a", "Production", false, "green"),
			makeTag("b", "Staging", true, "orange"),
		])

		const rows = Array.from(
			document.body.querySelectorAll("[role^='menuitem']"),
		).map((el) => el.textContent.trim())

		expect(rows).toEqual(["Production", "Staging"])
	})

	it("shows each tag's own colour beside its name", async ({ expect }) => {
		await openControl([
			makeTag("a", "Production", false, "green"),
			makeTag("b", "Staging", true, "orange"),
		])

		const dots = Array.from(
			document.body.querySelectorAll<HTMLElement>(
				"[role^='menuitem'] span[style]",
			),
		).map((el) => el.getAttribute("style"))

		expect(dots).toEqual([
			expect.stringContaining("var(--selectable-color-6)"),
			expect.stringContaining("var(--selectable-color-2)"),
		])
	})

	it("ticks a visible tag and leaves a hidden one unticked", async ({
		expect,
	}) => {
		await openControl([
			makeTag("a", "Production", false, "green"),
			makeTag("b", "Staging", true, "orange"),
		])

		// the tick is the menu item's own active marker, which renders after
		// the row's content rather than in a gutter before it
		const ticks = Array.from(
			document.body.querySelectorAll("[role^='menuitem']"),
		).map((el) =>
			Array.from(el.querySelectorAll(".iconify")).some((icon) =>
				icon.classList.contains("i-lucide:check"),
			),
		)

		expect(ticks).toEqual([true, false])
	})

	it("leaves every row usable, including the only visible tag", async ({
		expect,
	}) => {
		await openControl([
			makeTag("a", "Production", false, "green"),
			makeTag("b", "Staging", true, "orange"),
		])

		const states = Array.from(
			document.body.querySelectorAll("[role^='menuitem']"),
		).map((el) => el.getAttribute("data-disabled"))

		expect(states).toEqual([null, null])
	})

	it("reports the tag whose row was pressed", async ({ expect }) => {
		const hidden = makeTag("b", "Staging", true, "orange")
		const { control } = await openControl([
			makeTag("a", "Production", false, "green"),
			hidden,
		])

		menuItem("Staging").click()
		await nextTick()

		expect(control.emitted("toggle")).toEqual([[hidden]])
	})

	it("keeps the menu open so several tags can be toggled", async ({
		expect,
	}) => {
		await openControl([
			makeTag("a", "Production", false, "green"),
			makeTag("b", "Staging", true, "orange"),
		])

		menuItem("Staging").click()
		await nextTick()

		expect(
			document.body.querySelectorAll("[role^='menuitem']").length,
		).toBeGreaterThan(0)
	})
})
