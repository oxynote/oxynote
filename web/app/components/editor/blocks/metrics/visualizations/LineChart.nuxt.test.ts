import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import LineChart from "./LineChart.vue"
import { chartOption, chartSeries, mountChart } from "./test-helpers"
import { stubChartColorContext } from "../test-helpers"
import { VisualizationDataUnit } from "../utils"
import type { MultipleLineChartData } from "."
import { emitFrom, t } from "~/components/test-helpers"

// what the stand-in chart hands the tooltip slot, and the actions the
// component dispatches on it. The params are reactive so a test can move
// the cursor to another instant.
const echarts = await vi.hoisted(async () => {
	const { shallowRef } = await import("vue")

	return {
		tooltipParams: shallowRef<unknown[]>([]),
		dispatchAction: vi.fn(),
		containPixel: vi.fn(),
	}
})

// echarts draws onto a canvas happy-dom does not implement; the option
// the component builds is the whole contract under test, plus the
// tooltip it renders for the hovered instant
vi.mock("vue-echarts", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		default: defineComponent({
			name: "VChart",
			props: { option: { type: Object, required: true } },
			setup: (_props, { slots, expose }) => {
				expose({
					dispatchAction: echarts.dispatchAction,
					containPixel: echarts.containPixel,
				})

				return () => h("div", slots.tooltip?.(echarts.tooltipParams.value))
			},
		}),
	}
})

const SERIES: MultipleLineChartData = [
	{
		name: "web-1",
		data: [
			[new Date("2026-01-01T00:00:00Z"), 1],
			[new Date("2026-01-01T00:01:00Z"), 3],
		],
	},
	{
		name: "web-2",
		data: [[new Date("2026-01-01T00:00:00Z"), 2]],
	},
]

// local-time components keep the x-axis expectations timezone-independent
const DAY_SERIES: MultipleLineChartData = [
	{
		name: "web-1",
		data: [
			[new Date(2026, 9, 28, 14), 1],
			[new Date(2026, 9, 29, 14), 3],
		],
	},
]

const UNPINNED = {
	triggerOn: "mousemove",
	enterable: false,
	alwaysShowContent: false,
}
const PINNED = { triggerOn: "none", enterable: true, alwaysShowContent: true }
// where the test clicks the chart, and the actions that show the tooltip
// again there and hide it
const CLICK = { offsetX: 120, offsetY: 40 }
const SHOW_TIP = { type: "showTip", x: 120, y: 40 }
const HIDE_TIP = { type: "hideTip" }

function mountLine(
	props: Record<string, unknown> = {},
	options: { attachTo?: Element } = {},
) {
	return mountChart(
		LineChart,
		{
			seriesData: SERIES,
			unit: { type: null, custom: null },
			...props,
		},
		options,
	)
}

