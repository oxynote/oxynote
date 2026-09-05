<script lang="ts" setup>
import TagPill from "./TagPill.vue"
import ColorSelect from "./ColorSelect.vue"
import { chartStyles, colorToHex } from "~/assets/css"
import { showToastMessage } from "../toast"

// beyond this the pills collapse into a "+N" counter rather than pushing
// the rest of the metadata row off screen
const MAX_VISIBLE_TAGS = 4

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

const visibleTags = computed(() =>
	documentTags.value.slice(0, MAX_VISIBLE_TAGS),
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

watch(pickerOpen, (isOpen) => {
	if (isOpen) {
		newColor.value = suggestedColor()

		return
	}

	query.value = ""
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
	if (!documentId || !branchId || !trimmedQuery.value) {
		return
	}

	try {
		const created = await createTag.mutateAsync({
			tagName: trimmedQuery.value,
			// the swatches resolve from theme variables, which are oklch; the
			// stored colour is hex so it survives a theme change
			color: colorToHex(
				newColor.value ?? chartStyles().selectableColors.default,
			),
		})

		query.value = ""
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
	<div class="flex items-center gap-1">
		<span class="text-sm font-medium text-muted-foreground">
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
					panel into the top left corner
				-->
				<div
					:data-disabled="!isEditable ? '' : undefined"
					:title="$t('editor.tags.trigger-title')"
					class="flex cursor-pointer items-center gap-1.25 data-disabled:cursor-default"
				>
					<template v-if="documentTags.length">
						<TagPill
							v-for="tag in visibleTags"
							:key="tag.id"
							:name="tag.tagName"
							:color="tag.color"
						/>
						<TagPill
							v-if="overflowCount > 0"
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
					<div class="max-h-47.5 overflow-y-auto">
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
