<script lang="ts" setup>
import BarChart from "./visualizations/BarChart.vue"
import LineChart from "./visualizations/LineChart.vue"
import GaugeChart from "./visualizations/GaugeChart.vue"
import {
	CONFIG_DEBOUNCE_MS,
	RefreshInterval,
	refreshIntervalToMs,
	resolveTimeRange,
	type MetricConfig,
} from "./utils"
import {
	indexToAlphabeticLabel,
	mergeVisualizationResults,
	type MultipleBarChartData,
	type MultipleGaugeChartData,
	type MultipleLineChartData,
} from "./visualizations"

const props = defineProps<{
	config: MetricConfig
	uid: string
	titleRightPadding?: number
	hideEmptyActionButton?: boolean
	simplifiedEmpty?: boolean
	disableRefresh?: boolean
}>()
const emit = defineEmits<{
	(event: "loading", v: boolean): void
}>()

const { t } = useI18n({ useScope: "global" })
const { useMultipleGenericQueries } = useDataSourceAPI()
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

// NOTE: the debounced config cannot be changed (the changes won't be saved)
const debouncedConfig = ref(clone(props.config))
watchDebounced(
	() => props.config,
	(newV) => {
		debouncedConfig.value = clone(newV)
	},
	{ debounce: CONFIG_DEBOUNCE_MS, deep: true },
)

// after mount and uid changes we wait a bit before using the debounced config
// because it tends to cause "no data" states right after drag-drop or block
// creation due to the debounce delay
const canUseDebouncedConfigInParams = ref(false)

const processedThresholds = computed(() => {
	const thresholds = debouncedConfig.value.thresholds
	if (!thresholds?.length) {
		return []
	}

	return (
		thresholds
			// labels can be optional
			.filter(
				(v): v is { value: number; label?: string; color: string } =>
					v.value !== undefined && v.color !== undefined,
			)
			.map((t) => ({
				value: t.value,
				label: t.label,
				color: t.color,
			}))
	)
})
const fetchMetricData = useMultipleGenericQueries(
	() => props.config.dataSourceId,
	() => {
		const queries = canUseDebouncedConfigInParams.value
			? debouncedConfig.value.queries
			: props.config.queries
		const timeRange = canUseDebouncedConfigInParams.value
			? debouncedConfig.value.timeRange
			: props.config.timeRange
		const visualizationType = canUseDebouncedConfigInParams.value
			? debouncedConfig.value.visualizationType
			: props.config.visualizationType

		if (!queries?.length || !timeRange || !visualizationType) {
			return null
		}

		const resolvedTimeRange = resolveTimeRange(timeRange)

		return {
			queries: queries.map((q) => q.query),
			chartType: visualizationType,
			from: resolvedTimeRange.from,
			to: resolvedTimeRange.to,
			timeRangeKey: timeRange,
		}
	},
	() => false, // if true, the params above wouldn't trigger refetches
)
const processedQueries = computed(() => {
	const res = fetchMetricData.state.value.data
	const queries = debouncedConfig.value.queries

	if (!res || !queries) {
		return null
	}

	return queries.map((q, index) => ({
		name: q.name,
		legendFormat: q.legendFormat,
		result: res[index],
	}))
})

const data = computed<{
	status: GenericQueryResultStatus
	queryErrorMessage?: string
	queryIndex?: number
	multipleQueries?: boolean
	data?:
		| MultipleLineChartData
		| MultipleBarChartData
		| MultipleGaugeChartData
		| null
}>(() => {
	const results = fetchMetricData.state.value.data ?? []
	const queryErrIndex = results.findIndex(
		(d) => d.status === GenericQueryResultStatus.QueryError,
	)
	const queryErr = results[queryErrIndex]

	if (queryErr) {
		return {
			status: GenericQueryResultStatus.QueryError,
			queryErrorMessage: queryErr.queryErrorMessage,
			queryIndex: queryErrIndex,
			multipleQueries: results.length > 1,
			data: null,
		}
	}

	const res = processedQueries.value
	let orderIndex = 0

	return mergeVisualizationResults(
		props.config.visualizationType,
		res,
		debouncedConfig.value.decimals || null,
		(type) => {
			let prefix = ""
			switch (type) {
				case GenericQueryChartType.Line:
					prefix = t("editor.metrics.labels.line-chart")
					break
				case GenericQueryChartType.Bar:
					prefix = t("editor.metrics.labels.bar-chart")
					break
				case GenericQueryChartType.Gauge:
					prefix = t("editor.metrics.labels.gauge-chart")
					break
			}

			const res = `${prefix} ${indexToAlphabeticLabel(orderIndex)}`
			orderIndex++

			return res
		},
	)
})

