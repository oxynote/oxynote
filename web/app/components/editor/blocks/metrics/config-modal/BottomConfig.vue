<script lang="ts" setup>
import type { MetricConfig } from "../utils"
import LegendEditor from "./query/LegendEditor.vue"
import QueryEditor from "./query/QueryEditor.vue"
import ConfigField from "../ConfigField.vue"
import { cn } from "~/lib/utils"
import HelpPopoverContent from "./query-help/HelpPopoverContent.vue"
import { DiffStatus } from "~/components/editor/diff/position-map"

type QueryEntry = NonNullable<MetricConfig["queries"]>[number]

interface DiffQueryEntry {
	newQuery: QueryEntry | null
	oldQuery: QueryEntry | null
	queryChanged: boolean
	legendChanged: boolean
	diffStatus: DiffStatus
	key: string
}

const metricConfig = defineModel<MetricConfig>({ required: true })
const props = defineProps<{
	diffStatus?: DiffStatus | null
	oldConfig?: MetricConfig | null
}>()
const { t } = useI18n({ useScope: "global" })
const { isEditable } = useEditorMeta()
const { fetchDataSources } = useDataSourceAPI()
const editorStore = useEditorStore()

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
const dataSourceType = computed(() => {
	const ds = fetchDataSources.state.value.data?.find(
		(ds) => ds.id === metricConfig.value.dataSourceId,
	)
	return ds?.type ?? null
})
const isDiffModified = computed(() => props.diffStatus === DiffStatus.Modified)
const areQueriesModified = computed(() => {
	return (
		isDiffModified.value &&
		!!props.oldConfig &&
		jsonStableStringify(props.oldConfig.queries) !==
			jsonStableStringify(metricConfig.value.queries)
	)
})
const diffQueryEntries = computed<DiffQueryEntry[]>(() => {
	if (!areQueriesModified.value || !props.oldConfig) {
		return []
	}

	const newArr = metricConfig.value.queries ?? []
	const oldArr = props.oldConfig.queries ?? []
	const maxLen = Math.max(newArr.length, oldArr.length)
	const entries: DiffQueryEntry[] = []

	for (let i = 0; i < maxLen; i++) {
		const newQ = newArr[i] ?? null
		const oldQ = oldArr[i] ?? null

		if (newQ && oldQ) {
			const queryChanged = newQ.query !== oldQ.query
			const legendChanged = newQ.legendFormat !== oldQ.legendFormat
			entries.push({
				newQuery: newQ,
				oldQuery: queryChanged || legendChanged ? oldQ : null,
				queryChanged,
				legendChanged,
				diffStatus:
					queryChanged || legendChanged
						? DiffStatus.Modified
						: DiffStatus.Unchanged,
				key: queryChanged || legendChanged ? `modified-${i}` : `unchanged-${i}`,
			})
		} else if (newQ) {
			entries.push({
				newQuery: newQ,
				oldQuery: null,
				queryChanged: false,
				legendChanged: false,
				diffStatus: DiffStatus.Added,
				key: `added-${i}`,
			})
		} else if (oldQ) {
			entries.push({
				newQuery: null,
				oldQuery: oldQ,
				queryChanged: false,
				legendChanged: false,
				diffStatus: DiffStatus.Removed,
				key: `removed-${i}`,
			})
		}
	}

	return entries
})

function updateQueryField(
	index: number,
	field: "query" | "legendFormat",
	value: string,
) {
	// we need to create a new array/object reference to ensure reactivity,
	// since we're updating a nested field inside the queries array
	metricConfig.value.queries =
		metricConfig.value.queries?.map((q, i) =>
			i === index ? { ...q, [field]: value } : q,
		) ?? null
}

function addNewQuery() {
	// we need to create a new array/object reference to ensure reactivity,
	// since we're updating a nested field inside the queries array
	metricConfig.value.queries = [
		...(metricConfig.value.queries || []),
		{
			name: t("editor.metrics.config.query-name-default-format", {
				index: (metricConfig.value.queries?.length ?? 0) + 1,
			}),
			query: "",
			legendFormat: "",
		},
	]
}

