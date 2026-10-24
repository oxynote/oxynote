import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import BottomAction from "./BottomAction.vue"
import {
	clearTeleportedOverlays,
	findButtonByText,
	menuItem,
	mountUnderTooltipProvider,
	renderedIconNames,
	t,
} from "~/components/test-helpers"

const SHORTCUT = {
	keyboardKey: { macOS: "⌘+K", other: "Ctrl+K" },
	i18nKey: null,
}

const PARAMS_KEY = "editor.split-documentation.left-side-bottom-action-button"
const CODE_KEY =
	"editor.split-documentation.right-side-bottom-action-buttons.code"
const METRICS_KEY =
	"editor.split-documentation.right-side-bottom-action-buttons.metrics"
const TRIGGER_KEY = "editor.split-documentation.bottom-action-trigger-label"

enableAutoUnmount(afterEach)

// the menu is teleported into the shared <body> and the host os lives in
// a nuxt useState singleton, so these tests cannot interleave
describe("<SplitDocumentationBottomAction>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("labels a button with its text", async ({ expect }) => {
		const wrapper = await mountAction([{ id: "params", text: t(PARAMS_KEY) }])

		expect(wrapper.text()).toContain(t(PARAMS_KEY))
	})

	it("shows a plus icon by default", async ({ expect }) => {
		const wrapper = await mountAction([{ id: "params", text: t(PARAMS_KEY) }])

		expect(renderedIconNames(wrapper)).toEqual(["lucide:circle-plus"])
	})

	it("shows the given icon", async ({ expect }) => {
		const wrapper = await mountAction([
			{ id: "params", text: t(PARAMS_KEY), icon: "lucide:list-plus" },
		])

		expect(renderedIconNames(wrapper)).toEqual(["lucide:list-plus"])
	})

	it("reports a click with the button's id", async ({ expect }) => {
		const wrapper = await mountAction([{ id: "params", text: t(PARAMS_KEY) }])

		await findButtonByText(wrapper, t(PARAMS_KEY)).trigger("click")

		expect(clicks(wrapper)).toEqual(["params"])
	})

	it("shows a single closed menu trigger for two or more entries", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), icon: "lucide:code" },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		expect(wrapper.findAll("button")).toHaveLength(1)
		expect(wrapper.text()).toBe(t(TRIGGER_KEY))
		expect(renderedIconNames(wrapper)).toEqual(["mingcute:add-circle-line"])
		expect(menuLabels()).toEqual([])
	})

	it("lists every entry with its icon once the menu is opened", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), icon: "lucide:code" },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await openMenu(wrapper)

		expect(menuLabels()).toEqual([t(CODE_KEY), t(METRICS_KEY)])
		expect(menuIconNames()).toEqual(["lucide:code", "lucide:circle-plus"])
	})

	// the suite runs as a non-mac host, so an entry prints the keys of the
	// "other" variant, one <kbd> each
	it("spells out the shortcut of each menu entry that has one", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), shortcut: SHORTCUT },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await openMenu(wrapper)

		expect(shortcutKeys(t(CODE_KEY))).toEqual(["Ctrl", "K"])
		expect(shortcutKeys(t(METRICS_KEY))).toEqual([])
	})

	it("writes the connector only between the keys of a sequence", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{
				id: "code",
				text: t(CODE_KEY),
				shortcut: {
					keyboardKey: { macOS: "G Shift+I", other: "G Shift+I" },
					i18nKey: null,
				},
			},
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await openMenu(wrapper)

		expect(
			menuItem(t(CODE_KEY)).querySelector("kbd[data-slot='kbd-group']")
				?.textContent,
		).toBe(`G${t("shortcuts.connector")}ShiftI`)
	})

	it("reports a click on a later menu entry with its own id", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY) },
			{ id: "metrics", text: t(METRICS_KEY) },
		])
		await openMenu(wrapper)

		menuItem(t(METRICS_KEY)).click()
		await nextTick()

		expect(clicks(wrapper)).toEqual(["metrics"])
	})

	it("wraps a button carrying a shortcut in a tooltip trigger", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "params", text: t(PARAMS_KEY), shortcut: SHORTCUT },
		])

		expect(wrapper.findAll("[data-slot='tooltip-trigger']")).toHaveLength(1)
	})

	it("swallows a mousedown on the overlay background itself", async ({
		expect,
	}) => {
		const wrapper = await mountAction([{ id: "params", text: t(PARAMS_KEY) }])
		const event = new MouseEvent("mousedown", {
			bubbles: true,
			cancelable: true,
		})

		wrapper.get("div").element.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(true)
	})

	it("lets a mousedown on the button through", async ({ expect }) => {
		const wrapper = await mountAction([{ id: "params", text: t(PARAMS_KEY) }])
		const event = new MouseEvent("mousedown", {
			bubbles: true,
			cancelable: true,
		})

		findButtonByText(wrapper, t(PARAMS_KEY)).element.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(false)
	})
})

function mountAction(buttons: Record<string, unknown>[]) {
	return mountUnderTooltipProvider(BottomAction, { props: { buttons } })
}

// the emitted button-click payloads, in the order they were emitted.
// findComponent cannot type a generic component, so the wrapper is
// asserted
function clicks(wrapper: VueWrapper): unknown[] {
	const action = wrapper.findComponent(BottomAction) as unknown as VueWrapper

	return (action.emitted("button-click") ?? []).map((args) => args[0])
}

function openMenu(wrapper: VueWrapper) {
	return wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
}

// the open menu is teleported into <body>, so its entries cannot be read
// through the wrapper
function menuEntries(): HTMLElement[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role='menuitem']"),
	)
}

function menuLabels(): string[] {
	return menuEntries().map((entry) => entry.textContent.trim())
}

function menuIconNames(): string[] {
	return menuEntries().flatMap((entry) => {
		const cls = Array.from(
			entry.querySelector(".iconify")?.classList ?? [],
		).find((c) => c.startsWith("i-"))

		return cls ? [cls.slice("i-".length)] : []
	})
}

function shortcutKeys(label: string): string[] {
	return Array.from(
		menuItem(label).querySelectorAll("kbd[data-slot='kbd-group'] kbd"),
	).map((key) => key.textContent)
}
