<script lang="ts" setup>
import TagPill from "./TagPill.vue"
import { availableRowWidth, stepTagFit } from "./tag-fit"
import ColorSelect from "./ColorSelect.vue"
import { cn } from "@/lib/utils"
import { chartStyles, colorToHex } from "~/assets/css"
import { showToastMessage } from "../toast"
import { TAG_QUERY_KEYS } from "~/composables/api/useTagAPI"
import { DiffStatus } from "./diff/position-map"

// the ceiling on the pills, whatever the row's width. Measuring only ever
// takes it down from here
const MAX_VISIBLE_TAGS = 4
// how long the row waits out a resize before measuring again, and the
// longest it will hold off during one that never stops
const RESIZE_DEBOUNCE_MS = 100
const RESIZE_MAX_WAIT_MS = 300

const { t } = useI18n({ useScope: "global" })
const {
	fetchTagTree,
	useFetchBranchTags,
	createTag,
	assignBranchTag,
	unassignBranchTag,
} = useTagAPI()
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()
const fetchActiveBranchTags = useFetchBranchTags(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)
// the target's list loads alongside the active one, like its provider,
// so turning the diff on shows the counts without waiting on a request
const fetchTargetBranchTags = useFetchBranchTags(
	() => editorStore.activeDocumentId,
	() => editorStore.targetBranchId,
)
const queryCache = useQueryCache()
const wsState = useWebSocketStateStore()
let unsubWsBranchTagsChange: (() => void) | null | undefined = null

// the sidebar refetches its tree on the tag tree topic, but the pills read
// the branch's own list, which only this topic announces: a tag put on the
// document by someone else, or by the assistant, would otherwise sit
// unseen until the list went stale
watchImmediate(
	() => editorStore.activeDocumentId,
	(newId) => {
		unsubWsBranchTagsChange?.()
		unsubWsBranchTagsChange = null

		if (!newId) {
			return
		}

		unsubWsBranchTagsChange = wsState.state?.subscribe(
			makeWsBranchTagsChangeTopic(newId),
			(rawPayload) => {
				const payload = rawPayload as WSBranchTagsChangePayload

				void queryCache.invalidateQueries({
					key: TAG_QUERY_KEYS.branch(payload.branchId),
				})
			},
		)
	},
)

onUnmounted(() => {
	unsubWsBranchTagsChange?.()
})

const open = ref(false)
// the picker opens on a read only document too, but only to look through.
// The diff shows two branches at once, so it is read only as well
const canEdit = computed(
	() => isEditable.value && !editorStore.reviewableDiffActive,
)
const query = ref("")
// the picker paints its swatches from the theme, so a colour only resolves
// once the popover is on screen
const newColor = ref<string | undefined>(undefined)

const allTags = computed(() => fetchTagTree.data.value ?? [])

// the pills are the active branch's own tags, which the tree cannot answer:
// it lists a document under a tag by its default branch alone
const activeTags = computed(() => {
	const ids = fetchActiveBranchTags.data.value ?? []

	return allTags.value.filter((tag) => ids.includes(tag.id))
})

// null while no diff is shown, or while the target's list is loading
const targetTags = computed(() => {
	const ids = fetchTargetBranchTags.data.value
	if (!editorStore.reviewableDiffActive || !ids) {
		return null
	}

	return allTags.value.filter((tag) => ids.includes(tag.id))
})
const addedTags = computed(() => {
	const target = targetTags.value
	if (!target) {
		return []
	}

	return activeTags.value.filter(
		(tag) => !target.some((targetTag) => targetTag.id === tag.id),
	)
})
const removedTags = computed(
	() => targetTags.value?.filter((tag) => !carriesTag(tag.id)) ?? [],
)
// a sign marks a tag added or removed, so without one the rows need no
// room for it
const hasSigns = computed(
	() => addedTags.value.length > 0 || removedTags.value.length > 0,
)

const rowElem = useTemplateRef<HTMLElement>("tag-row")
const triggerElem = useTemplateRef<HTMLElement>("tag-trigger")
const listElem = useTemplateRef<HTMLElement>("tag-list")
const rowParentElem = computed(() => rowElem.value?.parentElement ?? null)
// the count settles by trying one more pill until the row overflows, then
// stepping back. tooMany remembers the count that overflowed, so the two
// steps cannot chase each other
const visibleCount = ref(MAX_VISIBLE_TAGS)
const tooMany = ref(Number.POSITIVE_INFINITY)
const tooManyAt = ref(0)

const visibleTags = computed(() =>
	activeTags.value.slice(0, visibleCount.value),
)
const overflowCount = computed(
	() => activeTags.value.length - visibleTags.value.length,
)