function deleteQuery(index: number) {
	if (!metricConfig.value.queries || metricConfig.value.queries.length <= 1) {
		return
	}

	metricConfig.value.queries = metricConfig.value.queries
		.filter((_, i) => i !== index)
		.map((q, i) => ({
			...q,
			name: t("editor.metrics.config.query-name-default-format", {
				index: i + 1,
			}),
		}))
}
</script>
<template>
	<ShadcnUiPopover>
		<div class="flex w-full flex-col gap-2 py-1.75">
			<div
				v-if="
					metricConfig.queries?.length === 1 &&
					(!areQueriesModified || (props.oldConfig?.queries?.length ?? 0) <= 1)
				"
				class="flex flex-col gap-2 pb-0.75"
			>
				<div class="px-1.75">
					<ConfigField>
						<template #label>
							<div class="flex items-center justify-between">
								<div>
									{{ $t("editor.metrics.config.query-label") }}
								</div>
								<ShadcnUiPopoverTrigger as-child>
									<ShadcnUiButton variant="dim" size="custom" class="text-2xs">
										{{
											$t("editor.metrics.config.query-explanation-button-label")
										}}
									</ShadcnUiButton>
								</ShadcnUiPopoverTrigger>
							</div>
						</template>
						<QueryEditor
							:model-value="metricConfig.queries[0]!.query"
							:data-source-id="metricConfig.dataSourceId"
							:time-range="metricConfig.timeRange"
							:diff-status="
								diffQueryEntries[0]?.queryChanged ? DiffStatus.Added : null
							"
							@update:model-value="
								(v: string) => updateQueryField(0, 'query', v)
							"
						/>
						<QueryEditor
							v-if="
								diffQueryEntries[0]?.queryChanged &&
								diffQueryEntries[0].oldQuery
							"
							:model-value="diffQueryEntries[0].oldQuery.query"
							:data-source-id="metricConfig.dataSourceId"
							:time-range="metricConfig.timeRange"
							:diff-status="DiffStatus.Removed"
						/>
					</ConfigField>
				</div>
				<div class="px-1.75">
					<ConfigField>
						<template #label>
							<span>
								{{ $t("editor.metrics.config.legend-format-label") }}
							</span>
						</template>
						<LegendEditor
							:model-value="metricConfig.queries[0]!.legendFormat"
							:data-source-id="metricConfig.dataSourceId"
							:time-range="metricConfig.timeRange"
							:query="metricConfig.queries[0]!.query"
							:diff-status="
								diffQueryEntries[0]?.legendChanged ? DiffStatus.Added : null
							"
							@update:model-value="
								(v: string) => updateQueryField(0, 'legendFormat', v)
							"
						/>
						<LegendEditor
							v-if="
								diffQueryEntries[0]?.legendChanged &&
								diffQueryEntries[0].oldQuery
							"
							:model-value="diffQueryEntries[0].oldQuery.legendFormat"
							:data-source-id="metricConfig.dataSourceId"
							:time-range="metricConfig.timeRange"
							:query="diffQueryEntries[0].oldQuery.query"
							:diff-status="DiffStatus.Removed"
						/>
					</ConfigField>
				</div>
			</div>
			<template v-else>
				<!-- diff mode with modifications: iterate diffQueryEntries to include removed entries -->
				<template v-if="areQueriesModified">
					<div
						v-for="entry in diffQueryEntries"
						:key="entry.key"
						class="flex flex-col px-1.75"
					>
						<!-- added entry: show both fields with Added status, no old duplicates -->
						<template
							v-if="entry.diffStatus === DiffStatus.Added && entry.newQuery"
						>
							<div class="flex items-center justify-between gap-2 self-stretch">
								<div class="text-2base font-medium text-muted-foreground">
									{{ entry.newQuery.name }}
								</div>
							</div>
							<div
								class="mt-0.75 ml-1.25 flex flex-col gap-2 border-l pt-0.75 pb-1 pl-3"
							>
								<ConfigField>
									<template #label>
										{{ $t("editor.metrics.config.query-label") }}
									</template>
									<QueryEditor
										:model-value="entry.newQuery.query"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:diff-status="DiffStatus.Added"
									/>
								</ConfigField>
								<ConfigField>
									<template #label>
										{{ $t("editor.metrics.config.legend-format-label") }}
									</template>
									<LegendEditor
										:model-value="entry.newQuery.legendFormat"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:query="entry.newQuery.query"
										:diff-status="DiffStatus.Added"
									/>
								</ConfigField>
							</div>
						</template>

						<!-- removed entry: show both fields with Removed status -->
						<template
							v-else-if="
								entry.diffStatus === DiffStatus.Removed && entry.oldQuery
							"
						>
							<div class="flex items-center gap-2 self-stretch">
								<div class="text-2base font-medium text-muted-foreground">
									{{ entry.oldQuery.name }}
								</div>
							</div>
							<div
								class="mt-0.75 ml-1.25 flex flex-col gap-2 border-l pt-0.75 pb-1 pl-3"
							>
								<ConfigField>
									<template #label>
										{{ $t("editor.metrics.config.query-label") }}
									</template>
									<QueryEditor
										:model-value="entry.oldQuery.query"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:diff-status="DiffStatus.Removed"
									/>
								</ConfigField>
								<ConfigField>
									<template #label>
										{{ $t("editor.metrics.config.legend-format-label") }}
									</template>
									<LegendEditor
										:model-value="entry.oldQuery.legendFormat"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:query="entry.oldQuery.query"
										:diff-status="DiffStatus.Removed"
									/>
								</ConfigField>
							</div>
						</template>

						<!-- unchanged or modified: show current fields with per-field diff -->
						<template v-else-if="entry.newQuery">
							<div class="flex items-center justify-between gap-2 self-stretch">
								<div class="text-2base font-medium text-muted-foreground">
									{{ entry.newQuery.name }}
								</div>
							</div>
							<div
								class="mt-0.75 ml-1.25 flex flex-col gap-2 border-l pt-0.75 pb-1 pl-3"
							>
								<ConfigField>
									<template #label>
										<div class="flex items-center justify-between">
											<div>
												{{ $t("editor.metrics.config.query-label") }}
											</div>
											<ShadcnUiPopoverTrigger as-child>
												<ShadcnUiButton
													variant="dim"
													size="custom"
													class="text-2xs"
												>
													{{
														$t(
															"editor.metrics.config.query-explanation-button-label",
														)
													}}
												</ShadcnUiButton>
											</ShadcnUiPopoverTrigger>
										</div>
									</template>
									<QueryEditor
										:model-value="entry.newQuery.query"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:diff-status="entry.queryChanged ? DiffStatus.Added : null"
									/>
									<QueryEditor
										v-if="entry.queryChanged && entry.oldQuery"
										:model-value="entry.oldQuery.query"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:diff-status="DiffStatus.Removed"
									/>
								</ConfigField>
								<ConfigField>
									<template #label>
										{{ $t("editor.metrics.config.legend-format-label") }}
									</template>
									<LegendEditor
										:model-value="entry.newQuery.legendFormat"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:query="entry.newQuery.query"
										:diff-status="entry.legendChanged ? DiffStatus.Added : null"
									/>
									<LegendEditor
										v-if="entry.legendChanged && entry.oldQuery"
										:model-value="entry.oldQuery.legendFormat"
										:data-source-id="metricConfig.dataSourceId"
										:time-range="metricConfig.timeRange"
										:query="entry.oldQuery.query"
										:diff-status="DiffStatus.Removed"
									/>
								</ConfigField>
							</div>
						</template>
					</div>
				</template>

				<!-- no diff: normal multi-query rendering -->
				<template v-else>
					<div
						v-for="(query, index) in metricConfig.queries"
						:key="index"
						class="flex flex-col px-1.75"
					>
						<div class="flex items-center justify-between gap-2 self-stretch">
							<div class="text-2base font-medium text-muted-foreground">
								{{ query.name }}
							</div>
							<ShadcnUiButton
								v-if="!isEditingDisabled"
								variant="ghost-plain"
								size="icon-sm"
								:class="
									cn(
										'size-5 text-muted-foreground! hover:text-muted-foreground/70! active:text-muted-foreground/90!',
									)
								"
								@click="deleteQuery(index)"
							>
								<Icon name="lucide:trash-2" />
							</ShadcnUiButton>
						</div>
						<div
							class="mt-0.75 ml-1.25 flex flex-col gap-2 border-l pt-0.75 pb-1 pl-3"
						>
							<ConfigField>
								<template #label>
									<div class="flex items-center justify-between">
										<div>
											{{ $t("editor.metrics.config.query-label") }}
										</div>
										<ShadcnUiPopoverTrigger as-child>
											<ShadcnUiButton
												variant="dim"
												size="custom"
												class="text-2xs"
											>
												{{
													$t(
														"editor.metrics.config.query-explanation-button-label",
													)
												}}
											</ShadcnUiButton>
										</ShadcnUiPopoverTrigger>
									</div>
								</template>
								<QueryEditor
									:model-value="query.query"
									:data-source-id="metricConfig.dataSourceId"
									:time-range="metricConfig.timeRange"
									@update:model-value="
										(v: string) => updateQueryField(index, 'query', v)
									"
								/>
							</ConfigField>
							<ConfigField>
								<template #label>
									<span>
										{{ $t("editor.metrics.config.legend-format-label") }}
									</span>
								</template>
								<LegendEditor
									:model-value="query.legendFormat"
									:data-source-id="metricConfig.dataSourceId"
									:time-range="metricConfig.timeRange"
									:query="query.query"
									@update:model-value="
										(v: string) => updateQueryField(index, 'legendFormat', v)
									"
								/>
							</ConfigField>
						</div>
					</div>
				</template>
			</template>
			<ShadcnUiButton
				v-if="!isEditingDisabled"
				variant="dim"
				size="custom"
				class="mb-0.75 gap-1 text-2sm"
				@click="addNewQuery"
			>
				<Icon name="lucide:circle-plus" />
				{{ $t("editor.metrics.config.add-query-button") }}
			</ShadcnUiButton>
		</div>
		<ShadcnUiPopoverContent
			v-if="dataSourceType"
			class="flex w-130 max-w-[85dvw] flex-col px-2 pt-1 pb-1.5 text-2sm leading-4.5 md:max-w-[50dvw]"
			side="bottom"
			side-flip
			inside-modal
			align="end"
		>
			<HelpPopoverContent :data-source-type="dataSourceType" />
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
</template>
