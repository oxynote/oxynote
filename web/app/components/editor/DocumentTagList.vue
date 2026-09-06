<script lang="ts" setup>
import TagPill from "./TagPill.vue"
import { availableRowWidth, stepTagFit } from "./tag-fit"
import ColorSelect from "./ColorSelect.vue"
import { chartStyles, colorToHex } from "~/assets/css"
import { showToastMessage } from "../toast"

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
const fetchBranchTags = useFetchBranchTags(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)

const open = ref(false)
// a read only document still shows its pills, but the picker behind them
// stays shut
const pickerOpen = computed(() => isEditable.value && open.value)
const query = ref("")
// the picker paints its swatches from the theme, so a colour only resolves
// once the popover is on screen
const newColor = ref<string | undefined>(undefined)

const allTags = computed(() => fetchTagTree.data.value ?? [])

// the pills are the open branch's own tags, which the tree cannot answer:
// it lists a document under a tag by its default branch alone
const documentTags = computed(() => {
	const ids = fetchBranchTags.data.value ?? []

	return allTags.value.filter((tag) => ids.includes(tag.id))
})

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
	documentTags.value.slice(0, visibleCount.value),
)
const overflowCount = computed(
	() => documentTags.value.length - visibleTags.value.length,
)

const trimmedQuery = computed(() => query.value.trim())

const matchingTags = computed(() => {
	const q = trimmedQuery.value.toLowerCase()

	return allTags.value.filter(
		(tag) => !q || tag.tagName.toLowerCase().includes(q),
	)
})

const showCreate = computed(() => {
	const q = trimmedQuery.value.toLowerCase()

	return (
		!!q && !matchingTags.value.some((tag) => tag.tagName.toLowerCase() === q)
	)
})

onBeforeMount(() => {
	void fetchTagTree.refresh()
})

onMounted(measure)

// a different set of pills is a fresh question, so the ceiling comes back
// and the row settles again from there
watch(documentTags, () => {
	tooMany.value = Number.POSITIVE_INFINITY
	visibleCount.value = Math.min(MAX_VISIBLE_TAGS, documentTags.value.length)
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
watch(pickerOpen, (isOpen) => {
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
		ceiling: Math.min(MAX_VISIBLE_TAGS, documentTags.value.length),
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
	return documentTags.value.some((tag) => tag.id === tagId)
}

async function toggleTag(tag: TagTreeElement) {
	const documentId = editorStore.activeDocumentId
	const branchId = editorStore.activeBranchId
	if (!documentId || !branchId) {
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
		<ShadcnUiDropdownMenu
			:open="pickerOpen"
			@update:open="(v: boolean) => (open = v)"
		>
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
					:data-disabled="!isEditable ? '' : undefined"
					:title="$t('editor.tags.trigger-title')"
					class="flex h-6 min-w-0 cursor-pointer items-center gap-1.25 overflow-hidden data-disabled:cursor-default"
				>
					<template v-if="documentTags.length">
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
					:placeholder="$t('editor.tags.search-placeholder')"
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
							@select="(event: Event) => event.preventDefault()"
							@click="toggleTag(tag)"
						>
							<div class="flex min-w-0 flex-1 items-center gap-2">
								<span
									class="size-2.5 shrink-0 rounded-full"
									:style="{ backgroundColor: tag.color }"
								/>
								<span class="min-w-0 truncate whitespace-nowrap">
									{{ tag.tagName }}
								</span>
							</div>
						</ShadcnUiDropdownMenuItem>
					</div>
					<ShadcnUiDropdownMenuSeparator />
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
				<div v-else class="px-2 py-1.25 text-xs text-muted-foreground">
					{{ $t("editor.tags.create-hint") }}
				</div>
			</ShadcnUiDropdownMenuContent>
		</ShadcnUiDropdownMenu>
	</div>
</template>
