<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import { showToastMessage } from "../toast"

const emit = defineEmits<{
	(event: "close"): void
}>()

const formSchema = toTypedSchema(
	z.object({
		email: z.string().trim().email(),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const { fetchAuthSession, fetchOrganization, inviteOrganizationMember } =
	useAuthSession()
const { t } = useI18n({ useScope: "global" })
const loading = ref(false)
const isMaxMembersReached = computed(() => {
	const org = fetchOrganization.state.value.data?.data
	if (!org) {
		return false
	}

	return (
		org.members.length +
			org.invitations.filter((inv) => inv.status === "pending").length >=
		ORGANIZATION_MAX_MEMBERS
	)
})

const onSubmit = form.handleSubmit(async (values) => {
	if (isMaxMembersReached.value) {
		return
	}

	if (values.email === fetchAuthSession.state.value.data?.data?.user.email) {
		emit("close")
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const { error } = (await inviteOrganizationMember({
		email: values.email,
		role: "owner", // in the future we will allow selecting roles
		resend: true,
	})) as AuthResponse
	if (error) {
		form.setErrors({ email: error.message })
		loading.value = false

		return
	}

	await fetchOrganization.refetch()

	showToastMessage(
		"success",
		t("settings.action-modals.workspace-invitation.success-message.title"),
		t(
			"settings.action-modals.workspace-invitation.success-message.description",
			{
				email: values.email,
			},
		),
	)
	emit("close")
})
</script>
<template>
	<div class="flex flex-col">
		<p class="text-2sm text-muted-foreground">
			{{
				isMaxMembersReached
					? $t(
							"settings.action-modals.workspace-invitation.description-max-members-reached",
						)
					: $t("settings.action-modals.workspace-invitation.description")
			}}
		</p>
		<form
			class="mt-5 flex w-full flex-col items-center gap-5"
			autocomplete="off"
			@submit="onSubmit"
		>
			<ShadcnUiFormField
				v-if="!isMaxMembersReached"
				v-slot="{ componentField }"
				name="email"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{ $t("settings.action-modals.workspace-invitation.email-label") }}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="email"
							:placeholder="
								$t(
									'settings.action-modals.workspace-invitation.email-placeholder',
								)
							"
							disable-focus-effect
							disable-destructive-effect
							:class="cn('h-8 text-2sm!', loading && 'pointer-events-none')"
							v-bind="{
								...componentField,
							}"
						/>
					</ShadcnUiFormControl>
					<ShadcnUiFormMessage class="text-2xs" />
				</ShadcnUiFormItem>
			</ShadcnUiFormField>
			<div class="flex w-full gap-2">
				<ShadcnUiButton
					v-if="isMaxMembersReached"
					type="button"
					size="sm"
					:disabled="loading"
					class="text-2sm"
					@click="emit('close')"
				>
					{{ $t("settings.action-modals.workspace-invitation.close-button") }}
				</ShadcnUiButton>
				<template v-else>
					<ShadcnUiButton
						type="submit"
						size="sm"
						:disabled="loading"
						class="text-2sm"
					>
						<Icon
							v-show="loading"
							name="svg-spinners:blocks-shuffle-3"
							class="size-3"
						/>
						{{
							$t("settings.action-modals.workspace-invitation.submit-button")
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
							$t("settings.action-modals.workspace-invitation.cancel-button")
						}}
					</ShadcnUiButton>
				</template>
			</div>
		</form>
	</div>
</template>
