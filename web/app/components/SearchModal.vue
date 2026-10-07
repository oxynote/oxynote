<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type {
	DocumentSearchDocument,
	DocumentSearchHit,
	DocumentSearchResult,
} from "~/utils"
import {
	documentTreeBreadcrumbs,
	documentTreeByLevel,
	extractDocumentTreeElement,
} from "./sidebar"
import { showToastMessage } from "./toast"

interface MoreHits {
	hits: DocumentSearchHit[]
	totalHits: number
	nextToken: string | null
}

// a page the list can show and open. One taken from the tree has no
// record of when it was changed or by whom.
type ListedDocument = Omit<DocumentSearchDocument, "updatedAt" | "updatedBy">

interface Row {
	document: ListedDocument
	href: string
	// set on the row that loads the next hits of this result
	more: DocumentSearchResult | null
}

const open = defineModel<boolean>({ required: true })

const { t, d } = useI18n({ useScope: "global" })
const {
	useSearchDocuments,
	searchDocumentBranch,
	useFetchRecentDocuments,
	recordBranchView,
	fetchDocumentTree,
} = useDocumentAPI()
const { fetchAuthSession, fetchOrganization } = useAuthSession()
const { osType } = useDetectHost()
const editorStore = useEditorStore()

const ROW_CLASS =
	"rounded-md no-underline active:bg-accent [@media(hover:hover)]:data-[selected]:bg-accent/50 [@media(hover:hover)]:data-[selected]:active:bg-accent"
const TOUCH_HIDDEN_CLASS = "[@media(hover:none),(max-width:30rem)]:hidden"
const CLOSE_KEY = "esc"
const START_LIST_MIN_LENGTH = 5
// keys drawn as icons. The enter character sits low in its key cap, and
// the arrows match its weight this way.
const KEY_ICONS: Record<string, string> = {
	"↑": "lucide:arrow-up",
	"↓": "lucide:arrow-down",
	"↵": "lucide:corner-down-left",
}

const searchQuery = ref("")
const trimmedSearchQuery = computed(() => searchQuery.value.trim())
const debouncedSearchQuery = refDebounced(trimmedSearchQuery, 300)
const isTyping = computed(
	() => trimmedSearchQuery.value !== debouncedSearchQuery.value,
)
// a query the search would refuse as too short counts as no query at all
const isSearchable = computed(
	() => trimmedSearchQuery.value.length >= DOCUMENT_SEARCH_QUERY_MIN_LENGTH,
)
const searchedQuery = computed(() =>
	debouncedSearchQuery.value.length >= DOCUMENT_SEARCH_QUERY_MIN_LENGTH
		? debouncedSearchQuery.value
		: "",
)
const search = useSearchDocuments(() => ({
	q: searchedQuery.value,
	currentDocId: editorStore.activeDocumentId,
}))
// loaded once the page is up and again after each open from here, so the
// list is already there when the modal opens. No page needs it to render,
// so the server leaves it alone.
const fetchRecentDocuments = useFetchRecentDocuments(
	"search",
	import.meta.client,
)
// the query the results on screen belong to. They stay while the next
// search loads, so this can trail the typed text.
const shownQuery = ref("")
const now = ref(new Date())
const selectedIndex = ref(-1)
const isLoadingMore = ref(false)
// the hits loaded after a result's first ones, by branch id
const moreHits = ref<Record<string, MoreHits>>({})
const loadingMoreHits = new Set<string>()
// ShadcnUiInput renders a bare <input>, so the ref resolves to the component
// instance whose $el is that element. Spelling the shape out keeps eslint's ts
// program — which cannot type .vue imports — from treating it as `any`, and
// keeps the focus fallback below type-checked
const inputElem = useTemplateRef<{
	$el?: HTMLInputElement
	focus?: () => void
}>("search-input")
const list = useTemplateRef<HTMLElement>("list")
// the next page is known a moment before the first page lands. A load
// started then would drop that page and fetch it again, so the search has
// to have succeeded first, with its own results and not the previous
// ones. A failed load also leaves the list at its end, and would
// otherwise be repeated at once, and without end.
useInfiniteScroll(list, handleLoadMore, {
	distance: 100,
	canLoadMore: () =>
		search.hasNextPage.value &&
		search.status.value === "success" &&
		!search.isPlaceholderData.value,
})

