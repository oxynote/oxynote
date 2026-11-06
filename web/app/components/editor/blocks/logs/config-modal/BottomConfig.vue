<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import type { LogConfig, LogQuery } from "../utils"
import ConfigField from "../../config-modal-utils/ConfigField.vue"
import QueryEditor from "../../config-modal-utils/query/QueryEditor.vue"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { useBlockVariables } from "../../variables"

interface QueryEntry {
	query: LogQuery
	diffStatus: DiffStatus | null
	// the query's old version, shown under it, when its text changed
	oldQuery: LogQuery | null
	key: string
}

const config = defineModel<LogConfig>({ required: true })
const props = defineProps<{
	diffStatus?: DiffStatus | null
	oldConfig?: LogConfig | null
}>()

const { t } = useI18n({ useScope: "global" })
const { isEditable } = useEditorMeta()
const blockVariables = useBlockVariables()
const editorStore = useEditorStore()

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
// the query editor looks up its completions over the block's own time range
const timeRange = computed(() =>
	blockVariables.timeRange(config.value.timeRange),
)
const areQueriesModified = computed(
	() =>
		props.diffStatus === DiffStatus.Modified &&
		!!props.oldConfig &&
		jsonStableStringify(props.oldConfig.queries) !==
			jsonStableStringify(config.value.queries),
)
// a changed query shows as one entry, its new version above its old one.
// A query only one side has shows as added or removed. So an entry's index
// is its query's index wherever an edit uses it.
const entries = computed<QueryEntry[]>(() => {
	const newQueries = config.value.queries ?? []

	if (!areQueriesModified.value) {
		return newQueries.map((query, i) => ({
			query: query,
			diffStatus: null,
			oldQuery: null,
			key: `query-${i}`,
		}))
	}

	const oldQueries = props.oldConfig?.queries ?? []

	return Array.from(
		{ length: Math.max(newQueries.length, oldQueries.length) },
		(_, i) => i,
	).flatMap((i): QueryEntry[] => {
		const newQuery = newQueries[i]
		const oldQuery = oldQueries[i]

		if (!newQuery) {
			return oldQuery
				? [
						{
							query: oldQuery,
							diffStatus: DiffStatus.Removed,
							oldQuery: null,
							key: `old-${i}`,
						},
					]
				: []
		}

		const changed = newQuery.query !== oldQuery?.query

		return [
			{
				query: newQuery,
				diffStatus: changed ? DiffStatus.Added : null,
				oldQuery: changed && oldQuery ? oldQuery : null,
				key: `new-${i}`,
			},
		]
	})
})
const showsQueryNames = computed(() => entries.value.length > 1)

function queryName(index: number) {
	return t("editor.logs.config.query-name-default-format", {
		index: index + 1,
	})
}

function updateQuery(index: number, value: string) {
	// a new array lets the node attribute change
	config.value.queries =
		config.value.queries?.map((q, i) =>
			i === index ? { ...q, query: value } : q,
		) ?? null
}

function addQuery() {
	const queries = config.value.queries ?? []

	config.value.queries = [
		...queries,
		{ name: queryName(queries.length), query: "" },
	]
}

function deleteQuery(index: number) {
	if (!config.value.queries || config.value.queries.length <= 1) {
		return
	}

	config.value.queries = config.value.queries
		.filter((_, i) => i !== index)
		.map((q, i) => ({ ...q, name: queryName(i) }))
}
</script>
<template>
	<div class="flex w-full flex-col gap-2 py-1.75">
		<div
			v-for="(entry, index) in entries"
			:key="entry.key"
			class="flex flex-col px-1.75"
		>
			<div
				v-if="showsQueryNames"
				class="flex items-center justify-between gap-2 self-stretch"
			>
				<div class="text-2base font-medium text-muted-foreground">
					{{ entry.query.name }}
				</div>
				<ShadcnUiButton
					v-if="!isEditingDisabled"
					variant="ghost-plain"
					size="icon-sm"
					class="size-5 text-muted-foreground! hover:text-muted-foreground/70! active:text-muted-foreground/90!"
					@click="deleteQuery(index)"
				>
					<Icon name="lucide:trash-2" />
					<span class="sr-only">
						{{ $t("editor.logs.config.delete-query-button") }}
					</span>
				</ShadcnUiButton>
			</div>
			<div
				:class="
					cn(
						showsQueryNames &&
							'mt-0.75 ml-1.25 flex flex-col gap-2 border-l pt-0.75 pb-1 pl-3',
					)
				"
			>
				<ConfigField>
					<template #label>
						{{ $t("editor.logs.config.query-label") }}
					</template>
					<QueryEditor
						:model-value="entry.query.query"
						:data-source-id="config.dataSourceId"
						:time-range="timeRange"
						:diff-status="entry.diffStatus"
						@update:model-value="(v: string) => updateQuery(index, v)"
					/>
					<QueryEditor
						v-if="entry.oldQuery"
						:model-value="entry.oldQuery.query"
						:data-source-id="config.dataSourceId"
						:time-range="timeRange"
						:diff-status="DiffStatus.Removed"
					/>
				</ConfigField>
			</div>
		</div>
		<ShadcnUiButton
			v-if="!isEditingDisabled"
			variant="dim"
			size="custom"
			class="mb-0.75 gap-1 text-2sm"
			@click="addQuery"
		>
			<Icon name="lucide:circle-plus" />
			{{ $t("editor.logs.config.add-query-button") }}
		</ShadcnUiButton>
	</div>
</template>
