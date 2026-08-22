import { beforeEach, describe, it, vi } from "vitest"
import GaugeChart from "./GaugeChart.vue"
import {
	chartOption,
	chartSeries,
	mountChart,
	type ChartOption,
	type ChartSeries,
} from "./test-helpers"
import { stubChartColorContext } from "../test-helpers"
import { VisualizationDataUnit } from "../utils"
import type { MultipleGaugeChartData } from "."

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

const BASE_COLOR = "#8a3ffc"

const GAUGES: MultipleGaugeChartData = [{ name: "cpu", value: 40 }]

function mountGauge(props: Record<string, unknown> = {}) {
	return mountChart(GaugeChart, {
		gauges: GAUGES,
		baseThresholdColor: BASE_COLOR,
		unit: { type: null, custom: null },
		...props,
	})
}

// the ring that shows the value, which is the last series of each gauge
function valueRing(option: ChartOption, index = 0): ChartSeries {
	return chartSeries(option, index)
}

// chartStyles paints colours onto a canvas, so every mount needs the
// stand-in context installed first
describe("<GaugeChart>", { concurrent: false }, () => {
	beforeEach(stubChartColorContext)

	it("draws the value and its name", async ({ expect }) => {
		const wrapper = await mountGauge()

		const ring = valueRing(chartOption(wrapper))

		expect(ring.type).toBe("gauge")
		expect(ring.data).toEqual([{ value: 40, name: "cpu" }])
	})

	it("draws one gauge per result", async ({ expect }) => {
		const wrapper = await mountGauge({
			gauges: [
				{ name: "cpu", value: 40 },
				{ name: "memory", value: 70 },
			],
		})

		const option = chartOption(wrapper)

		expect(option.series).toHaveLength(2)
		expect(
			option.series?.map(
				(series) => (series.data as { name: string }[])[0]?.name,
			),
		).toEqual(["cpu", "memory"])
	})

	it("draws nothing for an empty result", async ({ expect }) => {
		const wrapper = await mountGauge({ gauges: [] })

		expect(chartOption(wrapper).series).toBeUndefined()
	})

	it("hides the title area when there is no title", async ({ expect }) => {
		const wrapper = await mountGauge()

		expect(chartOption(wrapper).title.show).toBe(false)
	})

	it("shows the title it was given", async ({ expect }) => {
		const wrapper = await mountGauge({ title: "CPU" })

		const option = chartOption(wrapper)

		expect(option.title.show).toBe(true)
		expect(option.title.text).toBe("CPU")
	})

	it("animates by default", async ({ expect }) => {
		const wrapper = await mountGauge()

		expect(valueRing(chartOption(wrapper)).animation).toBe(true)
	})

	it("holds the animation still when asked to", async ({ expect }) => {
		const wrapper = await mountGauge({ disableAnimation: true })

		expect(valueRing(chartOption(wrapper)).animation).toBe(false)
	})

	it("colours the value with the base colour when there are no thresholds", async ({
		expect,
	}) => {
		const wrapper = await mountGauge()

		expect(valueRing(chartOption(wrapper)).progress?.itemStyle.color).toBe(
			BASE_COLOR,
		)
	})

	it("adds a threshold ring in front of the value ring", async ({ expect }) => {
		const wrapper = await mountGauge({
			thresholds: [{ value: 50, color: "#ff0000" }],
		})

		const option = chartOption(wrapper)

		expect(option.series).toHaveLength(2)
		expect(chartSeries(option).axisLine?.lineStyle.color).toEqual([
			[expect.any(Number), BASE_COLOR],
			[1, "#ff0000"],
		])
	})

	it("colours the value with the highest threshold it has passed", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "cpu", value: 80 }],
			thresholds: [
				{ value: 50, color: "#ffaa00" },
				{ value: 75, color: "#ff0000" },
			],
		})

		expect(valueRing(chartOption(wrapper), 1).progress?.itemStyle.color).toBe(
			"#ff0000",
		)
	})

	it("colours a value below every threshold with the base colour", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "cpu", value: 10 }],
			thresholds: [{ value: 50, color: "#ff0000" }],
		})

		expect(valueRing(chartOption(wrapper), 1).progress?.itemStyle.color).toBe(
			BASE_COLOR,
		)
	})

	it("honours the axis bounds the config fixes", async ({ expect }) => {
		const wrapper = await mountGauge({ axisBounds: { min: 0, max: 200 } })

		const ring = valueRing(chartOption(wrapper))

		expect(ring.min).toBe(0)
		expect(ring.max).toBe(200)
	})

	it("shows the value in the configured unit", async ({ expect }) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "cpu", value: 1024 }],
			unit: { type: VisualizationDataUnit.Bytes, custom: null },
			decimals: 0,
		})

		expect(valueRing(chartOption(wrapper)).detail?.formatter).toBe("1 KB")
	})

	it("shrinks the text of a long value so it still fits", async ({
		expect,
	}) => {
		const short = await mountGauge({ gauges: [{ name: "cpu", value: 1 }] })
		const long = await mountGauge({
			gauges: [{ name: "cpu", value: 123456789.123456 }],
			decimals: 6,
		})

		expect(valueRing(chartOption(long)).detail?.fontSize).toBeLessThan(
			valueRing(chartOption(short)).detail?.fontSize ?? 0,
		)
	})
})
