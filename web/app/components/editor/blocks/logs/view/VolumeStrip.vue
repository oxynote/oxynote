<script lang="ts" setup>
import { use } from "echarts/core"
import type { ComposeOption } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { BarChart } from "echarts/charts"
import type { BarSeriesOption } from "echarts/charts"
import { GridComponent, TooltipComponent } from "echarts/components"
import type {
	GridComponentOption,
	TooltipComponentOption,
} from "echarts/components"
import VChart from "vue-echarts"
import { cn } from "#layers/ui/app/lib/utils"
import type { VolumeBucket } from "../volume"
import { chartStyles } from "~/assets/css"

use([CanvasRenderer, BarChart, GridComponent, TooltipComponent])

type ECOption = ComposeOption<
	BarSeriesOption | GridComponentOption | TooltipComponentOption
>

interface TooltipParam {
	seriesId: string
	seriesName: string
	marker: string
	value: number
	dataIndex: number
}

// the levels from the bottom of a bar to its top
const STACK_ORDER = [
	LogLevel.Critical,
	LogLevel.Error,
	LogLevel.Warning,
	LogLevel.Info,
	LogLevel.Debug,
	LogLevel.Trace,
	LogLevel.Unknown,
]
// the series that marks an interval without lines on the baseline
const EMPTY_SERIES_ID = "empty"
// how many times the axis names, spread evenly from start to end
const AXIS_LABELS = 4
// a window longer than this names days on its axis instead of times
const DAY_AXIS_MS = 2 * 24 * 60 * 60 * 1000

const props = defineProps<{
	buckets: VolumeBucket[]
	window: { from: Date; to: Date }
}>()

const { t, d } = useI18n({ useScope: "global" })
const { color } = useAppearance()

const themeVersion = ref(0)
const option = computed<ECOption>(() => {
	// eslint-disable-next-line @typescript-eslint/no-unused-expressions -- reading it recomputes the colours once the theme changes
	themeVersion.value

	const st = chartStyles()
	const levels = STACK_ORDER.filter((level) =>
		props.buckets.some((bucket) => bucket.counts[level] > 0),
	)

	return {
		animation: false,
		grid: { top: 0, right: 0, bottom: 0, left: 0 },
		xAxis: {
			type: "category",
			show: false,
			data: props.buckets.map((bucket) => bucket.start),
		},
		yAxis: { type: "value", show: false, min: 0 },
		tooltip: {
			show: true,
			trigger: "axis",
			axisPointer: {
				type: "shadow",
				shadowStyle: { color: st.lineChart.mutedText, opacity: 0.25 },
			},
			transitionDuration: 0.2,
			backgroundColor: st.lineChart.tooltipBackground,
			borderColor: st.lineChart.tooltipBorder,
			borderWidth: 1,
			padding: 0,
			appendTo: () => document.body,
			className: cn("z-tooltip"),
		},
		series: [
			...levels.map((level) => ({
				id: level,
				name: t(`editor.logs.level-names.${level}`),
				type: "bar" as const,
				stack: "volume",
				barCategoryGap: "15%",
				itemStyle: { color: st.logLevels[level], opacity: 0.8 },
				emphasis: { disabled: true },
				data: props.buckets.map((bucket) => bucket.counts[level]),
			})),
			// echarts draws a zero at barMinHeight, which leaves a dash on the
			// baseline, and skips a null
			{
				id: EMPTY_SERIES_ID,
				type: "bar" as const,
				stack: "volume",
				barCategoryGap: "15%",
				barMinHeight: 2,
				itemStyle: { color: st.logVolumeEmpty },
				emphasis: { disabled: true },
				data: props.buckets.map((bucket) => (isEmpty(bucket) ? 0 : null)),
			},
		],
	}
})
const axisLabels = computed(() => {
	const from = props.window.from.getTime()
	const span = props.window.to.getTime() - from
	const format = span > DAY_AXIS_MS ? "month-day-short" : "hour-minute"

	return Array.from({ length: AXIS_LABELS }, (_, i) =>
		d(new Date(from + (span * i) / (AXIS_LABELS - 1)), format),
	)
})

watch(
	() => color.value.theme,
	async () => {
		// the theme's css variables apply a moment after the switch
		await nextTick()
		setTimeout(() => {
			themeVersion.value++
		}, 30)
	},
)

function isEmpty(bucket: VolumeBucket): boolean {
	return Object.values(bucket.counts).every((count) => count === 0)
}

function isEmptyAt(params: unknown): boolean {
	const bucket = tooltipBucket(params)

	return !!bucket && isEmpty(bucket)
}

function tooltipBucket(params: unknown): VolumeBucket | undefined {
	const [first] = Array.isArray(params) ? (params as TooltipParam[]) : []

	return first && props.buckets[first.dataIndex]
}

// tooltipRows reads the levels under the pointer, most severe first
function tooltipRows(params: unknown): TooltipParam[] {
	return Array.isArray(params)
		? (params as TooltipParam[]).filter((p) => p.seriesId !== EMPTY_SERIES_ID)
		: []
}

function tooltipTime(params: unknown): string {
	const bucket = tooltipBucket(params)

	return bucket ? d(new Date(bucket.start), "short-with-full-time") : ""
}
</script>
<template>
	<div
		role="img"
		:aria-label="$t('editor.logs.view.volume-label')"
		class="flex shrink-0 flex-col gap-1.5 px-2.5 pt-1.5 pb-2.5"
	>
		<div class="h-9">
			<VChart class="size-full" :option="option" autoresize>
				<template #tooltip="params">
					<div class="flex flex-col">
						<div class="px-2 pt-1 text-2sm text-foreground">
							{{ tooltipTime(params) }}
						</div>
						<div class="mt-1 mb-0.5 h-0.25 bg-border" />
						<div
							v-if="isEmptyAt(params)"
							class="px-2 pb-1 text-2sm text-foreground/60"
						>
							{{ $t("editor.logs.view.volume-empty") }}
						</div>
						<div v-else class="flex flex-col px-2 pb-1">
							<div
								v-for="row in tooltipRows(params)"
								:key="row.seriesId"
								class="flex items-center"
							>
								<!-- eslint-disable-next-line vue/no-v-html -->
								<div v-html="row.marker" />
								<div class="text-2sm text-foreground/60">
									{{ row.seriesName }}
								</div>
								<div class="ml-1.5 text-2sm text-foreground tabular-nums">
									{{ row.value }}
								</div>
							</div>
						</div>
					</div>
				</template>
			</VChart>
		</div>
		<div
			class="flex justify-between text-2xs leading-none text-muted-foreground tabular-nums"
		>
			<span v-for="(label, index) in axisLabels" :key="index">
				{{ label }}
			</span>
		</div>
	</div>
</template>
