<script lang="ts" setup>
import { use } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { GaugeChart as EChartsGaugeChart } from "echarts/charts"
import { TooltipComponent, TitleComponent } from "echarts/components"
import VChart from "vue-echarts"
import type { ComposeOption } from "echarts/core"
import type { GaugeSeriesOption } from "echarts/charts"
import type {
	TooltipComponentOption,
	TitleComponentOption,
} from "echarts/components"
import { useElementSize } from "@vueuse/core"
import { chartStyles } from "~/assets/css"
import {
	calculateGaugeAxisBounds,
	yAxisLabelFormatter,
	type GaugeChartData,
	type GaugeChartThreshold,
	type MultipleGaugeChartData,
} from "."
import type { VisualizationUnit } from "../utils"

// Register ECharts modules
use([CanvasRenderer, EChartsGaugeChart, TooltipComponent, TitleComponent])

// Define the option type
type ECOption = ComposeOption<
	GaugeSeriesOption | TooltipComponentOption | TitleComponentOption
>

const props = defineProps<{
	title?: string
	gauges: MultipleGaugeChartData
	unit: {
		type?: VisualizationUnit | null
		custom?: string | null
	}
	decimals?: number | null
	baseThresholdColor: string // color used from 0 until the first threshold
	thresholds?: GaugeChartThreshold[]
	axisBounds?: { min?: number | null; max?: number | null }
	titleRightPadding?: number
	disableAnimation?: boolean
}>()

const { color } = useAppearance()
const containerRef = useTemplateRef("chart-container")
const { width: containerWidth, height: containerHeight } =
	useElementSize(containerRef)

const MIN_GAUGE_SIZE = 180 // minimum size for a single gauge in pixels
const SIDE_PADDING = 10 // padding on each side in pixels
const GAUGE_GAP = 20 // gap between gauges in pixels
const BASE_SIZE = 300 // reference size for scaling

const gaugeLayout = computed(() => {
	const count = props.gauges.length
	if (count === 0) {
		return { gaugeSize: 0, totalWidth: 0, scale: 1 }
	}

	const availableHeight = Math.floor(containerHeight.value) - SIDE_PADDING * 2

	// for a single gauge, use container size; for multiple, use a fixed size per gauge
	let gaugeSize: number
	if (count === 1) {
		gaugeSize = Math.min(
			Math.floor(containerWidth.value) - SIDE_PADDING * 2,
			availableHeight,
		)
		gaugeSize = Math.max(gaugeSize, MIN_GAUGE_SIZE)
	} else {
		// multiple gauges: use height-based size with gentle scaling.
		// scale down slightly for more gauges but not as aggressively
		const multiGaugeScale = Math.max(0.7, 1 - (count - 1) * 0.05)
		gaugeSize = Math.max(
			MIN_GAUGE_SIZE,
			Math.floor(availableHeight * multiGaugeScale),
		)
	}

	// round gauge size to avoid sub-pixel rendering issues
	gaugeSize = Math.floor(gaugeSize)

	// total width needed for all gauges with gaps and padding
	const totalWidth =
		count * gaugeSize + (count - 1) * GAUGE_GAP + SIDE_PADDING * 2

	// scale factor for text/ring sizing based on gauge size
	const scale = Math.max(0.4, gaugeSize / BASE_SIZE)

	return { gaugeSize, totalWidth, scale }
})

const themeVersion = ref(0)
watch(
	() => color.value.theme,
	async () => {
		// track theme changes with nextTick/setTimeout to ensure CSS
		// variables are applied
		await nextTick()
		setTimeout(() => {
			themeVersion.value++
		}, 30)
	},
)

