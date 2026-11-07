<script lang="ts" setup>
import type { DynamicScrollerExposed } from "vue-virtual-scroller"
import { cn } from "#layers/ui/app/lib/utils"
import FilterChips from "./FilterChips.vue"
import Row from "./Row.vue"
import SearchControl from "./SearchControl.vue"
import VolumeStrip from "./VolumeStrip.vue"
import { useBlockVariables } from "../../variables"
import { mergeQueryLogs } from "../rows"
import { LineFormat, type LogDialect, type LogFilter } from "../dialect"
import { lokiDialect } from "../loki"
import type { LogConfig } from "../utils"
import { volumeBuckets } from "../volume"
import {
	RefreshInterval,
	refreshIntervalToMs,
	resolveTimeRange,
} from "~/utils/time-range"

// the scroller reads window when it is imported, so it loads only in the
// browser
const DynamicScroller = defineAsyncComponent(() =>
	import("vue-virtual-scroller").then((m) => m.DynamicScroller),
)
const DynamicScrollerItem = defineAsyncComponent(() =>
	import("vue-virtual-scroller").then((m) => m.DynamicScrollerItem),
)

// how long typing in a query or the search waits before the view reads
// again
const QUERY_DEBOUNCE_MS = 500
const SEARCH_DEBOUNCE_MS = 300
// how close to either end of the loaded rows the reader must get before the
// next page loads. The same distance also counts as being at the live end.
// It assumes a page arrives before the reader scrolls past this many rows.
// If it arrives later, the reader waits at the end for a moment.
const LOAD_MORE_ROWS = 20
// the most logs the list keeps. When the reader scrolls one way, logs are
// dropped from the other end. They load again when the reader scrolls back.
// A demo line takes about 400 bytes once parsed, so a full list holds
// about 2 MiB. Longer lines cost more in proportion. A block holds this
// many only once the reader has scrolled deep into it.
const MAX_LOADED_LOGS = 5000
// how many logs one request reads for each query. A block shows about ten
// rows, so a page fills it many times over. A demo line is about 300 bytes
// of JSON, and responses are not compressed, so a page is about 30 KB.
const PAGE_LOGS = 100
// the height of a closed row, which is h-8 and its bottom border. The
// scroller uses it for every row until it has measured the row. So a
// wrong value moves the rows once they are measured.
const ROW_HEIGHT_PX = 33
// how far past the viewport a block still counts as near, in screens.
// A block further away does not read. It assumes a read lands before the
// reader scrolls one screen. After a faster scroll, the block shows its
// spinner for a moment.
const NEAR_VIEWPORT_MARGIN = "100% 0%"

const props = defineProps<{
	config: LogConfig
	uid: string
	disableRefresh?: boolean
	hideEmptyActionButton?: boolean
	hideSearch?: boolean
	headerClass?: string
	headerRightPadding?: number
	compact?: boolean
}>()
const emit = defineEmits<{
	(e: "configure"): void
}>()

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const blockVariables = useBlockVariables()
const { useLogs, useLogVolume } = useLogDataSourceAPI()
const { fetchDataSources } = useDataSourceAPI()

const root = useTemplateRef<HTMLDivElement>("root")
const scroller = useTemplateRef<
	ComponentPublicInstance & DynamicScrollerExposed<Log>
