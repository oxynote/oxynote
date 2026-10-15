import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
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
	"editor.split-documentation.right-side-bottom-action-buttons.add-code"
const METRICS_KEY =
	"editor.split-documentation.right-side-bottom-action-buttons.add-metrics"
const TRIGGER_KEY = "editor.split-documentation.bottom-action-trigger-label"

function openDropdown(wrapper: VueWrapper) {
	return wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
}

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

	it("shows a single trigger for two or more entries", async ({ expect }) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), icon: "lucide:code" },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		expect(wrapper.findAll("button")).toHaveLength(1)
		expect(wrapper.text()).toContain(t(TRIGGER_KEY))
	})

	it("lists every entry once the trigger is opened", async ({ expect }) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), icon: "lucide:code" },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await openDropdown(wrapper)

		expect(menuItem(t(CODE_KEY))).not.toBeNull()
		expect(menuItem(t(METRICS_KEY))).not.toBeNull()
	})

	it("reports a click on a later button with its own id", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY) },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await openDropdown(wrapper)
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
