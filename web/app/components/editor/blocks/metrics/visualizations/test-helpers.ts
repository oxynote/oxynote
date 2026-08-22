// shared helpers for the chart component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"

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
	title: { show: boolean; text?: string }
	legend: { show?: boolean; data?: string[] }
	grid: { top: number; bottom: number }
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
