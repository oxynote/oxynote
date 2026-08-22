import { beforeEach, describe, it, vi } from "vitest"
import BarChart from "./BarChart.vue"
import { chartOption, chartSeries, mountChart } from "./test-helpers"
import { stubChartColorContext } from "../test-helpers"
import { VisualizationDataUnit } from "../utils"
import type { MultipleBarChartData } from "."

// echarts draws onto a canvas happy-dom does not implement; the option
// the component builds is the whole contract under test
vi.mock("vue-echarts", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		default: defineComponent({
			name: "VChart",
			props: { option: { type: Object, required: true } },
			setup: () => () => h("div"),
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
	beforeEach(stubChartColorContext)

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
})
