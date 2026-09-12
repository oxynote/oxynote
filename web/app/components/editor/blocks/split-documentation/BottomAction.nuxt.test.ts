import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import BottomAction from "./BottomAction.vue"
import {
	findButtonByText,
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

describe("<SplitDocumentationBottomAction>", () => {
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

	it("renders one button per entry", async ({ expect }) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY), icon: "lucide:code" },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		expect(wrapper.findAll("button")).toHaveLength(2)
		expect(wrapper.text()).toContain(t(CODE_KEY))
		expect(wrapper.text()).toContain(t(METRICS_KEY))
		expect(renderedIconNames(wrapper)).toEqual([
			"lucide:code",
			"lucide:circle-plus",
		])
	})

	it("reports a click on a later button with its own id", async ({
		expect,
	}) => {
		const wrapper = await mountAction([
			{ id: "code", text: t(CODE_KEY) },
			{ id: "metrics", text: t(METRICS_KEY) },
		])

		await findButtonByText(wrapper, t(METRICS_KEY)).trigger("click")

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
