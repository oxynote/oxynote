import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import ChartTitle from "./ChartTitle.vue"

const PADDING_CASES = [
	{
		name: "spans most of the chart",
		input: undefined,
		expected: "max(calc(90% - 0px), 3.125rem)",
	},
	{
		name: "keeps clear of the controls beside it",
		input: 40,
		expected: "max(calc(90% - 40px), 3.125rem)",
	},
]

describe("<ChartTitle>", () => {
	it("shows the title", async ({ expect }) => {
		const wrapper = await mountSuspended(ChartTitle, {
			props: { title: "Requests" },
		})

		expect(wrapper.text()).toBe("Requests")
	})

	it.for(PADDING_CASES)("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountSuspended(ChartTitle, {
			props: { title: "Requests", rightPadding: input },
		})

		expect(wrapper.attributes("style")).toContain(`max-width: ${expected}`)
	})
})
