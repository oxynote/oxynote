<script lang="ts" setup>
import { showToastMessage } from "../toast"

const emit = defineEmits<{
	(event: "close"): void
}>()

const { fetchOrganization, deleteUser } = useAuthSession()
const config = useRuntimeConfig()
const { t } = useI18n({ useScope: "global" })

const loading = ref(false)
const lastOrgMemeber = computed(() => {
	const org = fetchOrganization.state.value.data?.data
	if (!org) {
		return false
	}

	return org.members.length <= 1
})

async function deleteAccount() {
	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const { error } = await deleteUser({
		callbackURL: addDeletionSuccessStatusToUrl(
			`${config.public.appBaseURL}/signup`,
		),
	})
	if (error) {
		showToastMessage(
			"error",
			t("settings.action-modals.account-deletion.errors.deletion-failed.title"),
			t(
				"settings.action-modals.account-deletion.errors.deletion-failed.description",
			),
		)
		loading.value = false

		return
	}

	showToastMessage(
		"success",
		t(
			"settings.action-modals.account-deletion.success-message.confirmation-link-sent.title",
		),
		t(
			"settings.action-modals.account-deletion.success-message.confirmation-link-sent.description",
		),
	)
	emit("close")
}
</script>
<template>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2 self-stretch">
			<p class="text-2sm text-muted-foreground">
				{{ $t("settings.action-modals.account-deletion.description") }}
			</p>
			<div v-if="lastOrgMemeber" class="text-2sm font-medium text-foreground">
				{{
					$t(
						"settings.action-modals.account-deletion.description-last-org-member",
					)
				}}
			</div>
		</div>
		<div class="flex gap-2 self-stretch">
			<ShadcnUiButton
				type="button"
				variant="destructive"
				size="sm"
				:disabled="loading"
				class="text-2sm"
				@click="deleteAccount"
			>
				<Icon
					v-show="loading"
					name="svg-spinners:blocks-shuffle-3"
					class="size-3"
				/>
				{{ $t("settings.action-modals.account-deletion.confirm-button") }}
			</ShadcnUiButton>
			<ShadcnUiButton
				type="button"
				size="sm"
				variant="secondary"
				:disabled="loading"
				class="text-2sm"
				@click="emit('close')"
			>
				{{ $t("settings.action-modals.account-deletion.cancel-button") }}
			</ShadcnUiButton>
		</div>
	</div>
</template>