>("scroller")
// a block more than a screen away from the viewport reads nothing. It
// reads its first logs, or the logs of a changed query, once it comes
// within a screen. Its refresh timer keeps running, but it skips the
// reads. Once the block comes near, it reads the refresh it skipped. So
// a page with many blocks reads only the blocks near the reader.
const near = useElementVisibility(root, { rootMargin: NEAR_VIEWPORT_MARGIN })
const searchInput = ref("")
const search = refDebounced(searchInput, SEARCH_DEBOUNCE_MS)
const filters = ref<LogFilter[]>([])
const expandedIds = ref(new Set<string>())
const volumeVisible = ref(false)
// the first and last rows the scroller shows. They tell when the next page
// should load.
const firstVisibleRow = ref(0)
const lastVisibleRow = ref(-1)
const loadingPrevious = ref(false)
// set when a refresh had to wait, either for the reader or for a page
const missedRefresh = ref(false)
// set when a refresh came due while the block was away from the viewport
const refreshWhenNear = ref(false)
const oldestFirst = computed(() => props.config.order === LogOrder.Asc)
const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
const queries = refDebounced(
	computed(() => (props.config.queries ?? []).map((q) => q.query)),
	QUERY_DEBOUNCE_MS,
)
// the query language of the block's data source, or null until the
// source is known
const dialect = computed<LogDialect | null>(() => {
	const source = fetchDataSources.state.value.data?.find(
		(s) => s.id === props.config.dataSourceId,
	)

	return source?.type === DataSourceType.Loki ? lokiDialect : null
})
// a filter on a key read from a line runs on every query, so it needs
// every query to let it through
const lineFilterFormats = computed(() =>
	Object.values(LineFormat).filter((format) =>
		queries.value.every((q) => dialect.value?.canFilterLineKey(q, format)),
	),
)
const refreshIntervalMs = computed(() =>
	refreshIntervalToMs(
		blockVariables.refreshInterval(props.config.refreshInterval) ??
			RefreshInterval.M5,
	),
)
const fetchLogs = useLogs(
	() => props.config.dataSourceId,
	() => {
		const params = viewParams()

		return params && { ...params, order: props.config.order, limit: PAGE_LOGS }
	},
	near,
)
const fetchVolume = useLogVolume(
	() => props.config.dataSourceId,
	viewParams,
	() => volumeVisible.value && near.value,
)
const volumeWindow = ref<{ from: Date; to: Date } | null>(null)
const logs = computed(() => {
	const pages = (fetchLogs.data.value?.pages ?? []).filter(
		(page) => page !== null,
	)
	const lastPage = pages.at(-1)

	return mergeQueryLogs(
		(lastPage?.results ?? []).map((_, index) => {
			const results = pages.flatMap((page) => page.results[index] ?? [])

			return {
				logs: results.flatMap((result) => result.lines),
				hasMore: !!results.at(-1)?.nextCursor,
				hasPrevious: !!results[0]?.previousCursor,
			}
		}),
		props.config.order,
	)
})
const buckets = computed(() => volumeBuckets(fetchVolume.data.value ?? []))
const state = computed(() => {
	// a deleted source, or one whose queries the view cannot write, is
	// missing from the loaded list or has no dialect. So the block asks
	// for a source again.
	if (
		!props.config.dataSourceId ||
		(fetchDataSources.state.value.data && !dialect.value)
	) {
		return "data-source-not-selected"
	}

	if (!canRunLogQueries(queries.value)) {
		return "no-query"
	}

	// an older page that fails keeps the logs already shown. A changed
	// query that fails replaces the logs it was standing in for.
	if (
		fetchLogs.error.value &&
		(!fetchLogs.data.value || fetchLogs.isPlaceholderData.value)
	) {
		return "query-error"
	}

	if (!fetchLogs.data.value) {
		return "loading"
	}

	return logs.value.length ? "lines" : "no-lines"
})
// the search, the volume and the header's border serve a list of logs.
// They also stay while a search or filter is on. That way a reader whose
// search emptied the list can still change it. They stay too while the
// list is about to change: the search waits a moment after typing, and a
// changed query shows the old logs until its own arrive.
const hasControls = computed(
	() =>
		state.value === "lines" ||
		searchInput.value.trim().length > 0 ||
		search.value.trim().length > 0 ||
		filters.value.length > 0 ||
		fetchLogs.isPlaceholderData.value,
)
// the live end is where new logs join the list. Newest first, that is the top.
// Oldest first, it is the bottom.
const atLiveEnd = computed(() =>
	oldestFirst.value
		? lastVisibleRow.value >= logs.value.length - 1 - LOAD_MORE_ROWS &&
			!fetchLogs.hasNextPage.value
		: firstVisibleRow.value <= LOAD_MORE_ROWS &&
			!fetchLogs.hasPreviousPage.value,
)
const queryErrorMessage = computed(() => {
	const data = (
		fetchLogs.error.value as { data?: { message?: unknown } } | null
	)?.data

	return typeof data?.message === "string"
		? data.message
		: t("editor.logs.status.query-error.description")
})
const actionIcon = computed(() =>
	isEditingDisabled.value ? "mingcute:eye-line" : "mingcute:pencil-line",
)
// an element laid over the header's right end, as the modal's time
// controls are, is as far in as the header's own buttons. The header clears
// it, that inset and a little more, so the space before the element's first
// icon matches the space between the header's own icons.
const headerStyle = computed(() =>
	props.headerRightPadding
		? {
				paddingRight: `calc(${props.headerRightPadding / 16}rem + 0.125rem)`,
			}
		: undefined,
)

