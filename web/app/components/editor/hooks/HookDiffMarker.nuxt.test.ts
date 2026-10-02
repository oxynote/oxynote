import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import HookDiffMarker from "./HookDiffMarker.vue"
import DiffCountPill from "../diff/DiffCountPill.vue"
import { stubViewportMatches } from "~/components/test-helpers"

// the viewport stub is shared, so these tests cannot interleave
describe("<HookDiffMarker>", { concurrent: false }, () => {
	beforeEach(() => {
		stubViewportMatches(true)
	})

	it("stacks the counts it is given", async ({ expect }) => {
		const wrapper = await mountMarker({ count: { removed: 3, added: 1 } })

		expect(pill(wrapper).props()).toEqual({
			removed: 3,
			added: 1,
			vertical: true,
		})
	})

	it("shows the hook icon in the hook's colour at rest", async ({ expect }) => {
		const wrapper = await mountMarker({ hookStatus: "stale" })

		expect(hookIconSlot(wrapper).classes()).toContain("h-3.75")
		expect(hookIconSlot(wrapper).classes()).not.toContain("h-0")
		expect(
			hookIconSlot(wrapper)
				.get(".i-mingcute\\:leaf-line")
				.attributes("data-hook-status"),
		).toBe("stale")
		expect(wrapper.classes()).not.toContain("translate-y-5.5")
	})

	it("moves below the handle and drops the hook icon once lifted", async ({
		expect,
	}) => {
		const wrapper = await mountMarker({ lifted: true })

		expect(wrapper.classes()).toContain("translate-y-5.5")
		expect(wrapper.classes()).toContain("lg:translate-y-6")
		expect(hookIconSlot(wrapper).classes()).toContain("h-0")
		expect(hookIconSlot(wrapper).classes()).toContain("lg:h-0")
		expect(hookIconSlot(wrapper).classes()).not.toContain("lg:h-4.25")
		expect(hookIconSlot(wrapper).classes()).toContain("opacity-0")
	})

	it("keeps the hook icon when lifted on a narrow screen", async ({
		expect,
	}) => {
		stubViewportMatches(false)

		const wrapper = await mountMarker({ lifted: true })

		expect(wrapper.classes()).toContain("translate-y-5.5")
		expect(hookIconSlot(wrapper).classes()).not.toContain("h-0")
		expect(hookIconSlot(wrapper).classes()).not.toContain("opacity-0")
	})
})

function mountMarker(props: Record<string, unknown> = {}) {
	return mountSuspended(HookDiffMarker, {
		props: {
			count: { removed: 1, added: 1 },
			hookStatus: "fresh",
			lifted: false,
			...props,
		},
	})
}

function pill(wrapper: VueWrapper) {
	return wrapper.getComponent(DiffCountPill)
}

function hookIconSlot(wrapper: VueWrapper) {
	return pill(wrapper).get("span > span")
}