useIntervalFn(
	() => {
		void refreshData()
	},
	() => refreshIntervalToMs(props.config.refreshInterval || RefreshInterval.M5),
	{
		immediate: true,
		immediateCallback: false,
	},
)

onMounted(async () => {
	await refreshData()
	setTimeout(() => {
		canUseDebouncedConfigInParams.value = true
	}, 3000)
})

watch(
	() => props.uid,
	() => {
		canUseDebouncedConfigInParams.value = false
		setTimeout(() => {
			canUseDebouncedConfigInParams.value = true
		}, 3000)
	},
)

async function refreshData() {
	if (
		!editorStore.activeDocumentId ||
		!props.config.dataSourceId ||
		!props.config.queries?.length ||
		!props.config.timeRange ||
		props.disableRefresh
	) {
		return
	}

	// we need to check and track this outside the composable because
	// because these components have their own refresh intervals and slightly
	// unpredictable mounting/unmounting behavior which would cause
	// unwanted data fetches (e.g. when dragging blocks around in the editor).
	// This makes the charts reinitialise and that's not good UX
	if (!editorStore.isMetricBlockDueForRefresh(props.uid)) {
		return
	}

	editorStore.setMetricBlockNextRefreshTimestamp(
		props.uid,
		refreshIntervalToMs(props.config.refreshInterval || RefreshInterval.M5),
	)

	emit("loading", true)
	await fetchMetricData.refetch() // refetch is intended
	emit("loading", false)
}