useIntervalFn(() => void refreshData(), refreshIntervalMs, {
	immediate: true,
	immediateCallback: false,
})

onMounted(() => {
	void refreshData()
})

watch(near, (isNear) => {
	if (isNear && refreshWhenNear.value) {
		void readNewData()
	}
})

// a page loads once the scroller shows a row near either end of the loaded
// rows. When there are too few rows to fill the view, all of them are shown. So
// pages keep loading until the view is full, and then they stop.
//
// A failed load stops the paging too. Otherwise the same page would be asked
// for again right away, without end.
watch(
	[
		firstVisibleRow,
		lastVisibleRow,
		() => logs.value.length,
		() => fetchLogs.isLoading.value,
	],
	([first, last, count, loading]) => {
		if (loading || fetchLogs.status.value === "error") {
			return
		}

		if (last >= count - LOAD_MORE_ROWS && fetchLogs.hasNextPage.value) {
			void loadNextPage()
		} else if (first <= LOAD_MORE_ROWS && fetchLogs.hasPreviousPage.value) {
			void loadPreviousPage()
		}
	},
)

// a refresh that had to wait runs as soon as the reader is back at the live end
// and no page is loading
watch(
	() => atLiveEnd.value && !fetchLogs.isLoading.value,
	(ready) => {
		if (ready && missedRefresh.value) {
			void refreshLogs()
		}
	},
)

// a new order reads the window from its other end, so the rows start
// again at the top
watch(
	() => props.config.order,
	() => {
		const element = scroller.value?.$el as HTMLElement | undefined

		if (element) {
			element.scrollTop = 0
		}
	},
)

// the window the counts were read from. The strip's axis spans it while
// the strip has fewer than two bars.
watchImmediate(
	() => fetchVolume.data.value,
	() => {
		const timeRange = blockVariables.timeRange(props.config.timeRange)
		volumeWindow.value = timeRange ? resolveTimeRange(timeRange) : null
	},
)

