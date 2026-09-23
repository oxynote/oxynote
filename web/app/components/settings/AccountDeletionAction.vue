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

const { fetchAuthSession, fetchOrganization, hasPassword, deleteUser } =
	useAuthSession()
const { isEmailEnabled, isSingleOrganization } = useAuthAPI()
const config = useRuntimeConfig()
const { t } = useI18n({ useScope: "global" })

// the password is asked for, and required, only where it is the whole
// confirmation: without email there is no link to send instead.
const passwordRequired = computed(
	() => !isEmailEnabled.value && hasPassword.value,
)
const formSchema = toTypedSchema(
	z
		.object({
			password: z.string().optional(),
		})
		.superRefine((values, ctx) => {
			if (!passwordRequired.value || values.password) {
				return
			}

			ctx.addIssue({
				code: "custom",
				path: ["password"],
				message: t(
					"settings.action-modals.account-deletion.errors.password-required",
				),
			})
		}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref(false)
const lastOrgMemeber = computed(() => {
	const org = fetchOrganization.state.value.data?.data
	if (!org) {
		return false
	}

	return org.members.length <= 1
})
// a single-workspace instance has no signup to repopulate an empty
// workspace, so the server refuses to delete its last member.
const deletionBlocked = computed(
	() => isSingleOrganization.value && lastOrgMemeber.value,
)

const onSubmit = form.handleSubmit(async (values) => {
	if (deletionBlocked.value) {
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	// without email the password confirms the deletion instead of a link;
	// auth-realtime checks it before better-auth handles the request.
	const { error } = (await deleteUser({
		callbackURL: addDeletionSuccessStatusToUrl(
			`${config.public.appBaseURL}/signup`,
		),
		...(isEmailEnabled.value ? {} : { password: values.password }),
	})) as AuthResponse
	if (error) {
		loading.value = false

		if (error.code === "INVALID_PASSWORD") {
			form.setFieldError(
				"password",
				t("settings.action-modals.account-deletion.errors.invalid-password"),
			)

			return
		}

		if (error.code === "LAST_ORGANIZATION_MEMBER") {
			showToastMessage(
				"error",
				t("settings.action-modals.account-deletion.errors.last-member.title"),
				t(
					"settings.action-modals.account-deletion.errors.last-member.description",
				),
			)

			return
		}

		if (error.code === "CREDENTIAL_ACCOUNT_NOT_FOUND") {
			showToastMessage(
				"error",
				t("settings.action-modals.account-deletion.errors.no-password.title"),
				t(
					"settings.action-modals.account-deletion.errors.no-password.description",
				),
			)

			return
		}

		showToastMessage(
			"error",
			t("settings.action-modals.account-deletion.errors.deletion-failed.title"),
			t(
				"settings.action-modals.account-deletion.errors.deletion-failed.description",
			),
		)

		return
	}

	// without email better-auth has already deleted the account and ended
	// its session. The session query still caches the signed-in user, so it
	// is refetched before the middleware reads it on the way to signup.
	if (!isEmailEnabled.value) {
		await fetchAuthSession.refetch()
		emit("close")
		void navigateTo({ path: "/signup", query: { deletion: "success" } })

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
})
</script>
<template>
	<div class="flex flex-col">
		<div class="flex flex-col gap-2 self-stretch">
			<ShadcnUiDialogDescription class="text-2sm">
				{{
					isEmailEnabled
						? $t("settings.action-modals.account-deletion.description")
						: $t(
								"settings.action-modals.account-deletion.description-without-email",
							)
				}}
			</ShadcnUiDialogDescription>
			<div v-if="deletionBlocked" class="text-2sm font-medium text-foreground">
				{{
					$t(
						"settings.action-modals.account-deletion.description-last-member-kept",
					)
				}}
			</div>
			<div
				v-else-if="lastOrgMemeber"
				class="text-2sm font-medium text-foreground"
			>
				{{
					$t(
						"settings.action-modals.account-deletion.description-last-org-member",
					)
				}}
			</div>
		</div>
		<form class="mt-5 flex flex-col gap-5" @submit="onSubmit">
			<ShadcnUiFormField
				v-if="passwordRequired && !deletionBlocked"
				v-slot="{ componentField }"
				name="password"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{ $t("settings.action-modals.account-deletion.password-label") }}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="password"
							autocomplete="current-password"
							:placeholder="
								$t(
									'settings.action-modals.account-deletion.password-placeholder',
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
			<div class="flex gap-2 self-stretch">
				<ShadcnUiButton
					v-if="!deletionBlocked"
					type="submit"
					variant="destructive"
					size="sm"
					:disabled="loading"
					class="text-2sm"
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
					{{
						deletionBlocked
							? $t("settings.action-modals.account-deletion.close-button")
							: $t("settings.action-modals.account-deletion.cancel-button")
					}}
				</ShadcnUiButton>
			</div>
		</form>
	</div>
</template>