function buildGaugeSeries(
	gauge: GaugeChartData,
	center: [number, number],
	radius: number,
	scale: number,
	unit: typeof props.unit,
	decimals: number | null,
	bounds: { min: number; max: number },
	disableAnimation: boolean,
): GaugeSeriesOption[] {
	const st = chartStyles()
	const { min, max } = bounds
	const baseThresholdColor = props.baseThresholdColor

	// normalize thresholds to 0-1 range for axisLine segments
	const sortedThresholds = (clone(props.thresholds) ?? []).sort(
		(a, b) => a.value - b.value,
	)

	// build threshold segments for outer ring
	// Each threshold marks where its color STARTS (continues until next threshold)
	const thresholdSegments: [number, string][] = []
	let lastPos = 0

	for (const [i, t] of sortedThresholds.entries()) {
		const pos = Math.max(0, Math.min(1, (t.value - min) / (max - min)))

		// fill from lastPos to this threshold with the previous color
		if (pos > lastPos) {
			const prevColor = sortedThresholds[i - 1]?.color ?? baseThresholdColor
			thresholdSegments.push([pos, prevColor])
			lastPos = pos
		}
	}

	// fill remaining to 100% with the last threshold's color (or base if none)
	if (lastPos < 1) {
		const lastColor =
			sortedThresholds[sortedThresholds.length - 1]?.color ?? baseThresholdColor
		thresholdSegments.push([1, lastColor])
	}

	// if no segments built, use base color
	const outerSegments = thresholdSegments.length
		? thresholdSegments
		: [[1, baseThresholdColor] as [number, string]]

	// determine value color based on which threshold range it falls into
	let valueColor = baseThresholdColor

	// start with base color, then check each threshold
	for (const t of sortedThresholds) {
		if (gauge.value >= t.value) {
			valueColor = t.color
		}
	}

	const OUTER_WIDTH = Math.round(8 * scale) // outer ring width scaled
	const VISIBLE_GAP = Math.round(3 * scale) // visible gap between outer and inner rings
	const RING_GAP = OUTER_WIDTH + VISIBLE_GAP // total offset from outer radius to inner radius
	const INNER_WIDTH = Math.round(50 * scale) // inner ring width scaled
	const BASE_VALUE_FONT_SIZE = 30 * scale // base value text font size
	const VALUE_LINE_HEIGHT = Math.round(32 * scale) // value text line height scaled
	const TITLE_FONT_SIZE = Math.round(15 * scale) // title text font size scaled
	const TITLE_LINE_HEIGHT = Math.round(17 * scale) // title line height scaled
	const TEXT_WIDTH = Math.round(180 * scale) // text width scaled

	// calculate value font size based on text length
	const valueText = yAxisLabelFormatter(unit, gauge.value, decimals)
	const textLength = valueText.length

	// reduce font size for longer values: starts reducing after 7 chars
	// minimum factor of 0.35 allows up to ~24 characters to fit
	const lengthFactor = textLength <= 7 ? 1 : Math.max(0.35, 7 / textLength)
	const VALUE_FONT_SIZE = Math.round(BASE_VALUE_FONT_SIZE * lengthFactor)

	const hasThresholds = !!props.thresholds?.length

	// if no thresholds, inner ring extends to full outer radius and becomes
	// wider
	const innerRadius = hasThresholds ? radius - RING_GAP : radius
	const innerWidth = hasThresholds ? INNER_WIDTH : INNER_WIDTH + OUTER_WIDTH

	return [
		// outer ring - threshold segments (only shown when thresholds exist)
		...(hasThresholds
			? [
					{
						type: "gauge" as const,
						animation: !disableAnimation,
						center: center,
						radius: radius,
						startAngle: 200,
						endAngle: -20,
						min: min,
						max: max,
						splitNumber: 0,
						silent: true,
						axisLine: {
							lineStyle: {
								width: OUTER_WIDTH,
								color: outerSegments,
							},
						},
						axisTick: { show: false },
						splitLine: { show: false },
						axisLabel: { show: false },
						pointer: { show: false },
						detail: { show: false },
						progress: { show: false },
					},
				]
			: []),
		// inner ring - current value (bigger, in front)
		{
			type: "gauge",
			animation: !disableAnimation,
			center: center,
			radius: innerRadius,
			startAngle: 200,
			endAngle: -20,
			min: min,
			max: max,
			splitNumber: 0,
			axisLine: {
				lineStyle: {
					width: innerWidth,
					color: [[1, st.gaugeChart.emptyGauge]],
				},
			},
			axisTick: { show: false },
			splitLine: { show: false },
			axisLabel: { show: false },
			pointer: { show: false },
			progress: {
				show: true,
				width: innerWidth,
				itemStyle: {
					color: valueColor,
				},
			},
			silent: true,
			detail: {
				show: true,
				offsetCenter: [0, "0%"],
				fontSize: VALUE_FONT_SIZE,
				lineHeight: VALUE_LINE_HEIGHT,
				fontWeight: "bolder",
				fontFamily: st.fontFamily,
				color: valueColor,
				width: TEXT_WIDTH,
				overflow: "break",
				ellipsis: "...",
				formatter: valueText,
			},
			title: {
				show: true,
				offsetCenter: [0, "37%"],
				fontSize: TITLE_FONT_SIZE,
				lineHeight: TITLE_LINE_HEIGHT,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.gaugeChart.text,
				width: TEXT_WIDTH,
				overflow: "truncate",
				ellipsis: "...",
			},
			data: [
				{
					value: gauge.value,
					name: gauge.name,
				},
			],
		},
	]
}

