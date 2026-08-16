<script lang="ts" setup>
import { use } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { BarChart as EChartsBarChart } from "echarts/charts"
import {
	TitleComponent,
	TooltipComponent,
	LegendComponent,
	GridComponent,
	MarkLineComponent,
} from "echarts/components"
import VChart from "vue-echarts"
import type { ComposeOption } from "echarts/core"
import type { BarSeriesOption } from "echarts/charts"
import type {
	TitleComponentOption,
	TooltipComponentOption,
	LegendComponentOption,
	GridComponentOption,
	MarkLineComponentOption,
} from "echarts/components"
import { chartStyles } from "~/assets/css"
import {
	calculateYAxisBounds,
	installSoloLegendBehavior,
	xAxisLabelFormatter,
	yAxisLabelFormatter,
	type BarChartThreshold,
	type LegendSelectChangedPayload,
	type MultipleBarChartData,
} from "."
import { cn } from "~/lib/utils"
import type { VisualizationUnit } from "../utils"

// Register ECharts modules
use([
	CanvasRenderer,
	EChartsBarChart,
	TitleComponent,
	TooltipComponent,
	LegendComponent,
	GridComponent,
	MarkLineComponent,
])

// Define the option type
type ECOption = ComposeOption<
	| BarSeriesOption
	| TitleComponentOption
	| TooltipComponentOption
	| LegendComponentOption
	| GridComponentOption
	| MarkLineComponentOption
>

const props = defineProps<{
	title?: string
	seriesData: MultipleBarChartData
	unit: {
		type?: VisualizationUnit | null
		custom?: string | null
	}
	decimals?: number | null
	showLegend?: boolean
	thresholds?: BarChartThreshold[]
	axisBounds?: { min?: number | null; max?: number | null }
	titleRightPadding?: number
	disableAnimation?: boolean
}>()

const containerElem = useTemplateRef<HTMLDivElement>("chart-container")
const { width: containerWidth } = useElementSize(containerElem)
const chartElem = useTemplateRef<InstanceType<typeof VChart>>("chart")
const { color } = useAppearance()
const { browserType } = useDetectHost()

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

