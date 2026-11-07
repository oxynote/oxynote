import { flushPromises } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import { h } from "vue"
import DisabledHint from "./DisabledHint.vue"
import {
	clearTeleportedOverlays,
	mountUnderTooltipProvider,
	openTooltipText,
	stubViewportMatches,
} from "~/components/test-helpers"

const REASON = "Can't filter on this field."

// the teleport target and the stubbed matchMedia are shared, so these
// tests cannot interleave
describe("<DisabledHint>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it("shows its content", async ({ expect }) => {
		const wrapper = await mountHint()

		expect(wrapper.text()).toBe("Filter")
	})

	it("gives the reason in a tooltip on a screen that can hover", async ({
		expect,
	}) => {
		stubViewportMatches(false)
		const wrapper = await mountHint()

		await wrapper.get("[data-slot='tooltip-trigger']").trigger("focus")
		await flushPromises()

		expect(openTooltipText(wrapper)).toBe(REASON)
	})

	it("gives the reason in a popover on a tap when the screen cannot hover", async ({
		expect,
	}) => {
		stubViewportMatches((query) => query === "(hover: none)")
		const wrapper = await mountHint()
		// useMediaQuery reads the stub once mounted, then swaps the tooltip
		// for the popover
		await flushPromises()
		expect(wrapper.find("[data-slot='tooltip-trigger']").exists()).toBe(false)

		// under happy-dom a later task closes the popover again, so the check
		// runs straight after the tap, as in the other popover suites
		await wrapper.get("[data-slot='popover-trigger']").trigger("click")

		expect(
			document.body.querySelector("[data-slot='popover-content']")?.textContent,
		).toContain(REASON)
	})
})

function mountHint() {
	return mountUnderTooltipProvider(DisabledHint, {
		props: { reason: REASON },
		slots: { default: () => h("button", { disabled: true }, "Filter") },
	})
}
