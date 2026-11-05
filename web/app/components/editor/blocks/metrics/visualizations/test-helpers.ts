// shared helpers for the chart component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { nextTick } from "vue"
import { emitFrom } from "~/components/test-helpers"

// eslint's ts program resolves .vue imports as error typed, so a chart
// component handed to mountChart looks unsafe to it while vue-tsc types
// it fine
type TestComponent = any

export interface ChartSeries {
	name?: string
	type?: string
	data?: unknown
	animation?: boolean
	markLine?: {
		data: {
			yAxis: number
			label: { show: boolean; formatter?: string }
		}[]
	}
	min?: number
	max?: number
	axisLine?: { lineStyle: { color: [number, string][] } }
	progress?: { itemStyle: { color: string } }
	detail?: { formatter: string; fontSize: number }
}

// the slice of the echarts option the suites assert on
export interface ChartOption {
	title: { show: boolean; text?: string; top: number }
	legend: { show?: boolean; data?: string[] }
	tooltip: {
		confine?: boolean
		triggerOn?: string
		appendTo?: unknown
		className?: string
	}
	grid: { top: number; bottom: number }
	xAxis: { axisLabel: { formatter: (value: number) => string } }
	yAxis: {
		min?: number
		max?: number
		axisLabel: { formatter: (value: number) => string }
	}
	series?: ChartSeries[]
}

export function mountChart(
	component: TestComponent,
	props: Record<string, unknown>,
) {
	return mountSuspended(component, { props: props })
}

// the whole drawing contract of a chart component is the option object it
// hands echarts
export function chartOption(wrapper: VueWrapper): ChartOption {
	return wrapper
		.findComponent({ name: "VChart" })
		.props("option") as ChartOption
}

// noUncheckedIndexedAccess types every index access as possibly
// undefined, and the non-null assertion is banned, so an out-of-range
// series says so instead of failing later on an unrelated line
export function chartSeries(option: ChartOption, index = 0): ChartSeries {
	const series = option.series?.[index]
	if (!series) {
		throw new Error(`no chart series at index ${index}`)
	}

	return series
}

// one row of an axis tooltip, as echarts hands it to the slot
export function tooltipParam(
	seriesName: string,
	value: number,
	seriesIndex = 0,
) {
	return {
		seriesName: seriesName,
		seriesIndex: seriesIndex,
		marker: "",
		value: [new Date("2026-01-01T00:00:00Z"), value],
	}
}

// a click in the plot at a fixed spot, as echarts reports it on zr:click
export async function clickChart(wrapper: VueWrapper) {
	emitFrom(wrapper, "VChart", "zr:click", { offsetX: 120, offsetY: 40 })
	await nextTick()
}
