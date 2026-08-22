<script lang="ts" setup>
import { showToastMessage } from "../toast"
import type { OrganizationMember } from "./workspace"

const props = defineProps<{
	member: OrganizationMember
}>()
const emit = defineEmits<{
	(event: "close"): void
}>()

const {
	fetchOrganization,
	cancelOrganizationInvitation,
	removeOrganizationMember,
} = useAuthSession()
const { t } = useI18n({ useScope: "global" })
const loading = ref(false)

async function removeMember() {
	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	if (props.member.invitationPending) {
		const { error } = (await cancelOrganizationInvitation({
			invitationId: props.member.id,
		})) as AuthResponse
		if (error) {
			loading.value = false
			showToastMessage(
				"error",
				t(
					"settings.action-modals.workspace-member-removal.error-message.title",
					{
						member: props.member.user.name,
					},
				),
				t(
					"settings.action-modals.workspace-member-removal.error-message.description",
					{
						member: props.member.user.name,
					},
				),
			)
			emit("close")

			return
		}
	} else {
		const { error } = (await removeOrganizationMember({
			memberIdOrEmail: props.member.id,
		})) as AuthResponse
		if (error) {
			loading.value = false
			showToastMessage(
				"error",
				t(
					"settings.action-modals.workspace-member-removal.error-message.title",
					{
						member: props.member.user.name,
					},
				),
				t(
					"settings.action-modals.workspace-member-removal.error-message.description",
					{
						member: props.member.user.name,
					},
				),
			)
			emit("close")

			return
		}
	}

	await fetchOrganization.refetch()

	showToastMessage(
		"success",
		t("settings.action-modals.workspace-member-removal.success-message.title", {
			member: props.member.user.name,
		}),
	)
	emit("close")
}
</script>
<template>
	<div class="flex flex-col gap-5">
		<i18n-t
			scope="global"
			keypath="settings.action-modals.workspace-member-removal.description"
			class="text-2sm text-muted-foreground"
			tag="p"
		>
			<template #member>
				<span class="font-medium text-foreground">
					{{ props.member.user.name }}
				</span>
			</template>
		</i18n-t>
		<div class="flex w-full gap-2">
			<ShadcnUiButton
				type="submit"
				size="sm"
				:disabled="loading"
				class="text-2sm"
				@click="removeMember()"
			>
				<Icon
					v-show="loading"
					name="svg-spinners:blocks-shuffle-3"
					class="size-3"
				/>
				{{
					$t("settings.action-modals.workspace-member-removal.submit-button")
				}}
			</ShadcnUiButton>
			<ShadcnUiButton
				type="button"
				size="sm"
				variant="secondary"
				:disabled="loading"
				class="text-2sm"
				@click="emit('close')"
			>
				{{
					$t("settings.action-modals.workspace-member-removal.cancel-button")
				}}
			</ShadcnUiButton>
		</div>
	</div>
</template>
