import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import BarChart from "./BarChart.vue"
import {
	chartOption,
	chartSeries,
	clickChart,
	mountChart,
	tooltipParam,
} from "./test-helpers"
import { stubChartColorContext } from "../test-helpers"
import { VisualizationDataUnit } from "../utils"
import type { MultipleBarChartData } from "."

// what the stand-in chart hands the tooltip slot, and the actions the
// component dispatches on it
const echarts = vi.hoisted(() => ({
	tooltipParams: [] as unknown[],
	dispatchAction: vi.fn(),
	containPixel: vi.fn(),
}))

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

				return () => h("div", slots.tooltip?.(echarts.tooltipParams))
			},
		}),
	}
})

const SERIES: MultipleBarChartData = [
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
const DAY_SERIES: MultipleBarChartData = [
	{
		name: "web-1",
		data: [
			[new Date(2026, 9, 28, 14), 1],
			[new Date(2026, 9, 29, 14), 3],
		],
	},
]

function mountBar(props: Record<string, unknown> = {}) {
	return mountChart(BarChart, {
		seriesData: SERIES,
		unit: { type: null, custom: null },
		...props,
	})
}

// chartStyles paints colours onto a canvas, so every mount needs the
// stand-in context installed first
describe("<BarChart>", { concurrent: false }, () => {
	// a chart listens on the document for the keys and presses that let go
	// of a pinned tooltip, so each one is taken down before the next test
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		stubChartColorContext()
		echarts.tooltipParams = []
		echarts.dispatchAction.mockReset()
		echarts.containPixel.mockReset()
		echarts.containPixel.mockReturnValue(true)
	})

	it("draws one bar series per result", async ({ expect }) => {
		const wrapper = await mountBar()

		const option = chartOption(wrapper)

		expect(option.series).toHaveLength(2)
		expect(option.series?.map((series) => series.name)).toEqual([
			"web-1",
			"web-2",
		])
		expect(chartSeries(option).type).toBe("bar")
		expect(chartSeries(option).data).toEqual(SERIES[0]?.data)
	})

	it("draws nothing for an empty result", async ({ expect }) => {
		const wrapper = await mountBar({ seriesData: [] })

		expect(chartOption(wrapper).series).toEqual([])
	})

	it("hides the title area when there is no title", async ({ expect }) => {
		const wrapper = await mountBar()

		expect(chartOption(wrapper).title.show).toBe(false)
	})

	it("shows the title it was given", async ({ expect }) => {
		const wrapper = await mountBar({ title: "Requests" })

		const option = chartOption(wrapper)

		expect(option.title.show).toBe(true)
		expect(option.title.text).toBe("Requests")
	})

	it("moves the title and the plot down by the offset it was given", async ({
		expect,
	}) => {
		const moved = await mountBar({ title: "Requests", titleOffsetTop: 6.5 })
		const kept = await mountBar({ title: "Requests" })

		expect(chartOption(moved).title.top).toBe(chartOption(kept).title.top + 6.5)
		expect(chartOption(moved).grid.top).toBe(chartOption(kept).grid.top + 6.5)
	})

	it("leaves room above the plot for a title", async ({ expect }) => {
		const withTitle = await mountBar({ title: "Requests" })
		const withoutTitle = await mountBar()

		expect(chartOption(withTitle).grid.top).toBeGreaterThan(
			chartOption(withoutTitle).grid.top,
		)
	})

	it("keeps the legend hidden unless it is asked for", async ({ expect }) => {
		const wrapper = await mountBar()

		expect(chartOption(wrapper).legend.show).toBeFalsy()
	})

	it("lists every series in the legend", async ({ expect }) => {
		const wrapper = await mountBar({ showLegend: true })

		const option = chartOption(wrapper)

		expect(option.legend.show).toBe(true)
		expect(option.legend.data).toEqual(["web-1", "web-2"])
		expect(option.grid.bottom).toBeGreaterThan(10)
	})

	it("animates by default", async ({ expect }) => {
		const wrapper = await mountBar()

		expect(chartSeries(chartOption(wrapper)).animation).toBe(true)
	})

	it("holds the animation still when asked to", async ({ expect }) => {
		const wrapper = await mountBar({ disableAnimation: true })

		expect(chartSeries(chartOption(wrapper)).animation).toBe(false)
	})

	it("marks each threshold on the first series only", async ({ expect }) => {
		const wrapper = await mountBar({
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
		const wrapper = await mountBar({
			thresholds: [{ value: 2, color: "#ff0000" }],
		})

		expect(
			chartSeries(chartOption(wrapper)).markLine?.data[0]?.label.show,
		).toBe(false)
	})

	it("marks nothing when there are no thresholds", async ({ expect }) => {
		const wrapper = await mountBar()

		expect(chartSeries(chartOption(wrapper)).markLine?.data).toEqual([])
	})

	it("honours the axis bounds the config fixes", async ({ expect }) => {
		const wrapper = await mountBar({ axisBounds: { min: -5, max: 50 } })

		const option = chartOption(wrapper)

		expect(option.yAxis.min).toBe(-5)
		expect(option.yAxis.max).toBe(50)
	})

	it("labels the value axis in the configured unit", async ({ expect }) => {
		const wrapper = await mountBar({
			unit: { type: VisualizationDataUnit.Bytes, custom: null },
			decimals: 0,
		})

		const label = chartOption(wrapper).yAxis.axisLabel.formatter

		expect(label(1024)).toBe("1 KB")
	})

	it("labels the time axis by hour between midnights", async ({ expect }) => {
		const wrapper = await mountBar({ seriesData: DAY_SERIES })

		const label = chartOption(wrapper).xAxis.axisLabel.formatter

		expect(label(new Date(2026, 9, 28, 18).getTime())).toBe("18:00")
		expect(label(new Date(2026, 9, 29).getTime())).toBe("Oct 29")
	})

	describe("tooltip", () => {
		it("stays inside the chart", async ({ expect }) => {
			const wrapper = await mountBar()

			const tooltip = chartOption(wrapper).tooltip

			expect(tooltip.confine).toBe(true)
			expect(tooltip.appendTo).toBeUndefined()
			expect(tooltip.className?.split(" ")).toContain("max-w-full")
		})

		it("lists the series from the highest value down", async ({ expect }) => {
			echarts.tooltipParams = [
				tooltipParam("web-1", 1, 0),
				tooltipParam("web-2", 2, 1),
			]

			const wrapper = await mountBar()

			const text = wrapper.text()

			expect(text.indexOf("web-2")).toBeLessThan(text.indexOf("web-1"))
		})

		it("stays at the clicked instant so its list can be scrolled", async ({
			expect,
		}) => {
			const wrapper = await mountBar()

			await clickChart(wrapper)

			expect(chartOption(wrapper).tooltip.triggerOn).toBe("none")
			expect
				.soft(echarts.dispatchAction.mock.calls)
				.toEqual([[{ type: "showTip", x: 120, y: 40 }]])
		})
	})
})