const trimmedQuery = computed(() => query.value.trim())

// a read only picker lists the active tags alone, and the diff adds the
// ones the active branch removed
const listedTags = computed(() => {
	if (canEdit.value) {
		return allTags.value
	}

	return allTags.value.filter(
		(tag) =>
			carriesTag(tag.id) ||
			removedTags.value.some((removed) => removed.id === tag.id),
	)
})

const matchingTags = computed(() => {
	const q = trimmedQuery.value.toLowerCase()

	return listedTags.value.filter(
		(tag) => !q || tag.tagName.toLowerCase().includes(q),
	)
})

const showCreate = computed(() => {
	const q = trimmedQuery.value.toLowerCase()

	return (
		canEdit.value &&
		!!q &&
		!matchingTags.value.some((tag) => tag.tagName.toLowerCase() === q)
	)
})

onBeforeMount(() => {
	void fetchTagTree.refresh()
})

onMounted(measure)

// a different set of pills is a fresh question, so the ceiling comes back
// and the row settles again from there
watch(activeTags, () => {
	tooMany.value = Number.POSITIVE_INFINITY
	visibleCount.value = Math.min(MAX_VISIBLE_TAGS, activeTags.value.length)
})

// flush post because measure reads the row's geometry: it has to run once
// the pills are patched into the page, where the default pre
// would read the render before them. Watching the pills rather than the
// count keeps a step from taking the two from different renders
watch(visibleTags, measure, { flush: "post" })

// the row sizes to its content, so it does not grow when the page gives it
// more room — only the group holding it does.
//
// A window edge being dragged reports every frame, and each report can
// walk the row a step or two, so the reports are collected: the row keeps
// up with where the drag rests rather than with every frame of it, and
// maxWait still moves it along during a drag that never pauses
const measureOnResize = useDebounceFn(measure, RESIZE_DEBOUNCE_MS, {
	maxWait: RESIZE_MAX_WAIT_MS,
})
useResizeObserver([rowElem, rowParentElem], () => {
	void measureOnResize()
})

// the search is emptied on the way in rather than on the way out: the
// list is still on screen while the menu fades, and clearing it then
// shows the rows springing back to their full length mid-close
watch(open, (isOpen) => {
	if (!isOpen) {
		return
	}

	query.value = ""
	newColor.value = suggestedColor()
})

// the swatches come from theme variables, which resolve to oklch, while a
// tag stores hex — both sides go through hex so a colour the palette
// offers can be recognised in the tags that already hold it
function suggestedColor(): string {
	const colors = chartStyles().selectableColors

	return (
		pickTagColor(
			colors.available.map(colorToHex),
			allTags.value.map((tag) => colorToHex(tag.color)),
		) ?? colorToHex(colors.default)
	)
}

// measure walks the count one pill at a time towards the most that fit the
// space the row actually has. The trigger clips its own overflow, so its
// scrollWidth is what the pills would need
function measure() {
	const row = rowElem.value
	const trigger = triggerElem.value
	if (!row || !trigger) {
		return
	}

	const available = availableRowWidth(row, trigger)
	if (available <= 0) {
		// nothing is laid out yet, and collapsing to a single pill on a row
		// of no width would be worse than waiting
		return
	}

	const next = stepTagFit({
		count: visibleCount.value,
		ceiling: Math.min(MAX_VISIBLE_TAGS, activeTags.value.length),
		needed: trigger.scrollWidth,
		available: available,
		tooMany: tooMany.value,
		tooManyAt: tooManyAt.value,
	})

	tooMany.value = next.tooMany
	tooManyAt.value = next.tooManyAt
	visibleCount.value = next.count
}

function carriesTag(tagId: string): boolean {
	return activeTags.value.some((tag) => tag.id === tagId)
}

function tagDiffStatus(tagId: string): DiffStatus {
	if (addedTags.value.some((tag) => tag.id === tagId)) {
		return DiffStatus.Added
	}

	if (removedTags.value.some((tag) => tag.id === tagId)) {
		return DiffStatus.Removed
	}

	return DiffStatus.Unchanged
}

async function toggleTag(tag: TagTreeElement) {
	const documentId = editorStore.activeDocumentId
	const branchId = editorStore.activeBranchId
	if (!canEdit.value || !documentId || !branchId) {
		return
	}

	try {
		if (carriesTag(tag.id)) {
			await unassignBranchTag.mutateAsync({
				documentId: documentId,
				branchId: branchId,
				tagId: tag.id,
			})

			return
		}

		await assignBranchTag.mutateAsync({
			documentId: documentId,
			branchId: branchId,
			tagId: tag.id,
		})
	} catch {
		showToastMessage("error", t("editor.tags.errors.toggle-failed"))
	}
}