const results = computed(() => {
	const seen = new Set<string>()

	// the index can change between two page loads, so the next page can
	// repeat a branch
	return (search.data.value?.pages ?? [])
		.flatMap((page) => page.results)
		.filter((result) => {
			if (seen.has(result.document.branch.id)) {
				return false
			}

			seen.add(result.document.branch.id)

			return true
		})
})
const groups = computed(() => {
	let index = 0

	return results.value.map((result) => {
		const more = moreHits.value[result.document.branch.id]
		const hits = more ? [...result.hits, ...more.hits] : result.hits
		const hasMoreHits = !!(more ? more.nextToken : result.nextHitsToken)
		const group = {
			result: result,
			href: documentHref(result.document),
			breadcrumbs: documentBreadcrumbs(result.document),
			meta: updatedMeta(result.document),
			metaTooltip: updatedTooltip(result.document),
			branchLabel: branchLabel(result.document),
			current: isCurrentDocument(result.document),
			hits: hits.map((hit) => ({
				hit: hit,
				href: documentHref(result.document, hit),
			})),
			hasMoreHits: hasMoreHits,
			remainingHits: (more?.totalHits ?? result.totalHits) - hits.length,
			// where the group's first row sits among all rows
			index: index,
		}

		index += 1 + hits.length + (hasMoreHits ? 1 : 0)

		return group
	})
})
const recents = computed(() => {
	const viewed = fetchRecentDocuments.data.value?.results ?? []

	return [
		...viewed.map((document) => ({
			document: document,
			meta: relativeTimeLabel(document.viewedAt, now.value, t, d),
			metaTooltip: t("sidebar.search.viewed-tooltip", {
				date: d(new Date(document.viewedAt), "short-with-time"),
			}),
		})),
		...suggestedDocuments(viewed).map((document) => ({
			document: document,
			meta: t("sidebar.search.suggested"),
			metaTooltip: t("sidebar.search.suggested-tooltip"),
		})),
	].map(({ document, meta, metaTooltip }) => ({
		document: document,
		href: documentHref(document),
		breadcrumbs: documentBreadcrumbs(document),
		meta: meta,
		metaTooltip: metaTooltip,
		branchLabel: branchLabel(document),
		current: isCurrentDocument(document),
	}))
})
const isSearching = computed(
	() =>
		isSearchable.value &&
		(isTyping.value ||
			search.status.value === "pending" ||
			search.isPlaceholderData.value),
)
const view = computed(() => {
	if (isSearchable.value) {
		if (groups.value.length) {
			return "results"
		}

		// a search that found nothing or failed has been answered. Until the
		// first search of a session is, the start state stays.
		if (search.data.value || search.status.value === "error") {
			return "no-results"
		}
	}

	return "start"
})
// every row the arrow keys stop at, in the order the list shows them
const rows = computed((): Row[] => {
	if (view.value === "start") {
		return recents.value.map(({ document, href }) => ({
			document: document,
			href: href,
			more: null,
		}))
	}

	return groups.value.flatMap((group) => [
		{ document: group.result.document, href: group.href, more: null },
		...group.hits.map(({ href }) => ({
			document: group.result.document,
			href: href,
			more: null,
		})),
		...(group.hasMoreHits
			? [
					{
						document: group.result.document,
						href: group.href,
						more: group.result,
					},
				]
			: []),
	])
})
const showFooter = computed(
	() =>
		view.value === "results" ||
		(view.value === "start" && recents.value.length > 0),
)
const hints = computed(() => {
	const res = [
		{ keys: ["↑", "↓"], label: t("sidebar.search.hints.navigate") },
		{ keys: ["↵"], label: t("sidebar.search.hints.open") },
	]

	// the desktop app has no tabs
	if (view.value === "results" && !__DESKTOP_BUILD__) {
		res.push({
			keys: [osType.value === HostOsType.MacOS ? "⌘" : "Ctrl", "↵"],
			label: t("sidebar.search.hints.new-tab"),
		})
	}

	return res
})
const toggleKeys = computed(() =>
	extractShortcutKeys(
		shortcutByOS(SHORTCUT_ACTIONS.searchForDocuments.keyboardKey, osType.value),
	).flatMap(({ key }) => key ?? []),
)
const totalLabel = computed(() => {
	const total = search.data.value?.pages[0]?.total
	if (!total) {
		// NOCOV: the footer is only drawn once a page has loaded.
		return ""
	}

	// a capped search counted only a part of what matches
	return t("sidebar.search.total", {
		matches: total.capped
			? t("sidebar.search.total-matches-capped", {
					count: cappedCount(total.hits),
				})
			: t("sidebar.search.total-matches", { count: total.hits }),
		pages: total.capped
			? t("sidebar.search.total-pages-capped", {
					count: cappedCount(total.documents),
				})
			: t("sidebar.search.total-pages", { count: total.documents }),
	})
})

