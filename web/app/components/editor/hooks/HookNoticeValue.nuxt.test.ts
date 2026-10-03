import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookNoticeValue from "./HookNoticeValue.vue"

describe("<HookNoticeValue>", () => {
	it("shows a value in bold", async ({ expect }) => {
		const wrapper = await mountSuspended(HookNoticeValue, {
			props: { value: "oxynote.test", fallback: "the page" },
		})

		expect(wrapper.text()).toBe("oxynote.test")
		expect(wrapper.find(".font-semibold").text()).toBe("oxynote.test")
	})

	it.for([
		{ name: "missing", input: undefined },
		{ name: "null", input: null },
		{ name: "empty", input: "" },
	])(
		"shows the fallback plainly while the value is $name",
		async ({ input }, { expect }) => {
			const wrapper = await mountSuspended(HookNoticeValue, {
				props: { value: input, fallback: "the page" },
			})

			expect(wrapper.text()).toBe("the page")
			expect(wrapper.find(".font-semibold").exists()).toBe(false)
		},
	)
})
