<script lang="ts" setup>
const open = defineModel<{
	deleteDocument: () => Promise<void>
	name: string
} | null>({
	default: null,
})

const lastValidTarget = useLastValidRef(open)
const loading = ref(false)

async function deleteDocument() {
	if (!lastValidTarget.value || !open.value) {
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment
	await lastValidTarget.value.deleteDocument()
	open.value = null

	setTimeout(() => {
		// just to ensure that the spinner is visible until the modal closes
		loading.value = false
	}, 500)
}
</script>
<template>
	<ShadcnUiDialog
		:open="!!open"
		@update:open="
			(v) => {
				if (!v) {
					open = null
				}
			}
		"
	>
		<ShadcnUiDialogContent
			class="max-h-[90dvh] w-[85dvw] overflow-y-auto p-0 text-foreground sm:w-110 md:max-h-[80dvh]"
		>
			<div class="flex flex-col gap-8 p-6">
				<div class="flex min-h-0 flex-col gap-3">
					<ShadcnUiDialogHeader>
						<ShadcnUiDialogTitle class="text-base">
							{{ $t("editor.document-deletion-modal.title") }}
						</ShadcnUiDialogTitle>
						<ShadcnUiButton
							variant="ghost-plain"
							class="absolute top-1/2 right-0 shrink-0 -translate-y-1/2 p-0"
							@click="open = null"
						>
							<Icon name="lucide:x" size="1rem" />
							<span class="sr-only">
								{{ $t("general.modal-close-screen-reader-hint") }}
							</span>
						</ShadcnUiButton>
					</ShadcnUiDialogHeader>
					<div class="flex flex-col gap-2 self-stretch">
						<i18n-t
							keypath="editor.document-deletion-modal.description"
							tag="p"
							class="text-2sm text-muted-foreground"
						>
							<template #name>
								<span class="font-medium break-all">
									{{ lastValidTarget?.name }}
								</span>
							</template>
						</i18n-t>
					</div>
					<div class="flex gap-2 self-stretch">
						<ShadcnUiButton
							type="button"
							variant="destructive"
							size="sm"
							:disabled="loading"
							class="text-2sm"
							@click="deleteDocument"
						>
							<Icon
								v-show="loading"
								name="svg-spinners:blocks-shuffle-3"
								class="size-3"
							/>
							{{ $t("editor.document-deletion-modal.confirm-button") }}
						</ShadcnUiButton>
						<ShadcnUiButton
							type="button"
							size="sm"
							variant="secondary"
							:disabled="loading"
							class="text-2sm"
							@click="open = null"
						>
							{{ $t("editor.document-deletion-modal.cancel-button") }}
						</ShadcnUiButton>
					</div>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
