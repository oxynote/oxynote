<script lang="ts" setup>
import { showToastMessage } from "../toast"

const props = defineProps<{
	data: DataSource
}>()
const emit = defineEmits<{
	(event: "close"): void
}>()

const { deleteDataSource } = useDataSourceAPI()
const { t } = useI18n({ useScope: "global" })
const loading = ref(false)

async function removeDataSource() {
	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	try {
		await deleteDataSource.mutateAsync(props.data.id)
	} catch {
		loading.value = false
		showToastMessage(
			"error",
			t("settings.action-modals.data-source-removal.error-message.title", {
				name: props.data.name,
			}),
			t(
				"settings.action-modals.data-source-removal.error-message.description",
				{
					name: props.data.name,
				},
			),
		)

		return
	}

	showToastMessage(
		"success",
		t("settings.action-modals.data-source-removal.success-message.title", {
			name: props.data.name,
		}),
	)
	emit("close")
}
</script>
<template>
	<div class="flex flex-col gap-5">
		<i18n-t
			scope="global"
			keypath="settings.action-modals.data-source-removal.description"
			class="text-2sm text-muted-foreground"
			tag="p"
		>
			<template #name>
				<span class="font-medium text-foreground">
					{{ props.data.name }}
				</span>
			</template>
		</i18n-t>
		<div class="flex w-full gap-2">
			<ShadcnUiButton
				type="submit"
				size="sm"
				:disabled="loading"
				class="text-2sm"
				@click="removeDataSource()"
			>
				<Icon
					v-show="loading"
					name="svg-spinners:blocks-shuffle-3"
					class="size-3"
				/>
				{{ $t("settings.action-modals.data-source-removal.submit-button") }}
			</ShadcnUiButton>
			<ShadcnUiButton
				type="button"
				size="sm"
				variant="secondary"
				:disabled="loading"
				class="text-2sm"
				@click="emit('close')"
			>
				{{ $t("settings.action-modals.data-source-removal.cancel-button") }}
			</ShadcnUiButton>
		</div>
	</div>
</template>