function openModal() {
	if (!props.uid) {
		return
	}

	editorStore.activateMetricBlockConfig(props.uid)
}
</script>
<template>
	<div class="flex size-full min-w-0 items-center justify-center">
		<template v-if="data.status === GenericQueryResultStatus.Ok && data.data">
			<LineChart
				v-if="props.config.visualizationType === GenericQueryChartType.Line"
				:series-data="data.data as MultipleLineChartData"
				:title="debouncedConfig.title || undefined"
				show-legend
				:thresholds="processedThresholds"
				:title-right-padding="props.titleRightPadding"
				:unit="debouncedConfig.unit"
				:decimals="debouncedConfig.decimals ?? null"
				:axis-bounds="debouncedConfig.axisBounds"
				:disable-animation="props.disableRefresh"
			/>
			<BarChart
				v-else-if="props.config.visualizationType === GenericQueryChartType.Bar"
				:title="debouncedConfig.title || undefined"
				:series-data="data.data as MultipleBarChartData"
				show-legend
				:thresholds="processedThresholds"
				:title-right-padding="props.titleRightPadding"
				:unit="debouncedConfig.unit"
				:decimals="debouncedConfig.decimals ?? null"
				:axis-bounds="debouncedConfig.axisBounds"
				:disable-animation="props.disableRefresh"
			/>
			<GaugeChart
				v-else-if="
					props.config.visualizationType === GenericQueryChartType.Gauge
				"
				:gauges="data.data as MultipleGaugeChartData"
				:title="debouncedConfig.title || undefined"
				:base-threshold-color="debouncedConfig.baseThresholdColor"
				:thresholds="processedThresholds"
				:title-right-padding="props.titleRightPadding"
				:unit="debouncedConfig.unit"
				:decimals="debouncedConfig.decimals ?? null"
				:axis-bounds="debouncedConfig.axisBounds"
				:disable-animation="props.disableRefresh"
			/>
		</template>
		<div
			v-else-if="data.status === GenericQueryResultStatus.NoData"
			class="text-foreground"
		>
			<template v-if="!props.simplifiedEmpty">
				<ShadcnUiEmpty v-if="config.dataSourceId">
					<ShadcnUiEmptyHeader>
						<ShadcnUiEmptyMedia variant="icon" class="size-9">
							<Icon name="lucide:chart-line" class="size-6" />
						</ShadcnUiEmptyMedia>
						<ShadcnUiEmptyTitle>
							{{ $t("editor.metrics.status.no-data-loaded.title") }}
						</ShadcnUiEmptyTitle>
						<ShadcnUiEmptyDescription>
							{{ $t("editor.metrics.status.no-data-loaded.description") }}
						</ShadcnUiEmptyDescription>
					</ShadcnUiEmptyHeader>
					<ShadcnUiEmptyContent v-if="!props.hideEmptyActionButton">
						<ShadcnUiButton variant="outline" size="2sm" @click="openModal">
							{{
								!isEditingDisabled
									? $t(
											"editor.metrics.status.no-data-loaded.normal-action-button",
										)
									: $t(
											"editor.metrics.status.no-data-loaded.readonly-action-button",
										)
							}}
						</ShadcnUiButton>
					</ShadcnUiEmptyContent>
				</ShadcnUiEmpty>
				<ShadcnUiEmpty v-else>
					<ShadcnUiEmptyHeader>
						<ShadcnUiEmptyMedia variant="icon" class="size-9">
							<Icon name="lucide:chart-line" class="size-6" />
						</ShadcnUiEmptyMedia>
						<ShadcnUiEmptyTitle>
							{{ $t("editor.metrics.status.data-source-not-selected.title") }}
						</ShadcnUiEmptyTitle>
						<ShadcnUiEmptyDescription>
							{{
								$t("editor.metrics.status.data-source-not-selected.description")
							}}
						</ShadcnUiEmptyDescription>
					</ShadcnUiEmptyHeader>
					<ShadcnUiEmptyContent v-if="!props.hideEmptyActionButton">
						<ShadcnUiButton variant="outline" size="2sm" @click="openModal">
							{{
								!isEditingDisabled
									? $t(
											"editor.metrics.status.data-source-not-selected.normal-action-button",
										)
									: $t(
											"editor.metrics.status.data-source-not-selected.readonly-action-button",
										)
							}}
						</ShadcnUiButton>
					</ShadcnUiEmptyContent>
				</ShadcnUiEmpty>
			</template>
			<ShadcnUiEmpty v-else>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:chart-line" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{
							$t("editor.metrics.status.simplified-config-in-progress.title")
						}}
					</ShadcnUiEmptyTitle>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
		<div
			v-else-if="data.status === GenericQueryResultStatus.TypeNotSelected"
			class="text-foreground"
		>
			<ShadcnUiEmpty v-if="!props.simplifiedEmpty">
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:chart-line" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ $t("editor.metrics.status.type-not-selected.title") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						{{ $t("editor.metrics.status.type-not-selected.description") }}
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
				<ShadcnUiEmptyContent v-if="!props.hideEmptyActionButton">
					<ShadcnUiButton variant="outline" size="2sm" @click="openModal">
						{{
							!isEditingDisabled
								? $t(
										"editor.metrics.status.type-not-selected.normal-action-button",
									)
								: $t(
										"editor.metrics.status.type-not-selected.readonly-action-button",
									)
						}}
					</ShadcnUiButton>
				</ShadcnUiEmptyContent>
			</ShadcnUiEmpty>
			<ShadcnUiEmpty v-else>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:chart-line" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{
							$t("editor.metrics.status.simplified-config-in-progress.title")
						}}
					</ShadcnUiEmptyTitle>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
		<div
			v-else-if="
				data.status === GenericQueryResultStatus.QueryError &&
				data.queryErrorMessage &&
				data.queryIndex !== undefined
			"
			class="text-foreground"
		>
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:chart-line" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ $t("editor.metrics.status.query-error.title") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						<i18n-t
							v-if="data.multipleQueries"
							keypath="editor.metrics.status.query-error.multi-query-description"
							tag="div"
						>
							<template #index>
								{{ data.queryIndex + 1 }}
							</template>
							<template #error>
								<span class="font-medium">
									{{ data.queryErrorMessage }}
								</span>
							</template>
						</i18n-t>
						<div v-else class="font-medium">
							{{ cleanSentenceCase(data.queryErrorMessage) }}
						</div>
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
				<ShadcnUiEmptyContent v-if="!props.hideEmptyActionButton">
					<ShadcnUiButton variant="outline" size="2sm" @click="openModal">
						{{
							!isEditingDisabled
								? $t("editor.metrics.status.invalid-data.normal-action-button")
								: $t(
										"editor.metrics.status.invalid-data.readonly-action-button",
									)
						}}
					</ShadcnUiButton>
				</ShadcnUiEmptyContent>
			</ShadcnUiEmpty>
		</div>
		<div v-else class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:chart-line" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ $t("editor.metrics.status.invalid-data.title") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						{{ $t("editor.metrics.status.invalid-data.description") }}
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
				<ShadcnUiEmptyContent v-if="!props.hideEmptyActionButton">
					<ShadcnUiButton variant="outline" size="2sm" @click="openModal">
						{{
							!isEditingDisabled
								? $t("editor.metrics.status.invalid-data.normal-action-button")
								: $t(
										"editor.metrics.status.invalid-data.readonly-action-button",
									)
						}}
					</ShadcnUiButton>
				</ShadcnUiEmptyContent>
			</ShadcnUiEmpty>
		</div>
	</div>
</template>
