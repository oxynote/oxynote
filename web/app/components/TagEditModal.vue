<script lang="ts" setup>
import { colorToHex } from "~/assets/css"
import ColorSelect from "./editor/ColorSelect.vue"

const open = defineModel<Pick<
	TagTreeElement,
	"id" | "tagName" | "color"
> | null>({
	default: null,
})

const { t } = useI18n({ useScope: "global" })
const { updateTag } = useTagAPI()
const nameId = useId()
const tagName = ref("")
const color = ref<string | undefined>()
const loading = ref(false)
const errorMessage = ref("")
const canSave = computed(
	() =>
		!!open.value &&
		!!tagName.value.trim() &&
		(tagName.value.trim() !== open.value.tagName ||
			color.value !== open.value.color),
)

watch(
	open,
	(tag) => {
		if (!tag) {
			return
		}

		tagName.value = tag.tagName
		color.value = tag.color
		errorMessage.value = ""
	},
	{ immediate: true },
)

function close() {
	if (!loading.value) {
		open.value = null
	}
}

async function save() {
	const tag = open.value
	if (!tag || !canSave.value || loading.value) {
		return
	}

	const req: TagUpdateRequest = {}
	if (tagName.value.trim() !== tag.tagName) {
		req.tagName = tagName.value.trim()
	}

	if (color.value && color.value !== tag.color) {
		req.color = color.value
	}

	loading.value = true
	errorMessage.value = ""

	try {
		await updateTag.mutateAsync({ id: tag.id, req })
		open.value = null
	} catch (err) {
		const { data } = err as { data?: { code?: string } }
		errorMessage.value =
			data?.code === "tag.duplicate_name"
				? t("sidebar.tag-edit-modal.duplicate-name")
				: t("sidebar.errors.update-tag-failed")
	} finally {
		loading.value = false
	}
}
</script>

<template>
	<ShadcnUiDialog
		:open="!!open"
		@update:open="
			(value) => {
				if (!value) close()
			}
		"
	>
		<ShadcnUiDialogContent
			class="max-h-[90dvh] w-[85dvw] overflow-y-auto text-foreground sm:w-110"
		>
			<ShadcnUiDialogHeader>
				<ShadcnUiDialogTitle class="text-base">
					{{ $t("sidebar.tag-edit-modal.title") }}
				</ShadcnUiDialogTitle>
				<ShadcnUiDialogDescription class="text-2sm">
					{{ $t("sidebar.tag-edit-modal.description") }}
				</ShadcnUiDialogDescription>
				<ShadcnUiButton
					type="button"
					variant="ghost-plain"
					class="absolute top-0 right-0 shrink-0 p-0"
					:disabled="loading"
					@click="close"
				>
					<Icon name="mingcute:close-line" size="1rem" />
					<span class="sr-only">
						{{ $t("general.modal-close-screen-reader-hint") }}
					</span>
				</ShadcnUiButton>
			</ShadcnUiDialogHeader>
			<form class="mt-4 flex flex-col gap-4" @submit.prevent="save">
				<div class="flex flex-col gap-2">
					<ShadcnUiLabel :for="nameId">
						{{ $t("sidebar.tag-edit-modal.name-label") }}
					</ShadcnUiLabel>
					<ShadcnUiInput
						:id="nameId"
						v-model="tagName"
						:disabled="loading"
						required
					/>
				</div>
				<ShadcnUiPopover>
					<ShadcnUiPopoverTrigger as-child>
						<ShadcnUiButton
							type="button"
							variant="outline"
							class="w-fit gap-2"
							:disabled="loading"
						>
							<span
								class="size-3 rounded-full"
								:style="{ backgroundColor: color }"
							/>
							{{ $t("sidebar.tag-edit-modal.color-label") }}
						</ShadcnUiButton>
					</ShadcnUiPopoverTrigger>
					<ShadcnUiPopoverContent
						inside-modal
						class="w-fit min-w-0"
						align="start"
					>
						<ColorSelect
							:model-value="color"
							@update:model-value="
								(value) => {
									if (value) color = colorToHex(value)
								}
							"
						/>
					</ShadcnUiPopoverContent>
				</ShadcnUiPopover>
				<p v-if="errorMessage" role="alert" class="text-2sm text-destructive">
					{{ errorMessage }}
				</p>
				<div class="flex gap-2">
					<ShadcnUiButton
						type="submit"
						size="sm"
						:disabled="loading || !canSave"
					>
						<Icon
							v-if="loading"
							name="svg-spinners:blocks-shuffle-3"
							class="size-3"
						/>
						{{ $t("sidebar.tag-edit-modal.save-button") }}
					</ShadcnUiButton>
					<ShadcnUiButton
						type="button"
						size="sm"
						variant="secondary"
						:disabled="loading"
						@click="close"
					>
						{{ $t("sidebar.tag-edit-modal.cancel-button") }}
					</ShadcnUiButton>
				</div>
			</form>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