async function createAndAssign() {
	const documentId = editorStore.activeDocumentId
	const branchId = editorStore.activeBranchId
	const tagName = trimmedQuery.value
	if (!documentId || !branchId || !tagName) {
		return
	}

	// the search box is emptied before the request rather than after it.
	// mutateAsync settles only once the refetch its success triggers has,
	// and until then the typed name narrows the list to the one tag that
	// matches it — the one just created
	query.value = ""

	try {
		const creating = createTag.mutateAsync({
			tagName: tagName,
			// the swatches resolve from theme variables, which are oklch; the
			// stored colour is hex so it survives a theme change
			color: colorToHex(
				newColor.value ?? chartStyles().selectableColors.default,
			),
		})
		// the tag is appended, so the row it adds is below the fold on a
		// list long enough to scroll. onMutate has already put it there
		void nextTick(scrollListToEnd)

		const created = await creating
		// the tag just created holds a colour now, so the next one drawn has
		// to weigh it in — the menu stays open across several creations
		newColor.value = suggestedColor()

		await assignBranchTag.mutateAsync({
			documentId: documentId,
			branchId: branchId,
			tagId: created.id,
		})
	} catch {
		showToastMessage("error", t("editor.tags.errors.create-failed"))
	}
}

function scrollListToEnd() {
	const list = listElem.value
	if (!list) {
		return
	}

	list.scrollTop = list.scrollHeight
}

// the create row is the only thing enter can act on: an existing tag is
// toggled from its own row, and a name already taken leaves nothing to
// create
function handleSearchEnter() {
	if (!showCreate.value) {
		return
	}

	void createAndAssign()
}
</script>