// chartStyles paints colours onto a canvas, so every mount needs the
// stand-in context installed first
describe("<LineChart>", { concurrent: false }, () => {
	// a chart listens on the document for the keys and presses that let go
	// of a pinned tooltip, so each one is taken down before the next test
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		stubChartColorContext()
		echarts.tooltipParams.value = []
		echarts.dispatchAction.mockReset()
		echarts.containPixel.mockReset()
		echarts.containPixel.mockReturnValue(true)
	})

	it("draws one line per series", async ({ expect }) => {
		const wrapper = await mountLine()

		const option = chartOption(wrapper)

		expect(option.series).toHaveLength(2)
		expect(option.series?.map((series) => series.name)).toEqual([
			"web-1",
			"web-2",
		])
		expect(chartSeries(option).type).toBe("line")
		expect(chartSeries(option).data).toEqual(SERIES[0]?.data)
	})

	it("draws nothing for an empty result", async ({ expect }) => {
		const wrapper = await mountLine({ seriesData: [] })

		expect(chartOption(wrapper).series).toEqual([])
	})

	it("hides the title area when there is no title", async ({ expect }) => {
		const wrapper = await mountLine()

		expect(chartOption(wrapper).title.show).toBe(false)
	})

	it("shows the title it was given", async ({ expect }) => {
		const wrapper = await mountLine({ title: "Requests" })

		const option = chartOption(wrapper)

		expect(option.title.show).toBe(true)
		expect(option.title.text).toBe("Requests")
	})

	it("leaves room above the plot for a title", async ({ expect }) => {
		const withTitle = await mountLine({ title: "Requests" })
		const withoutTitle = await mountLine()

		expect(chartOption(withTitle).grid.top).toBeGreaterThan(
			chartOption(withoutTitle).grid.top,
		)
	})

	it("keeps the legend hidden unless it is asked for", async ({ expect }) => {
		const wrapper = await mountLine()

		expect(chartOption(wrapper).legend.show).toBeFalsy()
	})

	it("lists every series in the legend", async ({ expect }) => {
		const wrapper = await mountLine({ showLegend: true })

		const option = chartOption(wrapper)

		expect(option.legend.show).toBe(true)
		expect(option.legend.data).toEqual(["web-1", "web-2"])
		expect(option.grid.bottom).toBeGreaterThan(10)
	})

	it("animates by default", async ({ expect }) => {
		const wrapper = await mountLine()

		expect(chartSeries(chartOption(wrapper)).animation).toBe(true)
	})

	it("holds the animation still when asked to", async ({ expect }) => {
		const wrapper = await mountLine({ disableAnimation: true })

		expect(chartSeries(chartOption(wrapper)).animation).toBe(false)
	})

	it("marks each threshold on the first series only", async ({ expect }) => {
		const wrapper = await mountLine({
			thresholds: [{ value: 2, color: "#ff0000", label: "warn" }],
		})

		const option = chartOption(wrapper)

		expect(chartSeries(option).markLine?.data).toEqual([
			expect.objectContaining({ yAxis: 2 }),
		])
		expect(chartSeries(option).markLine?.data[0]?.label.show).toBe(true)
		expect(chartSeries(option).markLine?.data[0]?.label.formatter).toBe("warn")
		expect(chartSeries(option, 1).markLine).toBeUndefined()
	})

	it("leaves an unlabelled threshold line unlabelled", async ({ expect }) => {
		const wrapper = await mountLine({
			thresholds: [{ value: 2, color: "#ff0000" }],
		})

		expect(
			chartSeries(chartOption(wrapper)).markLine?.data[0]?.label.show,
		).toBe(false)
	})

	it("marks nothing when there are no thresholds", async ({ expect }) => {
		const wrapper = await mountLine()

		expect(chartSeries(chartOption(wrapper)).markLine?.data).toEqual([])
	})

	it("honours the axis bounds the config fixes", async ({ expect }) => {
		const wrapper = await mountLine({ axisBounds: { min: -5, max: 50 } })

		const option = chartOption(wrapper)

		expect(option.yAxis.min).toBe(-5)
		expect(option.yAxis.max).toBe(50)
	})

	it("labels the value axis in the configured unit", async ({ expect }) => {
		const wrapper = await mountLine({
			unit: { type: VisualizationDataUnit.Bytes, custom: null },
			decimals: 0,
		})

		const label = chartOption(wrapper).yAxis.axisLabel.formatter

		expect(label(1024)).toBe("1 KB")
	})

	it("labels the time axis by hour between midnights", async ({ expect }) => {
		const wrapper = await mountLine({ seriesData: DAY_SERIES })

		const label = chartOption(wrapper).xAxis.axisLabel.formatter

		expect(label(new Date(2026, 9, 28, 18).getTime())).toBe("18:00")
		expect(label(new Date(2026, 9, 29).getTime())).toBe("Oct 29")
	})

	describe("tooltip", () => {
		it("stays inside the chart", async ({ expect }) => {
			const wrapper = await mountLine()

			const tooltip = chartOption(wrapper).tooltip

			expect(tooltip.confine).toBe(true)
			expect(tooltip.appendTo).toBeUndefined()
			// a tooltip wider than a narrow chart would still run off its edge
			expect(tooltip.className?.split(" ")).toContain("max-w-full")
		})

		it("lists the series from the highest value down", async ({ expect }) => {
			echarts.tooltipParams.value = [
				tooltipParam("api-low", 1, 0),
				tooltipParam("api-high", 30, 1),
				tooltipParam("api-mid", 2, 2),
			]

			const wrapper = await mountLine()

			const text = wrapper.text()

			expect(text.indexOf("api-high")).toBeLessThan(text.indexOf("api-mid"))
			expect(text.indexOf("api-mid")).toBeLessThan(text.indexOf("api-low"))
		})

		it("keeps one row per series when several share a name", async ({
			expect,
		}) => {
			echarts.tooltipParams.value = [
				tooltipParam("api", 6, 0),
				tooltipParam("web", 5, 1),
				tooltipParam("api", 4, 2),
				tooltipParam("web", 3, 3),
				tooltipParam("api", 2, 4),
			]
			const wrapper = await mountLine()

			echarts.tooltipParams.value = [
				tooltipParam("api", 1, 0),
				tooltipParam("web", 9, 1),
				tooltipParam("api", 8, 2),
				tooltipParam("web", 7, 3),
				tooltipParam("api", 0, 4),
			]
			await nextTick()

			expect(tooltipRowTexts(wrapper)).toEqual([
				"web9",
				"api8",
				"web7",
				"api1",
				"api0",
			])
		})

		it("follows the cursor until it is pinned", async ({ expect }) => {
			const wrapper = await mountLine()

			expect(pinState(wrapper)).toEqual(UNPINNED)
		})

		it("stays at the clicked instant so its list can be scrolled", async ({
			expect,
		}) => {
			const wrapper = await mountLine()

			await clickChart(wrapper)

			expect(pinState(wrapper)).toEqual(PINNED)
			expect.soft(echarts.dispatchAction.mock.calls).toEqual([[SHOW_TIP]])
		})

		it("ignores a click outside the plot, such as on the legend", async ({
			expect,
		}) => {
			echarts.containPixel.mockReturnValue(false)
			const wrapper = await mountLine()

			await clickChart(wrapper)

			expect(pinState(wrapper)).toEqual(UNPINNED)
			expect
				.soft(echarts.containPixel)
				.toHaveBeenCalledExactlyOnceWith("grid", [120, 40])
			expect.soft(echarts.dispatchAction).not.toHaveBeenCalled()
		})

		it("follows the cursor again after a second click", async ({ expect }) => {
			const wrapper = await mountLine()
			await clickChart(wrapper)

			await clickChart(wrapper)

			expect(pinState(wrapper)).toEqual(UNPINNED)
			expect
				.soft(echarts.dispatchAction.mock.calls)
				.toEqual([[SHOW_TIP], [HIDE_TIP]])
		})

		it("lets go when Escape is pressed", async ({ expect }) => {
			const wrapper = await mountLine()
			await clickChart(wrapper)

			document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
			await nextTick()

			expect(pinState(wrapper)).toEqual(UNPINNED)
			expect
				.soft(echarts.dispatchAction.mock.calls)
				.toEqual([[SHOW_TIP], [HIDE_TIP]])
		})

		it("lets go on a press outside the chart", async ({ expect }) => {
			const wrapper = await mountLine()
			await clickChart(wrapper)

			document.body.dispatchEvent(
				new PointerEvent("pointerdown", { bubbles: true }),
			)
			await nextTick()

			expect(pinState(wrapper)).toEqual(UNPINNED)
			expect
				.soft(echarts.dispatchAction.mock.calls)
				.toEqual([[SHOW_TIP], [HIDE_TIP]])
		})

		it("stays pinned on a press inside its own list", async ({ expect }) => {
			echarts.tooltipParams.value = [tooltipParam("api", 1)]
			const wrapper = await mountLine({}, { attachTo: document.body })
			await clickChart(wrapper)

			wrapper
				.get(".overflow-y-auto")
				.element.dispatchEvent(
					new PointerEvent("pointerdown", { bubbles: true }),
				)
			await nextTick()

			expect(pinState(wrapper)).toEqual(PINNED)
			expect.soft(echarts.dispatchAction.mock.calls).toEqual([[SHOW_TIP]])
		})

		it("offers to pin a long list", async ({ expect }) => {
			echarts.tooltipParams.value = manyTooltipParams(11)

			const wrapper = await mountLine()

			expect(wrapper.text()).toContain(
				t("editor.metrics.tooltip.pin-hint", { count: 11 }),
			)
		})

		it("tells how to let go of a pinned long list", async ({ expect }) => {
			echarts.tooltipParams.value = manyTooltipParams(11)
			const wrapper = await mountLine()

			await clickChart(wrapper)

			expect(wrapper.text()).toContain(
				t("editor.metrics.tooltip.unpin-hint", { count: 11 }),
			)
			expect(wrapper.text()).not.toContain(
				t("editor.metrics.tooltip.pin-hint", { count: 11 }),
			)
		})

		it("shows no hint for a short list", async ({ expect }) => {
			echarts.tooltipParams.value = manyTooltipParams(10)

			const wrapper = await mountLine()

			expect(wrapper.text()).not.toContain(
				t("editor.metrics.tooltip.pin-hint", { count: 10 }),
			)
		})
	})
})

// one row of an axis tooltip, as echarts hands it to the slot
function tooltipParam(seriesName: string, value: number, seriesIndex = 0) {
	return {
		seriesName: seriesName,
		seriesIndex: seriesIndex,
		marker: "",
		value: [new Date("2026-01-01T00:00:00Z"), value],
	}
}

function manyTooltipParams(count: number) {
	return Array.from({ length: count }, (_, i) => tooltipParam(`api-${i}`, i, i))
}

// the text of every tooltip row, top to bottom
function tooltipRowTexts(wrapper: VueWrapper) {
	return wrapper.findAll(".overflow-y-auto > div").map((row) => row.text())
}

async function clickChart(wrapper: VueWrapper) {
	emitFrom(wrapper, "VChart", "zr:click", CLICK)
	await nextTick()
}

function pinState(wrapper: VueWrapper) {
	const tooltip = chartOption(wrapper).tooltip

	return {
		triggerOn: tooltip.triggerOn,
		enterable: tooltip.enterable,
		alwaysShowContent: tooltip.alwaysShowContent,
	}
}