const option = computed<ECOption>(() => {
	// Access themeVersion to trigger recompute after CSS variables are updated
	// eslint-disable-next-line @typescript-eslint/no-unused-expressions
	themeVersion.value

	// Spread to access all nested fields for reactivity tracking
	// (they're used inside formatter callbacks which Vue can't track)
	const unit: typeof props.unit = { ...props.unit }
	const decimals = props.decimals ?? null

	const st = chartStyles()

	// echarts does not support dynamic width with ellipsis, so we calculate a
	// fixed width based on container size
	const titleRightPadding = props.titleRightPadding ?? 0
	const titleWidth = Math.max(
		containerWidth.value * 0.9 - titleRightPadding,
		50,
	)

	return {
		color: st.dataColors,
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
		tooltip: {
			show: true,
			trigger: "axis",
			axisPointer: {
				type: "none",
			},
			transitionDuration: 0.2,
			backgroundColor: st.lineChart.tooltipBackground,
			borderColor: st.lineChart.tooltipBorder,
			borderWidth: 1,
			padding: 0,
			order: "seriesAsc",
			appendTo: () => document.body,
			className: cn("z-tooltip"),
		},
		legend: {
			show: props.showLegend,
			type: "scroll",
			data: props.seriesData.map((s) => s.name),
			bottom: 10,
			left: 18,
			padding: [0, 10, 0, 0],
			itemGap: 10,
			textStyle: {
				fontSize: 11, // text-2xs
				lineHeight: 13,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.lineChart.text,
			},
			icon: "roundRect",
			itemWidth: 12,
			itemHeight: 5,
			inactiveColor: st.lineChart.mutedText,
			pageButtonItemGap: 0,
			pageTextStyle: {
				fontSize: 11, // text-2xs
				lineHeight: 13,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.lineChart.text,
			},
			pageIcons: {
				horizontal: [st.lineChart.leftArrow, st.lineChart.rightArrow],
			},
			pageIconSize: 9,
			pageIconColor: st.lineChart.legendScrollButton,
			pageIconInactiveColor: st.lineChart.legendScrollButtonInactive,
		},
		grid: {
			left: 10,
			right: 12,
			bottom: props.showLegend ? 30 : 10,
			top: props.title ? 40 : 10,
			outerBoundsMode: "same",
			outerBoundsContain: "axisLabel",
		},
		xAxis: {
			type: "time",
			axisTick: {
				show: false,
			},
			axisLine: {
				show: false,
			},
			splitLine: {
				show: false,
			},
			axisLabel: {
				show: true,
				hideOverlap: true,
				margin: 6,
				fontSize: 11, // text-2xs
				lineHeight: 13,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.lineChart.text,
				formatter: xAxisLabelFormatter(props.seriesData),
			},
		},
		yAxis: {
			type: "value",
			...calculateYAxisBounds(
				props.seriesData,
				props.thresholds,
				0.1,
				props.axisBounds,
			),
			axisTick: {
				show: false,
			},
			axisLine: {
				show: false,
			},
			splitLine: {
				show: true,
				lineStyle: {
					type: "solid",
					color: st.lineChart.gridLine,
				},
			},
			axisLabel: {
				show: true,
				margin: 6,
				fontSize: 11, // text-2xs
				lineHeight: 13,
				fontWeight: "normal",
				fontFamily: st.fontFamily,
				color: st.lineChart.text,
				formatter: (value: number) =>
					yAxisLabelFormatter(unit, value, decimals),
			},
		},
		series: props.seriesData.map((s, index) => ({
			z: 5,
			name: s.name,
			type: "bar" as const,
			animation: !props.disableAnimation,
			data: s.data,
			barGap: "0%",
			emphasis: {
				// safari's canvas implementation mishandles globalAlpha during
				// emphasis/blur transitions, causing bars to disappear
				focus: browserType.value === HostBrowserType.Safari ? "none" : "series",
				disabled: browserType.value === HostBrowserType.Safari,
			},
			itemStyle: {
				borderRadius: [2, 2, 0, 0] as [number, number, number, number],
			},
			markLine:
				index === 0
					? {
							silent: true,
							symbol: "none",
							z: 10,
							animation: !props.disableAnimation,
							label: {
								distance: [0, 4],
							},
							emphasis: {
								disabled: true,
							},
							data: (props.thresholds ?? []).map((t) => ({
								yAxis: t.value,
								lineStyle: {
									color: t.color,
									type: [8, 6] as [number, number],
									width: 1,
									opacity: 1, // make sure line is fully visible even when hovering other bars
								},
								label: {
									show: !!t.label,
									formatter: t.label,
									position: "insideStartTop" as const,
									color: t.color,
									fontSize: 11,
									lineHeight: 13,
									fontWeight: "normal" as const,
									fontFamily: st.fontFamily,
								},
							})),
						}
					: undefined,
		})),
	}
})
const legendHandler = computed(() => {
	if (!chartElem.value) {
		return () => {
			// no-op until the chart element is mounted
		}
	}

	return installSoloLegendBehavior(chartElem.value)
})
</script>
<template>
	<div ref="chart-container" class="size-full">
		<VChart
			ref="chart"
			class="size-full"
			:option="option"
			autoresize
			@legendselectchanged="(p: LegendSelectChangedPayload) => legendHandler(p)"
		>
			<template #tooltip="params">
				<div class="flex flex-col">
					<div
						v-if="Array.isArray(params) && params.length > 0"
						class="px-2 pt-1 text-2sm text-foreground"
					>
						{{
							$d((params[0] as any).value[0] as Date, "short-with-full-time")
						}}
					</div>
					<div class="mt-1 mb-0.5 h-0.25 bg-border" />
					<div class="flex flex-col px-2 pb-1">
						<div
							v-for="(param, i) in params"
							:key="(param as any).seriesName || i"
							class="flex items-center"
						>
							<!-- eslint-disable-next-line vue/no-v-html -->
							<div v-html="(param as any).marker" />
							<div class="text-2sm text-foreground/60">
								{{ (param as any).seriesName }}
							</div>
							<div class="ml-1.5 text-2sm text-foreground">
								{{
									yAxisLabelFormatter(
										props.unit,
										(param as any).value[1],
										props.decimals ?? null,
									)
								}}
							</div>
						</div>
					</div>
				</div>
			</template>
		</VChart>
	</div>
</template>
