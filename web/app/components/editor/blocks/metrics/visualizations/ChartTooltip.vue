<script lang="ts" setup>
import { yAxisLabelFormatter } from "."
import type { VisualizationUnit } from "../utils"

// one row of the axis tooltip, as echarts hands it to the slot
interface TooltipParam {
	seriesName: string
	seriesIndex: number
	marker: string
	value: [Date, number]
}

const props = defineProps<{
	// what echarts hands the tooltip slot
	params: unknown
	unit: {
		type?: VisualizationUnit | null
		custom?: string | null
	}
	decimals?: number | null
	pinned: boolean
}>()

// about as many rows as the tooltip shows before its list scrolls
const PIN_HINT_MIN_SERIES = 11

// the highest values first, so the rows that matter most are on top
const rows = computed<TooltipParam[]>(() => {
	if (!Array.isArray(props.params)) {
		return []
	}

	return [...(props.params as TooltipParam[])].sort(
		(a, b) => b.value[1] - a.value[1],
	)
})
</script>
<template>
	<div class="flex flex-col">
		<div v-if="rows[0]" class="px-2 pt-1 text-2sm text-foreground">
			{{ $d(rows[0].value[0], "short-with-full-time") }}
		</div>
		<div class="mt-1 mb-0.5 h-0.25 bg-border" />
		<div class="flex max-h-44 flex-col overflow-y-auto px-2 pb-1">
			<div v-for="row in rows" :key="row.seriesIndex" class="flex items-center">
				<!-- eslint-disable-next-line vue/no-v-html -->
				<div v-html="row.marker" />
				<div class="min-w-0 truncate text-2sm text-foreground/60">
					{{ row.seriesName }}
				</div>
				<div class="ml-1.5 shrink-0 text-2sm text-foreground">
					{{
						yAxisLabelFormatter(
							props.unit,
							row.value[1],
							props.decimals ?? null,
						)
					}}
				</div>
			</div>
		</div>
		<div
			v-if="rows.length >= PIN_HINT_MIN_SERIES"
			class="border-t border-border px-2 py-1 text-2sm text-foreground/60"
		>
			{{
				$t(
					pinned
						? "editor.metrics.tooltip.unpin-hint"
						: "editor.metrics.tooltip.pin-hint",
					{ count: rows.length },
				)
			}}
		</div>
	</div>
</template>
