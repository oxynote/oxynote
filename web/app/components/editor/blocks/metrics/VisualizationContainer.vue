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
	type MetricSimulationPreset,
} from "./utils"
import {
	DEFAULT_SIMULATION_PRESET,
	generateSimulatedQueryResults,
	isSimulationPreset,
	SIMULATION_CHECK_INTERVAL_MS,
} from "./simulation"
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
	(event: "simulate", preset: MetricSimulationPreset): void
}>()

const { t } = useI18n({ useScope: "global" })
const { fetchDataSources, useMultipleGenericQueries } = useDataSourceAPI()
const { checkMetricBlockSimulation } = useDocumentAPI()
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

// a simulation stands in for a metric that has not arrived yet. A source
// that cannot be reached is a different problem, and drawing a plausible
// chart over one hides the thing actually worth fixing — so the block
// queries instead, and says what the source said.
const dataSourceUnavailable = computed(() => {
	const dataSource = fetchDataSources.state.value.data?.find(
		(v) => v.id === props.config.dataSourceId,
	)

	return !!dataSource && dataSource.status !== DataSourceStatus.Success
})

// an unknown preset reads as "not simulating", so a block carrying one
// falls back to querying instead of breaking the render for every viewer
const simulationPreset = computed(() => {
	const preset: unknown = props.config.simulationPreset

	if (dataSourceUnavailable.value || !isSimulationPreset(preset)) {
		return null
	}

	return preset
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
	// a simulated block draws generated data, so it must not reach the data
	// source at all until the simulation is cleared
	() => !!simulationPreset.value,
)
// the preset a block simulated until its data arrived. The query starts
// cold the moment the simulation is lifted, and a block being answered is
// not one whose queries returned nothing — so it keeps drawing what it
// was drawing rather than flashing its empty state at data that is on its
// way. It follows the fetch and not the absence of a result: a query that
// cannot run never leaves "pending", and a block holding on that would
// draw generated data for good.
const clearedPreset = ref<MetricSimulationPreset | null>(null)

watch(simulationPreset, (preset, previous) => {
	if (preset) {
		clearedPreset.value = null

		return
	}

	if (previous) {
		clearedPreset.value = previous

		// the cached answer for this query is keyed by the block's time
		// range and not by the window it resolves to, so the one waiting
		// here was taken while the metric was still missing — which is
		// what the block has just been told it no longer is. refetch
		// ignores whether the query may run, so it is asked only when it
		// may.
		if (queryRunnable.value) {
			void fetchMetricData.refetch()
		}
	}
})

// bumped on every refresh tick so the generated window re-resolves against
// the current time and the chart slides forward like a live one
const simulationTick = ref(0)

// a block can only simulate what it will later query, so the same
// configuration a real query needs has to be in place first
// the same rows the query needs: a block that could simulate but never
// query would be stranded on generated data once the simulation lifted
const queryRunnable = computed(() => {
	return canRunGenericQueries(
		props.config.dataSourceId,
		props.config.queries?.map((q) => q.query),
	)
})

const canSimulate = computed(() => {
	return queryRunnable.value && !dataSourceUnavailable.value
})

// what the block draws, which outlives the attribute by one load
const drawnPreset = computed(() => {
	return (
		simulationPreset.value ??
		(fetchMetricData.asyncStatus.value === "loading"
			? clearedPreset.value
			: null)
	)
})

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
		MultipleLineChartData | MultipleBarChartData | MultipleGaugeChartData | null
}>(() => {
	const preset = drawnPreset.value
	if (preset) {
		// reading the tick is what makes a refresh regenerate the window
		void simulationTick.value

		return mergeVisualizationResults(
			props.config.visualizationType,
			generateSimulatedQueryResults(preset, props.config.timeRange),
			debouncedConfig.value.decimals || null,
			orderedLegendLabel(),
		)
	}

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

	return mergeVisualizationResults(
		props.config.visualizationType,
		processedQueries.value,
		debouncedConfig.value.decimals || null,
		orderedLegendLabel(),
	)
})

// labels series the query gave no legend format for, counting up per
// merge call ("Line A", "Line B", ...)
function orderedLegendLabel() {
	let orderIndex = 0

	return (type: GenericQueryChartType) => {
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
	}
}

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

const simulationCheck = useIntervalFn(
	() => {
		void checkSimulation()
	},
	SIMULATION_CHECK_INTERVAL_MS,
	{ immediate: false },
)

// the document and branch ids arrive with the route, often after the
// block has mounted, so the first check waits for them rather than being
// skipped until the next tick
watchImmediate(
	[
		simulationPreset,
		() => editorStore.activeDocumentId,
		() => editorStore.activeBranchId,
	],
	([preset]) => {
		if (!preset) {
			simulationCheck.pause()

			return
		}

		void checkSimulation()
		simulationCheck.resume()
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
		!editorStore.activeBranchId ||
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

	if (simulationPreset.value) {
		simulationTick.value++

		return
	}

	emit("loading", true)
	await fetchMetricData.refetch() // refetch is intended
	emit("loading", false)
}

// asks core whether the block's real data has shown up. Core removes the
// simulation from the block itself when it has, which reaches this editor
// as a yjs attribute change, so there is nothing to apply here.
//
// clearing is a system action rather than an edit: the real data either
// arrived or it did not, so a reader may trigger the check as much as an
// editor.
async function checkSimulation() {
	const documentId = editorStore.activeDocumentId
	const branchId = editorStore.activeBranchId

	if (
		!simulationPreset.value ||
		props.disableRefresh ||
		!documentId ||
		!branchId ||
		!props.uid
	) {
		return
	}

	try {
		await checkMetricBlockSimulation(documentId, branchId, props.uid)
	} catch {
		// a failed check just means the block keeps simulating until the
		// next tick; there is nothing to tell the reader
	}
}

function startSimulation() {
	emit("simulate", DEFAULT_SIMULATION_PRESET)
}

function openModal() {
	if (!props.uid) {
		return
	}

	editorStore.activateMetricBlockConfig(props.uid)
}
</script>
<template>
	<div class="relative flex size-full min-w-0 items-center justify-center">
		<div
			v-if="drawnPreset"
			class="pointer-events-none absolute top-2.5 left-1/2 z-1 flex -translate-x-1/2 items-center gap-1 text-2sm text-foreground/70"
		>
			<Icon name="lucide:hourglass" class="size-3" />
			{{ $t("editor.metrics.simulation.label") }}
		</div>
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
						<ShadcnUiButton
							v-if="!isEditingDisabled && canSimulate"
							variant="outline"
							size="2sm"
							@click="startSimulation"
						>
							{{
								$t(
									"editor.metrics.status.no-data-loaded.simulate-action-button",
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
				<!-- editing the block is what the simplified state is already
				inside of, but simulating it is not offered anywhere else -->
				<ShadcnUiEmptyContent v-if="!isEditingDisabled && canSimulate">
					<ShadcnUiButton variant="outline" size="2sm" @click="startSimulation">
						{{
							$t("editor.metrics.status.no-data-loaded.simulate-action-button")
						}}
					</ShadcnUiButton>
				</ShadcnUiEmptyContent>
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
							scope="global"
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
