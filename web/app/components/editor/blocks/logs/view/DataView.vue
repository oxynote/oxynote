<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import FilterChips from "./FilterChips.vue"
import Row from "./Row.vue"
import SearchControl from "./SearchControl.vue"
import VolumeStrip from "./VolumeStrip.vue"
import { useBlockVariables } from "../../variables"
import { mergeQueryLines } from "../lines"
import { withViewFilters, type LogFilter } from "../logql"
import { LogOrder, type LogConfig } from "../utils"
import { volumeBuckets } from "../volume"
import {
	RefreshInterval,
	refreshIntervalToMs,
	resolveTimeRange,
} from "~/utils/time-range"

// how long typing in a query or the search waits before the view reads
// again
const QUERY_DEBOUNCE_MS = 500
const SEARCH_DEBOUNCE_MS = 300
// how close to the end of the rows the next page starts loading
const LOAD_MORE_DISTANCE_PX = 64

const props = defineProps<{
	config: LogConfig
	uid: string
	disableRefresh?: boolean
	hideEmptyActionButton?: boolean
	headerRightPadding?: number
}>()
const emit = defineEmits<{
	(e: "configure"): void
}>()

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const blockVariables = useBlockVariables()
const { useLogLines, useLogVolume } = useLogDataSourceAPI()

const rows = useTemplateRef<HTMLElement>("rows")
const searchInput = ref("")
const search = refDebounced(searchInput, SEARCH_DEBOUNCE_MS)
const filters = ref<LogFilter[]>([])
const expandedIds = ref(new Set<string>())
const volumeVisible = ref(false)
const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
const queries = refDebounced(
	computed(() => (props.config.queries ?? []).map((q) => q.query)),
	QUERY_DEBOUNCE_MS,
)
const oldestFirst = computed(() => props.config.order === LogOrder.OldestFirst)
const refreshIntervalMs = computed(() =>
	refreshIntervalToMs(
		blockVariables.refreshInterval(props.config.refreshInterval) ??
			RefreshInterval.M5,
	),
)
const fetchLines = useLogLines(
	() => props.config.dataSourceId,
	viewParams,
	true,
)
const fetchVolume = useLogVolume(
	() => props.config.dataSourceId,
	viewParams,
	volumeVisible,
)
const volumeWindow = ref<{ from: Date; to: Date } | null>(null)
const lines = computed(() => {
	const pages = (fetchLines.data.value?.pages ?? []).filter(
		(page) => page !== null,
	)
	const lastPage = pages.at(-1)

	return mergeQueryLines(
		(lastPage?.results ?? []).map((result, index) => ({
			lines: pages.flatMap((page) => page.results[index]?.lines ?? []),
			hasMore: !!result?.cursor,
		})),
	)
})
const buckets = computed(() =>
	volumeWindow.value
		? volumeBuckets(fetchVolume.data.value ?? [], volumeWindow.value)
		: [],
)
const state = computed(() => {
	if (!props.config.dataSourceId) {
		return "data-source-not-selected"
	}

	if (!canRunLogQueries(queries.value)) {
		return "no-query"
	}

	// an older page that fails keeps the lines already shown. A changed
	// query that fails replaces the lines it was standing in for.
	if (
		fetchLines.error.value &&
		(!fetchLines.data.value || fetchLines.isPlaceholderData.value)
	) {
		return "query-error"
	}

	if (!fetchLines.data.value) {
		return "loading"
	}

	return lines.value.length ? "lines" : "no-lines"
})
const queryErrorMessage = computed(() => {
	const data = (
		fetchLines.error.value as { data?: { message?: unknown } } | null
	)?.data

	return typeof data?.message === "string"
		? data.message
		: t("editor.logs.status.query-error.description")
})
const actionIcon = computed(() =>
	isEditingDisabled.value ? "mingcute:eye-line" : "mingcute:pencil-line",
)
// an element laid over the header's right end, as the modal's time
// controls are, is as far in as the header's own buttons. The header keeps
// that inset and a gap past it.
const headerStyle = computed(() =>
	props.headerRightPadding
		? {
				paddingRight: `calc(${props.headerRightPadding / 16}rem + 0.625rem)`,
			}
		: undefined,
)

const { arrivedState } = useScroll(rows, {
	offset: { top: LOAD_MORE_DISTANCE_PX, bottom: LOAD_MORE_DISTANCE_PX },
})

useIntervalFn(() => void refreshData(), refreshIntervalMs, {
	immediate: true,
	immediateCallback: false,
})

onMounted(() => {
	void refreshData()
})

// older lines load at the end the rows end on, which is the top when the
// oldest come first. Rows too few to scroll sit at that end already, so
// pages load until they fill the view. A failed load stops it, or the
// next page would be asked for again at once, and without end.
watch(
	[
		() => (oldestFirst.value ? arrivedState.top : arrivedState.bottom),
		() => lines.value.length,
		() => fetchLines.isLoading.value,
	],
	([arrived, , loading]) => {
		if (
			arrived &&
			!loading &&
			fetchLines.hasNextPage.value &&
			fetchLines.status.value !== "error"
		) {
			void fetchLines.loadNextPage()
		}
	},
)

// the strip bins the counts across the window they were read from
watchImmediate(
	() => fetchVolume.data.value,
	() => {
		const timeRange = blockVariables.timeRange(props.config.timeRange)
		volumeWindow.value = timeRange ? resolveTimeRange(timeRange) : null
	},
)

