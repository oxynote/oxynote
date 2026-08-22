import { beforeEach, describe, it } from "vitest"
import AppSidebarHeader from "./AppSidebarHeader.vue"
import {
	clearTeleportedOverlays,
	menuItem,
	mountUnderSidebarProvider,
	t,
} from "./test-helpers"

const AVATAR = { src: "", alt: "Logo", initials: "AC" }

function mountHeader(workspaceName: string | null) {
	return mountUnderSidebarProvider(AppSidebarHeader, {
		props: { workspaceName: workspaceName, avatar: AVATAR },
	})
}

// the dropdown body is teleported into the shared <body>, and the colour
// theme lives in an app-wide persistent state every mount in the file sees
describe("<AppSidebarHeader>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		useAppearance().changeColorTheme("auto")
	})

	it("shows the workspace name", async ({ expect }) => {
		const wrapper = await mountHeader("Acme Corp")

		expect(wrapper.text()).toContain("Acme Corp")
	})

	it("falls back to a placeholder name when the workspace has none", async ({
		expect,
	}) => {
		const wrapper = await mountHeader(null)

		expect(wrapper.text()).toContain(t("sidebar.default-workspace-name"))
	})

	it("shows the workspace initials while the logo has no source", async ({
		expect,
	}) => {
		const wrapper = await mountHeader("Acme Corp")

		expect(wrapper.text()).toContain("AC")
	})

	it("asks for a new document when the compose action is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountHeader("Acme Corp")
		const header = wrapper.findComponent(AppSidebarHeader)

		await header.get("[data-sidebar='menu-action']").trigger("click")

		expect(header.emitted("create-new-item")).toHaveLength(1)
	})

	it("keeps the menu closed until the workspace button is pressed", async ({
		expect,
	}) => {
		await mountHeader("Acme Corp")

		expect(
			document.body.querySelector("[data-slot='dropdown-menu-content']"),
		).toBeNull()
	})

	it("opens the workspace menu when the button is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountHeader("Acme Corp")

		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		expect(
			document.body.querySelector("[data-slot='dropdown-menu-content']"),
		).not.toBeNull()
	})

	it("asks to open settings from the menu", async ({ expect }) => {
		const wrapper = await mountHeader("Acme Corp")
		const header = wrapper.findComponent(AppSidebarHeader)
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(t("sidebar.header.settings")).click()
		await nextTick()

		expect(header.emitted("open-settings")).toHaveLength(1)
	})

	it("asks to log out from the menu", async ({ expect }) => {
		const wrapper = await mountHeader("Acme Corp")
		const header = wrapper.findComponent(AppSidebarHeader)
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(t("sidebar.header.log-out")).click()
		await nextTick()

		expect(header.emitted("log-out")).toHaveLength(1)
	})

	it.for([
		{ label: "sidebar.header.appearance.light", expected: "light" },
		{ label: "sidebar.header.appearance.dark", expected: "dark" },
	] as const)(
		"switches the colour theme to $expected from the menu",
		async ({ label, expected }, { expect }) => {
			const wrapper = await mountHeader("Acme Corp")
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
			await openAppearanceSubMenu()

			menuItem(t(label)).click()
			await nextTick()

			expect(useAppearance().color.value.system).toBe(false)
			expect(useAppearance().color.value.theme).toBe(expected)
		},
	)

	it("switches the colour theme back to the system setting", async ({
		expect,
	}) => {
		useAppearance().changeColorTheme("dark")
		const wrapper = await mountHeader("Acme Corp")
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
		await openAppearanceSubMenu()

		menuItem(t("sidebar.header.appearance.auto")).click()
		await nextTick()

		expect(useAppearance().color.value.system).toBe(true)
	})
})

async function openAppearanceSubMenu() {
	menuItem(t("sidebar.header.appearance.title")).click()
	await nextTick()
	await nextTick()
}