watch(
	[() => search.status.value, () => search.isPlaceholderData.value],
	([status, isPlaceholderData]) => {
		if (status !== "pending" && !isPlaceholderData) {
			shownQuery.value = searchedQuery.value
		}
	},
)

// new results and the recent pages both start with nothing selected. The
// first page stays the same object while the previous results stand in,
// so the first watcher runs once the new ones have arrived.
watch(
	() => search.data.value?.pages[0],
	() => {
		moreHits.value = {}
		selectedIndex.value = -1
	},
)

watch(view, (view) => {
	if (view === "start") {
		selectedIndex.value = -1
	}
})

watchImmediate(open, (open) => {
	if (open) {
		now.value = new Date()
		selectedIndex.value = -1
		// reload a stale list in the background. Another tab may have added
		// to it.
		void fetchRecentDocuments.refresh()

		setTimeout(() => {
			const input = inputElem.value?.$el ?? inputElem.value
			if (input && typeof input.focus === "function") {
				input.focus()
			}
		}, 100)

		return
	}

	searchQuery.value = ""
})

// a capped count is a lower bound, so its whole thousands say enough
function cappedCount(count: number) {
	return count >= 1000
		? t("sidebar.search.total-thousands", { count: Math.floor(count / 1000) })
		: String(count)
}

// the pages that fill a short list of recent ones, taken from the top of
// the tree down. They wait for the recent pages, so the list is not
// reordered under the user.
function suggestedDocuments(viewed: ListedDocument[]): ListedDocument[] {
	if (fetchRecentDocuments.status.value === "pending") {
		return []
	}

	const viewedIds = new Set(viewed.map((document) => document.id))

	return documentTreeByLevel(fetchDocumentTree.data.value ?? [])
		.filter((elem) => !viewedIds.has(elem.id))
		.flatMap((elem) =>
			// a page still being created has no branch to open yet
			elem.defaultBranchId
				? {
						id: elem.id,
						title: elem.documentName,
						titleHtml: null,
						icon: elem.icon,
						branch: { id: elem.defaultBranchId, name: "", default: true },
					}
				: [],
		)
		.slice(0, Math.max(START_LIST_MIN_LENGTH - viewed.length, 0))
}

function documentHref(document: ListedDocument, hit?: DocumentSearchHit) {
	const { id, branch } = document
	const orgName = fetchOrganization.data.value?.data?.name || ""
	const orgSlug = createNameSlug(orgName)

	const tree = fetchDocumentTree.data.value
	const doc = tree ? extractDocumentTreeElement(tree, id) : null
	const docSlug = doc ? createNameSlugWithId(doc.documentName, id) : id

	let href = orgSlug ? `/${orgSlug}/${docSlug}` : `/${docSlug}`

	// the document page opens the default branch unless told otherwise
	if (!branch.default) {
		href = href + `?branch=${encodeURIComponent(branch.id)}`
	}

	if (hit) {
		href = href + `#${encodeURIComponent(hit.id)}`
	}

	return href
}

