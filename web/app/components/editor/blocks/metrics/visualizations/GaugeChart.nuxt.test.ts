import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { nextTick, ref } from "vue"
import GaugeChart from "./GaugeChart.vue"
import ChartTitle from "./ChartTitle.vue"
import {
	chartOption,
	chartSeries,
	mountChart,
	type ChartOption,
	type ChartSeries,
} from "./test-helpers"
import { stubChartColorContext } from "../test-helpers"
import { VisualizationDataUnit, VisualizationMiscUnit } from "../utils"
import type { MultipleGaugeChartData } from "."
import { t } from "~/components/test-helpers"

// the stand-in chart reports a canvas that never fits its host, so every
// redraw the component asks for reaches resize
const { chartStub } = vi.hoisted(() => ({
	chartStub: {
		resize: vi.fn(),
		getDom: () => ({ clientWidth: 1, clientHeight: 1 }),
		getWidth: () => 0,
		getHeight: () => 0,
	},
}))

const gaugeSize = vi.hoisted(() => ({
	width: null as unknown as { value: number },
	height: null as unknown as { value: number },
}))

mockNuxtImport("useChartSize", () => () => ({
	width: gaugeSize.width,
	height: gaugeSize.height,
	redraw: chartStub.resize,
}))

// echarts draws onto a canvas happy-dom does not implement; the option
// the component builds is the whole contract under test
vi.mock("vue-echarts", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		default: defineComponent({
			name: "VChart",
			props: { option: { type: Object, required: true } },
			setup: (_, { expose }) => {
				expose({ chart: chartStub })

				return () => h("div")
			},
		}),
	}
})

const BASE_COLOR = "#8a3ffc"

const GAUGES: MultipleGaugeChartData = [{ name: "cpu", value: 40 }]

const FIVE_GAUGES: MultipleGaugeChartData = Array.from(
	{ length: 5 },
	(_, i) => ({ name: `cpu-${i}`, value: 40 }),
)

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
	beforeEach(() => {
		stubChartColorContext()
		chartStub.resize.mockReset()
		gaugeSize.width = ref(500)
		gaugeSize.height = ref(320)
	})

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

	it("redraws the chart when its gauges need a wider row", async ({
		expect,
	}) => {
		const wrapper = await mountGauge()

		await wrapper.setProps({ gauges: FIVE_GAUGES })

		expect(chartStub.resize).toHaveBeenCalledTimes(1)
	})

	it("leaves the chart alone when its gauges keep the row's width", async ({
		expect,
	}) => {
		const wrapper = await mountGauge()

		await wrapper.setProps({ gauges: [{ name: "cpu", value: 70 }] })

		expect(chartStub.resize).toHaveBeenCalledTimes(0)
	})

	it("draws nothing for an empty result", async ({ expect }) => {
		const wrapper = await mountGauge({ gauges: [] })

		expect(chartOption(wrapper).series).toBeUndefined()
	})

	it("hides the title area when there is no title", async ({ expect }) => {
		const wrapper = await mountGauge()

		expect(wrapper.findComponent(ChartTitle).exists()).toBe(false)
	})

	it("moves the title down by the offset it was given", async ({ expect }) => {
		const moved = await mountGauge({ title: "CPU", titleOffsetTop: 6.5 })
		const kept = await mountGauge({ title: "CPU" })

		expect(moved.findComponent(ChartTitle).props("offsetTop")).toBe(6.5)
		expect(kept.findComponent(ChartTitle).props("offsetTop")).toBeUndefined()
	})

	it("shows the title it was given", async ({ expect }) => {
		const wrapper = await mountGauge({ title: "CPU" })

		expect(wrapper.findComponent(ChartTitle).text()).toBe("CPU")
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

	it("uses the configured percentage bounds even when a threshold exceeds them", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "cpu", value: 29 }],
			unit: { type: VisualizationMiscUnit.Percent0to100, custom: null },
			thresholds: [{ value: 120, color: "#ff0000" }],
		})

		const ring = valueRing(chartOption(wrapper), 1)

		expect(ring.min).toBe(0)
		expect(ring.max).toBe(100)
	})

	it("keeps the percentage default on an unconfigured bound", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "ratio", value: 0.29 }],
			unit: { type: VisualizationMiscUnit.Percent0to1, custom: null },
			axisBounds: { min: 0.2 },
		})

		const ring = valueRing(chartOption(wrapper))

		expect(ring.min).toBe(0.2)
		expect(ring.max).toBe(1)
	})

	it("keeps threshold positions valid when a percentage range is invalid", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({
			gauges: [{ name: "ratio", value: 0.29 }],
			unit: { type: VisualizationMiscUnit.Percent0to1, custom: null },
			axisBounds: { min: 20 },
			thresholds: [
				{ value: 0.5, color: "#ff0000" },
				{ value: 0.75, color: "#00ff00" },
			],
		})
		const option = chartOption(wrapper)
		const thresholdRing = chartSeries(option)
		const ring = valueRing(option, 1)

		expect(ring.min).toBe(0)
		expect(ring.max).toBe(1)
		expect(thresholdRing.axisLine?.lineStyle.color).toEqual([
			[0.5, BASE_COLOR],
			[0.75, "#ff0000"],
			[1, "#00ff00"],
		])
	})

	it("exposes overflowing gauges through a labelled keyboard scroll region", async ({
		expect,
	}) => {
		const wrapper = await mountGauge({ gauges: FIVE_GAUGES })
		const region = wrapper.get('[role="region"]')

		expect(region.attributes("tabindex")).toBe("0")
		expect(region.attributes("aria-label")).toBe(
			t("editor.metrics.gauge-scroll-label"),
		)
		expect(wrapper.text()).toContain(t("editor.metrics.gauge-scroll-hint"))
	})

	it("shows and hides the overflow cue as the series count changes", async ({
		expect,
	}) => {
		const wrapper = await mountGauge()
		const hint = t("editor.metrics.gauge-scroll-hint")

		expect(wrapper.text()).not.toContain(hint)

		await wrapper.setProps({ gauges: FIVE_GAUGES })
		expect(wrapper.text()).toContain(hint)

		await wrapper.setProps({ gauges: GAUGES })
		expect(wrapper.text()).not.toContain(hint)
	})

	it("shows the overflow cue when the container narrows", async ({
		expect,
	}) => {
		gaugeSize.height.value = 220
		const wrapper = await mountGauge({
			gauges: [
				{ name: "cpu", value: 40 },
				{ name: "memory", value: 70 },
			],
		})

		expect(wrapper.text()).not.toContain(t("editor.metrics.gauge-scroll-hint"))

		gaugeSize.width.value = 220
		await nextTick()

		expect(wrapper.text()).toContain(t("editor.metrics.gauge-scroll-hint"))
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
