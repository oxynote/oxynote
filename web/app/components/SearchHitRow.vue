<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type {
	DocumentSearchHit,
	DocumentSearchHitAttrsCodeBlock,
	DocumentSearchHitAttrsMetricBlock,
} from "~/utils"
import { extendedCodeBlockLanguageOptions } from "./editor/blocks/code-block/languages"
import { CODE_BLOCK_NAME, METRIC_BLOCK_NAME } from "./editor/blocks/node-names"

const props = defineProps<{
	hit: DocumentSearchHit
	href: string
}>()
const { t } = useI18n({ useScope: "global" })
// only a metric hit names a data source, so only a metric hit loads them
const fetchDataSources =
	props.hit.type === METRIC_BLOCK_NAME
		? useDataSourceAPI().fetchDataSources
		: null

const CHART_TITLE_I18N_KEYS: Record<GenericQueryChartType, string> = {
	[GenericQueryChartType.Line]:
		"editor.metrics.config.type-options.line-chart.title",
	[GenericQueryChartType.Bar]:
		"editor.metrics.config.type-options.bar-chart.title",
	[GenericQueryChartType.Gauge]:
		"editor.metrics.config.type-options.gauge-chart.title",
}

const codeAttrs = computed(() =>
	props.hit.type === CODE_BLOCK_NAME
		? (props.hit.attrs as DocumentSearchHitAttrsCodeBlock | undefined)
		: undefined,
)
const metricAttrs = computed(() =>
	props.hit.type === METRIC_BLOCK_NAME
		? (props.hit.attrs as DocumentSearchHitAttrsMetricBlock | undefined)
		: undefined,
)
// a metric block without a chart type it knows draws a line chart
const chartType = computed(() => {
	if (props.hit.type !== METRIC_BLOCK_NAME) {
		return null
	}

	const type = metricAttrs.value?.visualizationType

	return type && type in METRIC_CHART_ICONS ? type : GenericQueryChartType.Line
})
const icon = computed(() =>
	chartType.value
		? METRIC_CHART_ICONS[chartType.value]
		: blockIcon(props.hit.type),
)
const label = computed(() => {
	if (chartType.value) {
		return t(CHART_TITLE_I18N_KEYS[chartType.value])
	}

	if (codeAttrs.value) {
		return extendedCodeBlockLanguageOptions[codeAttrs.value.language]
	}

	return undefined
})
const dataSourceName = computed(
	() =>
		fetchDataSources?.data.value?.find(
			(dataSource) => dataSource.id === metricAttrs.value?.dataSourceId,
		)?.name,
)
</script>

<template>
	<NuxtLink
		:href="props.href"
		:prefetch="false"
		class="flex items-start gap-1.25 py-1.25 pr-2 pl-8.25 [@media(hover:none),(max-width:30rem)]:py-1.75"
	>
		<div class="flex w-4.5 shrink-0 justify-center">
			<Icon :name="icon" class="size-4 text-muted-foreground/70" />
		</div>
		<!-- eslint-disable vue/no-v-html -->
		<div
			:class="
				cn(
					'line-clamp-5 min-w-0 flex-1 break-words text-foreground/80 [&_mark]:rounded-xs [&_mark]:bg-comment-highlight/40 [&_mark]:text-inherit',
					props.hit.type === CODE_BLOCK_NAME
						? 'font-mono text-xs [&_mark]:font-mono'
						: 'text-2sm',
				)
			"
			v-html="sanitizeSearchResult(props.hit.text)"
		/>
		<!-- eslint-enable vue/no-v-html -->
		<div
			v-if="label"
			class="ml-2.5 shrink-0 pt-px text-2xs text-muted-foreground/70"
		>
			{{ label }}
			<span v-if="dataSourceName" class="max-[30rem]:hidden">
				{{ t("sidebar.search.metric-source", { source: dataSourceName }) }}
			</span>
		</div>
	</NuxtLink>
</template>