const option = computed<ECOption>(() => {
	// Access themeVersion to trigger recompute after CSS variables are updated
	// eslint-disable-next-line @typescript-eslint/no-unused-expressions
	themeVersion.value

	const st = chartStyles()

	// echarts does not support dynamic width with ellipsis, so we calculate a
	// fixed width based on container size
	const titleRightPadding = props.titleRightPadding ?? 0
	const titleWidth = Math.max(
		containerWidth.value * 0.9 - titleRightPadding,
		50,
	)
	const res: ECOption = {
		title: {
			show: !!props.title,
			text: props.title,
			top: 10,
			left: 10,
			padding: 0,
			textStyle: {
				fontSize: 15, // text-2base
				lineHeight: 17,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.lineChart.text,
				overflow: "truncate",
				width: titleWidth,
				ellipsis: "...",
			},
		},
		series: [],
	}

	const count = props.gauges.length
	const { gaugeSize, totalWidth, scale } = gaugeLayout.value
	if (count === 0 || gaugeSize === 0) {
		return { res }
	}

	// Calculate shared bounds for all gauges (includes thresholds)
	const bounds = calculateGaugeAxisBounds(
		props.gauges,
		props.thresholds,
		0.1,
		props.axisBounds,
	)
	const series: GaugeSeriesOption[] = []

	// Calculate offset to center gauges when container is wider than needed
	const offsetX = Math.max(0, (containerWidth.value - totalWidth) / 2)

	for (const [i, gauge] of props.gauges.entries()) {
		let centerX: number
		if (count === 1) {
			centerX = containerWidth.value / 2
		} else {
			centerX =
				offsetX + SIDE_PADDING + gaugeSize / 2 + i * (gaugeSize + GAUGE_GAP)
		}

		const centerY = containerHeight.value * 0.6

		// radius: use percentage of gauge size (45% to leave room for text)
		const radius = gaugeSize * 0.45

		series.push(
			...buildGaugeSeries(
				gauge,
				[centerX, centerY],
				radius,
				scale,
				// Spread to access all nested fields for reactivity tracking
				// (they're used inside formatter callbacks which Vue can't track)
				{ ...props.unit },
				props.decimals ?? null,
				bounds,
				props.disableAnimation,
			),
		)
	}

	res.series = series

	return res
})

const chartStyle = computed(() => {
	const { totalWidth } = gaugeLayout.value
	// floor to avoid sub-pixel values that cause scrollbar flickering
	const minWidth = Math.floor(Math.max(totalWidth, containerWidth.value))
	const height = Math.floor(containerHeight.value)
	return {
		width: `${minWidth}px`,
		height: `${height}px`,
	}
})
</script>

<template>
	<div
		ref="chart-container"
		class="size-full min-w-0 overflow-x-auto overflow-y-hidden"
	>
		<VChart
			:style="chartStyle"
			:option="option"
			:update-options="{ notMerge: true }"
			autoresize
		/>
	</div>
</template>