<template>
	<div ref="tag-row" class="flex min-w-0 items-center gap-1">
		<span class="shrink-0 text-sm font-medium text-muted-foreground">
			{{ $t("editor.tags.label") }}
		</span>
		<ShadcnUiDropdownMenu v-model:open="open">
			<ShadcnUiDropdownMenuTrigger as-child>
				<!--
					one element across both states: replacing the trigger leaves
					the menu anchored to the removed node, which drops the open
					panel into the top left corner.
					Its height is pinned to the taller state, the plus avatar,
					so that adding or removing the last tag cannot move the menu
					hanging off it
				-->
				<div
					ref="tag-trigger"
					:title="$t('editor.tags.trigger-title')"
					class="flex h-6 min-w-0 cursor-pointer items-center gap-1.25 overflow-hidden"
				>
					<!-- the same height as the pills beside it -->
					<span
						v-if="removedTags.length || addedTags.length"
						class="flex h-5 shrink-0 overflow-hidden rounded-full border border-border bg-background text-xs leading-4 font-semibold select-none"
					>
						<span class="sr-only">
							{{
								$t("editor.diff-change-marker.label", {
									removed: removedTags.length,
									added: addedTags.length,
								})
							}}
						</span>
						<span
							v-if="removedTags.length"
							aria-hidden="true"
							class="flex items-center bg-diff-removed px-2 text-diff-removed-foreground"
						>
							{{
								$t("editor.diff-change-marker.removed", {
									count: removedTags.length,
								})
							}}
						</span>
						<span
							v-if="addedTags.length"
							aria-hidden="true"
							class="flex items-center bg-diff-added px-2 text-diff-added-foreground"
						>
							{{
								$t("editor.diff-change-marker.added", {
									count: addedTags.length,
								})
							}}
						</span>
					</span>
					<template v-if="activeTags.length">
						<!--
							pills hold their natural width so the row's overflow is
							what the count is measured against. The last one left
							has nothing to give way to, so it shrinks and ellipsises
							instead of being clipped on a narrow screen
						-->
						<TagPill
							v-for="tag in visibleTags"
							:key="tag.id"
							:name="tag.tagName"
							:color="tag.color"
							:class="visibleTags.length > 1 ? 'shrink-0' : 'min-w-0'"
						/>
						<TagPill
							v-if="overflowCount > 0"
							class="shrink-0"
							:name="$t('editor.tags.overflow', { count: overflowCount })"
						/>
					</template>
					<ShadcnUiAvatar v-else class="size-6 border">
						<ShadcnUiAvatarFallback>
							<Icon name="lucide:plus" class="size-3.5" />
						</ShadcnUiAvatarFallback>
					</ShadcnUiAvatar>
				</div>
			</ShadcnUiDropdownMenuTrigger>
			<ShadcnUiDropdownMenuContent side="bottom" align="start" class="w-63">
				<ShadcnUiInput
					v-model="query"
					:placeholder="
						canEdit
							? $t('editor.tags.search-placeholder')
							: $t('editor.tags.search-only-placeholder')
					"
					class="h-[1.775rem] border-none bg-muted px-2 text-2sm md:text-2sm"
					disable-focus-effect
					@keydown.enter="handleSearchEnter"
				/>
				<ShadcnUiDropdownMenuSeparator />
				<template v-if="matchingTags.length">
					<div ref="tag-list" class="max-h-47.5 overflow-y-auto">
						<ShadcnUiDropdownMenuItem
							v-for="tag in matchingTags"
							:key="tag.id"
							:value="tag.id"
							:active="carriesTag(tag.id)"
							:class="
								cn(
									// a row that toggles nothing does not react to the pointer
									!canEdit &&
										'cursor-default focus:bg-transparent focus:text-inherit active:bg-transparent active:text-inherit',
									tagDiffStatus(tag.id) === DiffStatus.Added &&
										'bg-diff-added/30 focus:bg-diff-added/30 active:bg-diff-added/30',
									tagDiffStatus(tag.id) === DiffStatus.Removed &&
										'bg-diff-removed/30 focus:bg-diff-removed/30 active:bg-diff-removed/30',
								)
							"
							@select="(event: Event) => event.preventDefault()"
							@click="toggleTag(tag)"
						>
							<div class="flex min-w-0 flex-1 items-center gap-2">
								<!-- every row keeps the sign's column so the dots line up -->
								<span
									v-if="hasSigns"
									aria-hidden="true"
									class="w-2 shrink-0 text-center font-semibold"
									:class="{
										'text-diff-added-foreground':
											tagDiffStatus(tag.id) === DiffStatus.Added,
										'text-diff-removed-foreground':
											tagDiffStatus(tag.id) === DiffStatus.Removed,
									}"
								>
									<template v-if="tagDiffStatus(tag.id) === DiffStatus.Added">
										{{ $t("editor.tags.diff.added-sign") }}
									</template>
									<template
										v-else-if="tagDiffStatus(tag.id) === DiffStatus.Removed"
									>
										{{ $t("editor.tags.diff.removed-sign") }}
									</template>
								</span>
								<span
									class="size-2.5 shrink-0 rounded-full"
									:style="{ backgroundColor: tag.color }"
								/>
								<span
									class="min-w-0 truncate whitespace-nowrap"
									:class="{
										'line-through':
											tagDiffStatus(tag.id) === DiffStatus.Removed,
									}"
								>
									{{ tag.tagName }}
								</span>
								<span
									v-if="tagDiffStatus(tag.id) === DiffStatus.Added"
									class="sr-only"
								>
									{{ $t("editor.tags.diff.added") }}
								</span>
								<span
									v-else-if="tagDiffStatus(tag.id) === DiffStatus.Removed"
									class="sr-only"
								>
									{{ $t("editor.tags.diff.removed") }}
								</span>
							</div>
						</ShadcnUiDropdownMenuItem>
					</div>
					<ShadcnUiDropdownMenuSeparator v-if="canEdit" />
				</template>
				<div v-if="showCreate" class="flex items-stretch gap-1">
					<ShadcnUiDropdownMenuItem
						class="min-w-0 flex-1 py-0"
						@select="(event: Event) => event.preventDefault()"
						@click="createAndAssign"
					>
						<span class="text-muted-foreground">
							{{ $t("editor.tags.create") }}
						</span>
						<TagPill
							v-if="newColor"
							:name="trimmedQuery"
							:color="newColor"
							class="max-w-35"
						/>
					</ShadcnUiDropdownMenuItem>
					<ShadcnUiPopover>
						<ShadcnUiPopoverTrigger as-child>
							<ShadcnUiButton
								size="icon"
								variant="outline-transparent"
								class="size-[1.775rem]!"
								:title="$t('editor.tags.color-title')"
							>
								<div
									class="size-3.5 rounded-full transition-colors"
									:style="{ backgroundColor: newColor }"
								/>
							</ShadcnUiButton>
						</ShadcnUiPopoverTrigger>
						<ShadcnUiPopoverContent
							side="bottom"
							align="end"
							class="w-fit min-w-0"
						>
							<ColorSelect v-model="newColor" />
						</ShadcnUiPopoverContent>
					</ShadcnUiPopover>
				</div>
				<div
					v-else-if="canEdit"
					class="px-2 py-1.25 text-xs text-muted-foreground"
				>
					{{ $t("editor.tags.create-hint") }}
				</div>
				<div
					v-else-if="!matchingTags.length"
					class="px-2 py-1.25 text-xs text-muted-foreground"
				>
					{{ $t("editor.tags.no-results") }}
				</div>
			</ShadcnUiDropdownMenuContent>
		</ShadcnUiDropdownMenu>
	</div>
</template>