function documentBreadcrumbs(document: ListedDocument) {
	const orgName = fetchOrganization.data.value?.data?.name
	// the last step is the document itself, which the row already names
	const parents = documentTreeBreadcrumbs(
		fetchDocumentTree.data.value ?? [],
		document.id,
	)
		.slice(0, -1)
		.map((crumb) => crumb.name)

	return orgName ? [orgName, ...parents] : parents
}

// like the navbar, a page names its branches only once it has a draft.
// The tree marks such a page as protected.
function branchLabel(document: ListedDocument) {
	const hasDraft = extractDocumentTreeElement(
		fetchDocumentTree.data.value ?? [],
		document.id,
	)?.protected

	return document.branch.default && !hasDraft
		? null
		: documentBranchLabel(document.branch, t, true)
}

function updatedMeta(document: DocumentSearchDocument) {
	const time = relativeTimeLabel(document.updatedAt, now.value, t, d)
	const user = updatedBy(document)

	return user
		? t("sidebar.search.updated-meta", { user: user, time: time })
		: time
}

function updatedTooltip(document: DocumentSearchDocument) {
	const date = d(new Date(document.updatedAt), "short-with-time")
	const user = updatedBy(document)

	return user
		? t("sidebar.search.updated-by-tooltip", { user: user, date: date })
		: t("sidebar.search.updated-tooltip", { date: date })
}

function updatedBy(document: DocumentSearchDocument) {
	const userId = document.updatedBy
	if (!userId) {
		return null
	}

	if (userId === fetchAuthSession.data.value?.data?.user.id) {
		return t("sidebar.search.updated-by-you")
	}

	return (
		fetchOrganization.data.value?.data?.members.find((m) => m.userId === userId)
			?.user.name || t("general.deleted-user")
	)
}

function isCurrentDocument(document: ListedDocument) {
	return (
		document.id === editorStore.activeDocumentId &&
		document.branch.id === editorStore.activeBranchId
	)
}

function moveSelection(delta: number) {
	if (!rows.value.length) {
		return
	}

	selectedIndex.value = Math.min(
		Math.max(selectedIndex.value + delta, 0),
		rows.value.length - 1,
	)

	void nextTick(() => {
		list.value
			?.querySelector("[data-selected]")
			?.scrollIntoView({ block: "nearest" })
	})
}

function handleEnter(event: KeyboardEvent) {
	const row = rows.value[selectedIndex.value]
	if (!row) {
		return
	}

	if (row.more) {
		void handleLoadMoreHits(row.more)
		return
	}

	handleOpen(row.document)

	void navigateTo(
		row.href,
		!__DESKTOP_BUILD__ && (event.metaKey || event.ctrlKey)
			? { open: { target: "_blank" } }
			: undefined,
	)
}

function handleOpen(document: ListedDocument) {
	open.value = false
	recordOpen(document)
}

// an open from the search box is what puts a page on the list of recent
// ones. The page's own record of the view does not say where it came from.
function recordOpen(document: ListedDocument) {
	void recordBranchView
		.mutateAsync({
			docId: document.id,
			branchId: document.branch.id,
			from: "search",
		})
		.catch((err: unknown) => {
			console.error("Failed to record branch view:", err)
		})
}

async function handleLoadMore() {
	isLoadingMore.value = true
	await search.loadNextPage()
	isLoadingMore.value = false
}