// the block's queries narrowed by this view's filters and search, or null
// while a query or a variable it needs is missing
function viewParams(): LogQueriesParams | null {
	const timeRange = blockVariables.timeRange(props.config.timeRange)
	const variableValues = blockVariables.queryValues(queries.value)

	if (!timeRange || !variableValues || !canRunLogQueries(queries.value)) {
		return null
	}

	const window = resolveTimeRange(timeRange)

	return {
		queries: queries.value.map((q) =>
			withViewFilters(q, filters.value, search.value),
		),
		from: window.from,
		to: window.to,
		timeRangeKey: timeRange,
		variableValues: variableValues,
	}
}

// a key keeps one filter, so a new value for it replaces the old one
function addFilter(filter: LogFilter) {
	filters.value = [...filters.value.filter((f) => f.key !== filter.key), filter]
}

function setExpanded(id: string, expanded: boolean) {
	const next = new Set(expandedIds.value)

	if (expanded) {
		next.add(id)
	} else {
		next.delete(id)
	}

	expandedIds.value = next
}

async function refreshData() {
	if (
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId ||
		props.disableRefresh ||
		!viewParams()
	) {
		return
	}

	// a block remounts when it is dragged, and its refresh interval runs
	// per mount. The store tracks when the block is next due, so a
	// remount does not read again.
	if (!editorStore.isLogBlockDueForRefresh(props.uid)) {
		return
	}

	editorStore.setLogBlockNextRefreshTimestamp(
		props.uid,
		refreshIntervalMs.value,
	)

	// the first read starts by itself once the query can run
	if (!fetchLines.data.value) {
		return
	}

	await Promise.all([
		fetchLines.refetch(),
		volumeVisible.value ? fetchVolume.refetch() : null,
	])
}
</script>
<template>
	<div class="@container flex size-full min-w-0 flex-col">
		<div
			class="flex h-8 shrink-0 items-center gap-1 pt-1 pr-1.5 pl-2.5"
			:style="headerStyle"
		>
			<div class="min-w-0 flex-1 truncate text-2base text-foreground">
				{{ props.config.title }}
			</div>
			<SearchControl v-model="searchInput" />
			<ShadcnUiButton
				variant="ghost-plain"
				size="icon-sm"
				class="size-6"
				:aria-pressed="volumeVisible"
				@click="volumeVisible = !volumeVisible"
			>
				<Icon
					:name="
						volumeVisible
							? 'mingcute:layout-top-close-line'
							: 'mingcute:layout-top-open-line'
					"
				/>
				<span class="sr-only">
					{{
						volumeVisible
							? $t("editor.logs.view.hide-volume")
							: $t("editor.logs.view.show-volume")
					}}
				</span>
			</ShadcnUiButton>
			<slot name="actions" />
		</div>
		<FilterChips v-if="filters.length" v-model="filters" class="pt-1" />
		<VolumeStrip
			v-if="volumeVisible && volumeWindow"
			:buckets="buckets"
			:window="volumeWindow"
		/>
		<div
			ref="rows"
			:class="
				cn(
					'flex min-h-0 flex-1 overflow-y-auto',
					oldestFirst ? 'flex-col-reverse' : 'flex-col',
				)
			"
		>
			<template v-if="state === 'lines'">
				<Row
					v-for="line in lines"
					:key="line.id"
					:line="line"
					:timestamp-format="props.config.timestampFormat"
					:search="search"
					:expanded="expandedIds.has(line.id)"
					:class="
						cn(
							'shrink-0',
							oldestFirst
								? 'border-t last:border-t-0'
								: 'border-b last:border-b-0',
							expandedIds.has(line.id)
								? 'border-border'
								: 'border-muted-highlight',
						)
					"
					@update:expanded="(expanded) => setExpanded(line.id, expanded)"
					@filter="addFilter"
				/>
				<div
					v-if="fetchLines.isLoading.value && fetchLines.hasNextPage.value"
					class="flex shrink-0 items-center justify-center py-2 text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-5 opacity-50"
					/>
				</div>
			</template>
			<div
				v-else-if="state === 'loading'"
				class="flex size-full items-center justify-center text-muted-foreground"
			>
				<Icon name="svg-spinners:blocks-shuffle-3" class="size-5 opacity-50" />
			</div>
			<div
				v-else
				class="flex size-full items-center justify-center text-foreground"
			>
				<ShadcnUiEmpty>
					<ShadcnUiEmptyHeader>
						<ShadcnUiEmptyMedia variant="icon" class="size-9">
							<Icon :name="BLOCK_ICONS.logBlock" class="size-6" />
						</ShadcnUiEmptyMedia>
						<ShadcnUiEmptyTitle>
							{{ $t(`editor.logs.status.${state}.title`) }}
						</ShadcnUiEmptyTitle>
						<ShadcnUiEmptyDescription>
							{{
								state === "query-error"
									? queryErrorMessage
									: $t(`editor.logs.status.${state}.description`)
							}}
						</ShadcnUiEmptyDescription>
					</ShadcnUiEmptyHeader>
					<ShadcnUiEmptyContent
						v-if="
							!props.hideEmptyActionButton &&
							(state === 'data-source-not-selected' || state === 'no-query')
						"
					>
						<ShadcnUiButton size="2sm" @click="emit('configure')">
							<Icon :name="actionIcon" class="size-3.5" />
							{{
								!isEditingDisabled
									? $t(`editor.logs.status.${state}.normal-action-button`)
									: $t(`editor.logs.status.${state}.readonly-action-button`)
							}}
						</ShadcnUiButton>
					</ShadcnUiEmptyContent>
				</ShadcnUiEmpty>
			</div>
		</div>
	</div>
</template>
