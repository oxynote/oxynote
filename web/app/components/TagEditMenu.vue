<script lang="ts" setup>
import { showToastMessage } from "./toast"

const props = defineProps<{
	tag: TagTreeElement
}>()
const emit = defineEmits<{
	(e: "close"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { fetchTagTree, updateTag } = useTagAPI()
const nameId = useId()

const name = ref(props.tag.tagName)
const color = ref(props.tag.color)
const trimmedName = computed(() => name.value.trim())
const isNameTaken = computed(
	() =>
		!!fetchTagTree.data.value?.some(
			(tag) => tag.id !== props.tag.id && tag.tagName === trimmedName.value,
		),
)
const changes = computed(() => {
	const req: TagUpdateRequest = {}
	if (trimmedName.value !== props.tag.tagName) {
		req.tagName = trimmedName.value
	}

	if (color.value !== props.tag.color) {
		req.color = color.value
	}

	return req
})
const canSave = computed(
	() =>
		!!trimmedName.value &&
		!isNameTaken.value &&
		Object.keys(changes.value).length > 0,
)

function pickColor(value: string | undefined) {
	if (!value) {
		return
	}

	color.value = value
}

// the menu closes before the request goes out. The tree already shows the
// change by then, and a toast reports how it ended
async function save() {
	if (!canSave.value) {
		return
	}

	const req = changes.value
	emit("close")

	try {
		await updateTag.mutateAsync({ id: props.tag.id, req: req })
		showToastMessage(
			"success",
			t("sidebar.tag-edit-menu.update-success", {
				name: req.tagName ?? props.tag.tagName,
			}),
		)
	} catch (err) {
		// the mutation rejects with an ofetch FetchError, whose `data` holds
		// the core API's JSON body. The tree can be stale, so a name taken
		// by someone else meanwhile only shows up here
		const { data } = err as { data?: { code?: string } }
		showToastMessage(
			"error",
			data?.code === "tag.duplicate_name"
				? t("sidebar.tag-edit-menu.name-taken")
				: t("sidebar.errors.update-tag-failed"),
		)
	}
}
</script>
<template>
	<form class="flex w-52 flex-col" @submit.prevent="save" @keydown.tab.stop>
		<!--
			reka-ui's sub menu closes on ArrowLeft, and its modal root swallows
			Tab, wherever inside them the key is pressed. So Tab stays with the
			form, and ArrowLeft with the name field
		-->
		<div class="flex flex-col gap-1 px-2 py-1.5">
			<ShadcnUiLabel
				:for="nameId"
				class="mt-0.5 text-left text-2sm font-medium text-foreground"
			>
				{{ $t("sidebar.tag-edit-menu.name-label") }}
			</ShadcnUiLabel>
			<div class="flex gap-1">
				<ShadcnUiInput
					:id="nameId"
					v-model="name"
					class="h-[1.775rem] px-2 text-2sm md:text-2sm"
					disable-focus-effect
					@keydown.arrow-left.stop
				/>
				<ShadcnUiPopover>
					<ShadcnUiPopoverTrigger as-child>
						<ShadcnUiButton
							type="button"
							size="icon"
							variant="outline-transparent"
							class="size-[1.775rem]! shrink-0"
							:title="$t('sidebar.tag-edit-menu.color-title')"
						>
							<div
								class="size-3.5 rounded-full transition-colors"
								:style="{ backgroundColor: paletteColor(color) }"
							/>
						</ShadcnUiButton>
					</ShadcnUiPopoverTrigger>
					<ShadcnUiPopoverContent
						side="bottom"
						align="end"
						class="w-fit min-w-0"
						inside-sheet
					>
						<ColorSelect :model-value="color" @update:model-value="pickColor" />
					</ShadcnUiPopoverContent>
				</ShadcnUiPopover>
			</div>
			<p v-if="isNameTaken" class="text-xs text-destructive-foreground">
				{{ $t("sidebar.tag-edit-menu.name-taken") }}
			</p>
		</div>
		<ShadcnUiDropdownMenuSeparator />
		<div class="px-2 py-1.5">
			<ShadcnUiButton
				type="submit"
				class="w-full gap-1"
				size="2sm"
				:disabled="!canSave"
			>
				<Icon name="mingcute:save-2-line" />
				{{ $t("sidebar.tag-edit-menu.update-button") }}
			</ShadcnUiButton>
		</div>
	</form>
</template>