// the block's queries narrowed by this view's filters and search, or null
// while a query, a variable it needs or the source's dialect is missing
function viewParams(): LogQueriesParams | null {
	const timeRange = blockVariables.timeRange(props.config.timeRange)
	const variableValues = blockVariables.queryValues(queries.value)
	const queryDialect = dialect.value

	if (
		!timeRange ||
		!variableValues ||
		!queryDialect ||
		!canRunLogQueries(queries.value)
	) {
		return null
	}

	const window = resolveTimeRange(timeRange)

	return {
		queries: queries.value.map((q) =>
			queryDialect.withViewFilters(q, filters.value, search.value),
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

async function loadNextPage() {
	await fetchLogs.loadNextPage()
	await keepPlace(() => {
		fetchLogs.trimStart(MAX_LOADED_LOGS)
	})
}

async function loadPreviousPage() {
	loadingPrevious.value = true

	// the spinner above the rows is removed in the same change that adds the
	// rows. Otherwise its height would move the rows afterwards.
	await keepPlace(async () => {
		try {
			await fetchLogs.loadPreviousPage()
		} finally {
			loadingPrevious.value = false
		}
	})

	await keepPlace(() => {
		fetchLogs.trimEnd(MAX_LOADED_LOGS)
	})
}

// a refresh adds logs only at the live end, where the reader can see them.
// Anywhere else it waits until the reader comes back. It also waits while a
// page is loading. Otherwise the refresh would be skipped.
async function refreshLogs() {
	if (!atLiveEnd.value || fetchLogs.isLoading.value) {
		missedRefresh.value = true

		return
	}

	missedRefresh.value = false

	const element = scroller.value?.$el as HTMLElement | undefined

	// at the very top of a newest-first list, the reader follows the new logs.
	// Anywhere else, the reader keeps the row they are reading.
	if (!oldestFirst.value && element?.scrollTop === 0) {
		await fetchLogs.loadNewer()
		await nextTick()
		element.scrollTop = 0
	} else {
		await keepPlace(() => fetchLogs.loadNewer())
	}

	await keepPlace(() => {
		if (oldestFirst.value) {
			fetchLogs.trimStart(MAX_LOADED_LOGS)
		} else {
			fetchLogs.trimEnd(MAX_LOADED_LOGS)
		}
	})
}

// keepPlace runs a change to the rows. Then it scrolls the first row the reader
// saw back to where it was before. So rows added or dropped above do not move
// the view.
async function keepPlace(change: () => unknown) {
	const index = firstVisibleRow.value
	const anchor = logs.value[index]
	const list = scroller.value
	const element = list?.$el as HTMLElement | undefined
	// how far the anchor row sits below the top of the view
	const offset =
		list && element ? list.getItemOffset(index) - element.scrollTop : 0

	await change()
	await nextTick()

	const moved = anchor
		? logs.value.findIndex((log) => log.id === anchor.id)
		: -1

	if (moved >= 0 && list && element) {
		element.scrollTop = list.getItemOffset(moved) - offset
	}
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

	if (!near.value) {
		refreshWhenNear.value = true

		return
	}

	await readNewData()
}

// readNewData reads the newest logs and the volume
async function readNewData() {
	refreshWhenNear.value = false

	// a refresh waits for the query's first read. That read is still to
	// come when the block has no rows yet, or when the query changed and
	// the block still shows the old query's rows. The first read starts by
	// itself and brings the newest logs, so a refresh now would read them
	// twice.
	if (!fetchLogs.data.value || fetchLogs.isPlaceholderData.value) {
		return
	}

	await Promise.all([
		refreshLogs(),
		volumeVisible.value ? fetchVolume.refetch() : null,
	])
}
</script>
<template>
	<div ref="root" class="@container flex size-full min-w-0 flex-col">
		<div :class="cn('shrink-0', hasControls && 'border-b pb-1')">
			<!-- when compact, the icon buttons overlap their neighbours by part of
			their blank edge -->
			<div
				:class="
					cn(
						'flex h-8 shrink-0 items-center pt-1 pr-1.5 pl-2.5',
						props.headerClass,
						!props.compact && 'gap-1',
					)
				"
				:style="headerStyle"
			>
				<div class="min-w-0 flex-1 truncate text-2base text-foreground">
					{{ props.config.title }}
				</div>
				<SearchControl
					v-if="hasControls && !props.hideSearch"
					v-model="searchInput"
					:compact="props.compact"
				/>
				<slot name="marker" />
				<ShadcnUiButton
					v-if="hasControls"
					variant="ghost-plain"
					size="icon-sm"
					:class="cn('size-6', props.compact && '-mx-0.5')"
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
				v-if="hasControls && volumeVisible && volumeWindow"
				:buckets="buckets"
				:window="volumeWindow"
			/>
		</div>
		<!-- keepPlace holds the reader's row when rows change above. The
		scroller's own shift option would hold a row as well. Its hold lasts a
		moment after each change, and on a slow machine it pulls the view back
		while the reader scrolls on. -->
		<DynamicScroller
			v-if="state === 'lines'"
			ref="scroller"
			:items="logs"
			:min-item-size="ROW_HEIGHT_PX"
			key-field="id"
			emit-update
			class="min-h-0 flex-1"
			@update="
				(
					_start: number,
					_end: number,
					visibleStart: number,
					visibleEnd: number,
				) => {
					firstVisibleRow = visibleStart
					lastVisibleRow = visibleEnd
				}
			"
		>
			<template #before>
				<div
					v-if="loadingPrevious"
					class="flex items-center justify-center py-2 text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-5 opacity-50"
					/>
				</div>
			</template>
			<template #default="{ item, index, active }">
				<DynamicScrollerItem :item="item" :active="active" :data-index="index">
					<!-- the scroller reuses a row's view for another log while the
					reader scrolls. The key gives each log its own row. So an open row
					does not close and open again with an animation. -->
					<Row
						v-if="dialect"
						:key="item.id"
						:log="item"
						:timestamp-format="props.config.timestampFormat"
						:search="search"
						:dialect="dialect"
						:line-filter-formats="lineFilterFormats"
						:expanded="expandedIds.has(item.id)"
						:class="
							cn(
								index < logs.length - 1 && 'border-b',
								expandedIds.has(item.id)
									? 'border-border'
									: 'border-muted-highlight',
							)
						"
						@update:expanded="(expanded) => setExpanded(item.id, expanded)"
						@filter="addFilter"
					/>
				</DynamicScrollerItem>
			</template>
			<template #after>
				<div
					v-if="
						fetchLogs.isLoading.value &&
						fetchLogs.hasNextPage.value &&
						!loadingPrevious
					"
					class="flex items-center justify-center py-2 text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-5 opacity-50"
					/>
				</div>
			</template>
		</DynamicScroller>
		<div v-else class="flex min-h-0 flex-1 flex-col">
			<div
				v-if="state === 'loading'"
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
									: state === "no-lines" && search.trim()
										? $t("editor.logs.status.no-lines.search-description")
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
