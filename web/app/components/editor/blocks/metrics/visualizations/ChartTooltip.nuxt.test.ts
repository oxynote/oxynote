import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import ChartTooltip from "./ChartTooltip.vue"
import { tooltipParam } from "./test-helpers"
import { VisualizationDataUnit } from "../utils"
import { t } from "~/components/test-helpers"

describe("<ChartTooltip>", () => {
	it("lists the series from the highest value down", async ({ expect }) => {
		const wrapper = await mountTooltip({
			params: [
				tooltipParam("api-low", 1, 0),
				tooltipParam("api-high", 30, 1),
				tooltipParam("api-mid", 2, 2),
			],
		})

		expect(tooltipRowTexts(wrapper)).toEqual([
			"api-high30",
			"api-mid2",
			"api-low1",
		])
	})

	it("formats each value in the configured unit", async ({ expect }) => {
		const wrapper = await mountTooltip({
			params: [tooltipParam("api", 1024)],
			unit: { type: VisualizationDataUnit.Bytes, custom: null },
			decimals: 0,
		})

		expect(tooltipRowTexts(wrapper)).toEqual(["api1 KB"])
	})

	it("keeps one row per series when several share a name", async ({
		expect,
	}) => {
		const wrapper = await mountTooltip({
			params: [
				tooltipParam("api", 6, 0),
				tooltipParam("web", 5, 1),
				tooltipParam("api", 4, 2),
				tooltipParam("web", 3, 3),
				tooltipParam("api", 2, 4),
			],
		})

		await wrapper.setProps({
			params: [
				tooltipParam("api", 1, 0),
				tooltipParam("web", 9, 1),
				tooltipParam("api", 8, 2),
				tooltipParam("web", 7, 3),
				tooltipParam("api", 0, 4),
			],
		})

		expect(tooltipRowTexts(wrapper)).toEqual([
			"web9",
			"api8",
			"web7",
			"api1",
			"api0",
		])
	})

	it("shows nothing but the frame before an instant is hovered", async ({
		expect,
	}) => {
		const wrapper = await mountTooltip({ params: undefined })

		expect(wrapper.text()).toBe("")
	})

	it("offers to pin a long list", async ({ expect }) => {
		const wrapper = await mountTooltip({ params: manyTooltipParams(11) })

		expect(wrapper.text()).toContain(
			t("editor.metrics.tooltip.pin-hint", { count: 11 }),
		)
	})

	it("tells how to let go of a pinned long list", async ({ expect }) => {
		const wrapper = await mountTooltip({
			params: manyTooltipParams(11),
			pinned: true,
		})

		expect(wrapper.text()).toContain(
			t("editor.metrics.tooltip.unpin-hint", { count: 11 }),
		)
		expect(wrapper.text()).not.toContain(
			t("editor.metrics.tooltip.pin-hint", { count: 11 }),
		)
	})

	it("shows no hint for a short list", async ({ expect }) => {
		const wrapper = await mountTooltip({ params: manyTooltipParams(10) })

		expect(wrapper.text()).not.toContain(
			t("editor.metrics.tooltip.pin-hint", { count: 10 }),
		)
	})
})

function mountTooltip(props: Record<string, unknown>) {
	return mountSuspended(ChartTooltip, {
		props: {
			params: [],
			unit: { type: null, custom: null },
			pinned: false,
			...props,
		},
	})
}

function manyTooltipParams(count: number) {
	return Array.from({ length: count }, (_, i) => tooltipParam(`api-${i}`, i, i))
}

// the text of every tooltip row, top to bottom
function tooltipRowTexts(wrapper: VueWrapper) {
	return wrapper.findAll(".overflow-y-auto > div").map((row) => row.text())
}