async function handleLoadMoreHits(result: DocumentSearchResult) {
	const { id, branch } = result.document
	const loaded = moreHits.value[branch.id]
	const nextToken = loaded ? loaded.nextToken : result.nextHitsToken

	if (!nextToken || loadingMoreHits.has(branch.id)) {
		return
	}

	const query = shownQuery.value
	loadingMoreHits.add(branch.id)

	try {
		const res = await searchDocumentBranch(id, branch.id, query, nextToken)

		// the answer belongs to a search the modal has moved on from
		if (query !== shownQuery.value) {
			return
		}

		moreHits.value = {
			...moreHits.value,
			[branch.id]: {
				hits: [...(loaded?.hits ?? []), ...res.hits],
				totalHits: res.totalHits,
				nextToken: res.nextToken,
			},
		}
	} catch {
		showToastMessage("error", t("sidebar.errors.search-more-matches-failed"))
	} finally {
		loadingMoreHits.delete(branch.id)
	}
}
</script>
<template>
	<ShadcnUiDialog v-model:open="open">
		<ShadcnUiDialogContent
			class="top-[15dvh] flex max-h-[70dvh] w-160 max-w-[90dvw] translate-y-0 flex-col overflow-hidden p-0 max-[30rem]:w-[calc(100dvw-1rem)] max-[30rem]:max-w-none"
			@interact-outside="open = false"
		>
			<ShadcnUiDialogTitle class="sr-only">
				{{ t("sidebar.search.title-screen-reader-hint") }}
			</ShadcnUiDialogTitle>
			<ShadcnUiDialogDescription class="sr-only">
				{{ t("sidebar.search.description-screen-reader-hint") }}
			</ShadcnUiDialogDescription>
			<div
				class="flex items-center gap-2.5 border-b px-3.25 py-3"
				@pointerdown="selectedIndex = -1"
			>
				<Icon
					:name="
						isSearching ? 'svg-spinners:blocks-shuffle-3' : 'lucide:search'
					"
					:class="
						cn(
							'mx-px size-4.5 shrink-0 text-muted-foreground',
							isSearching && 'opacity-50',
						)
					"
				/>
				<ShadcnUiInput
					ref="search-input"
					v-model="searchQuery"
					:placeholder="t('sidebar.search.input-placeholder')"
					class="min-w-0 flex-1 md:text-2base!"
					:maxlength="DOCUMENT_SEARCH_QUERY_MAX_LENGTH"
					ghost
					disable-focus-effect
					@keydown.esc="open = false"
					@keydown.down.prevent="moveSelection(1)"
					@keydown.up.prevent="moveSelection(-1)"
					@keydown.enter.prevent="handleEnter"
				/>
				<ShadcnUiKbd :class="TOUCH_HIDDEN_CLASS">
					{{ CLOSE_KEY }}
				</ShadcnUiKbd>
				<ShadcnUiButton
					variant="ghost"
					size="icon-xsm"
					class="hidden rounded-full bg-accent text-muted-foreground [@media(hover:none),(max-width:30rem)]:inline-flex"
					@click="open = false"
				>
					<Icon name="lucide:x" class="size-3" />
					<span class="sr-only">
						{{ t("general.modal-close-screen-reader-hint") }}
					</span>
				</ShadcnUiButton>
			</div>
			<div ref="list" class="flex max-h-full min-h-30 overflow-y-auto p-1.25">
				<div
					v-if="view === 'start' && recents.length"
					class="flex min-w-0 flex-1 flex-col gap-0.5"
				>
					<h3
						class="px-2 pt-2 pb-1 text-2xs font-semibold tracking-wide text-muted-foreground/70 uppercase"
					>
						{{ t("sidebar.search.recent") }}
					</h3>
					<SearchResultRow
						v-for="(recent, index) in recents"
						:key="recent.document.branch.id"
						:document="recent.document"
						:href="recent.href"
						:breadcrumbs="recent.breadcrumbs"
						:meta="recent.meta"
						:meta-tooltip="recent.metaTooltip"
						:branch-label="recent.branchLabel"
						:current="recent.current"
						:class="ROW_CLASS"
						:data-selected="selectedIndex === index ? '' : undefined"
						@click="handleOpen(recent.document)"
						@click.middle="recordOpen(recent.document)"
						@mousemove="selectedIndex = index"
					/>
				</div>
				<div
					v-else-if="view === 'start'"
					class="flex flex-1 flex-col items-center justify-center gap-3 text-center text-muted-foreground"
				>
					<Icon name="lucide:search" class="size-8 opacity-50" />
					<span class="text-2base">{{ t("sidebar.search.empty-state") }}</span>
				</div>
				<ShadcnUiEmpty v-else-if="view === 'no-results'" class="md:p-8">
					<ShadcnUiEmptyHeader class="max-w-full min-w-0">
						<ShadcnUiEmptyMedia>
							<Icon
								name="lucide:search-x"
								class="size-8 text-muted-foreground/50"
							/>
						</ShadcnUiEmptyMedia>
						<ShadcnUiEmptyTitle class="max-w-full truncate font-semibold">
							{{ t("sidebar.search.no-results", { query: shownQuery }) }}
						</ShadcnUiEmptyTitle>
						<ShadcnUiEmptyDescription>
							{{ t("sidebar.search.no-results-description") }}
						</ShadcnUiEmptyDescription>
					</ShadcnUiEmptyHeader>
				</ShadcnUiEmpty>
				<div v-else class="flex min-w-0 flex-1 flex-col gap-0.5">
					<template
						v-for="group in groups"
						:key="group.result.document.branch.id"
					>
						<SearchResultRow
							:document="group.result.document"
							:href="group.href"
							:breadcrumbs="group.breadcrumbs"
							:meta="group.meta"
							:meta-tooltip="group.metaTooltip"
							:branch-label="group.branchLabel"
							:current="group.current"
							:class="ROW_CLASS"
							:data-selected="selectedIndex === group.index ? '' : undefined"
							@click="handleOpen(group.result.document)"
							@click.middle="recordOpen(group.result.document)"
							@mousemove="selectedIndex = group.index"
						/>
						<SearchHitRow
							v-for="({ hit, href }, hitIndex) in group.hits"
							:key="hit.id"
							:hit="hit"
							:href="href"
							:class="ROW_CLASS"
							:data-selected="
								selectedIndex === group.index + 1 + hitIndex ? '' : undefined
							"
							@click="handleOpen(group.result.document)"
							@click.middle="recordOpen(group.result.document)"
							@mousemove="selectedIndex = group.index + 1 + hitIndex"
						/>
						<button
							v-if="group.hasMoreHits"
							type="button"
							:class="
								cn(
									ROW_CLASS,
									'cursor-pointer py-1 pr-2 pl-14 text-left text-xs text-link',
								)
							"
							:data-selected="
								selectedIndex === group.index + 1 + group.hits.length
									? ''
									: undefined
							"
							@click="handleLoadMoreHits(group.result)"
							@mousemove="selectedIndex = group.index + 1 + group.hits.length"
						>
							{{
								t("sidebar.search.more-matches", { count: group.remainingHits })
							}}
						</button>
					</template>
					<div
						v-if="isLoadingMore"
						class="flex shrink-0 items-center justify-center py-3 text-muted-foreground"
					>
						<Icon
							name="svg-spinners:blocks-shuffle-3"
							class="size-5 opacity-50"
						/>
					</div>
				</div>
			</div>
			<div
				v-if="showFooter"
				:class="
					cn(
						'flex items-center gap-3.5 border-t px-3.25 py-2 text-2xs text-muted-foreground/70',
						view === 'start' && TOUCH_HIDDEN_CLASS,
					)
				"
				@pointerdown="selectedIndex = -1"
			>
				<div
					v-for="hint in hints"
					:key="hint.label"
					:class="cn('flex items-center gap-1.5', TOUCH_HIDDEN_CLASS)"
				>
					<ShadcnUiKbdGroup>
						<ShadcnUiKbd v-for="key in hint.keys" :key="key">
							<Icon
								v-if="KEY_ICONS[key]"
								:name="KEY_ICONS[key]"
								class="size-3"
							/>
							<template v-else>{{ key }}</template>
						</ShadcnUiKbd>
					</ShadcnUiKbdGroup>
					{{ hint.label }}
				</div>
				<div v-if="view === 'results'" class="ml-auto">{{ totalLabel }}</div>
				<ShadcnUiKbdGroup v-else class="ml-auto">
					<ShadcnUiKbd v-for="key in toggleKeys" :key="key">
						{{ key }}
					</ShadcnUiKbd>
				</ShadcnUiKbdGroup>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
