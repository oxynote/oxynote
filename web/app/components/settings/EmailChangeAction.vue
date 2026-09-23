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
		password: z.string().optional(),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const { fetchAuthSession, hasPassword, changeEmail } = useAuthSession()
const { isEmailEnabled, defaultAdmin } = useAuthAPI()
const config = useRuntimeConfig()
const { t } = useI18n({ useScope: "global" })
const loading = ref(false)
const currentEmail = computed(
	() => fetchAuthSession.state.value.data?.data?.user.email ?? "",
)
// no mail reaches the default admin's email. So its change skips the
// approval by the current email, and only the new email gets a link.
const isDefaultAdmin = computed(
	() =>
		defaultAdmin.value !== null &&
		defaultAdmin.value.email === currentEmail.value,
)

const onSubmit = form.handleSubmit(async (values) => {
	if (values.email === currentEmail.value) {
		emit("close")
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	// without email the password confirms the change instead of a link;
	// auth-realtime checks it before better-auth handles the request.
	const { error } = (await changeEmail({
		newEmail: values.email,
		callbackURL: postEmailVerificationUrl(
			config.public.appBaseURL,
			values.email,
		),
		...(isEmailEnabled.value ? {} : { password: values.password }),
	})) as AuthResponse
	if (error) {
		loading.value = false

		if (error.code === "INVALID_PASSWORD") {
			form.setFieldError(
				"password",
				t("settings.action-modals.email-change.errors.invalid-password"),
			)

			return
		}

		if (error.code === "CREDENTIAL_ACCOUNT_NOT_FOUND") {
			showToastMessage(
				"error",
				t("settings.action-modals.email-change.errors.no-password.title"),
				t("settings.action-modals.email-change.errors.no-password.description"),
			)

			return
		}

		form.setErrors({ email: error.message })

		return
	}

	// read before the refetch. The session may already carry the new email.
	const skippedApproval = isDefaultAdmin.value
	const previousEmail = currentEmail.value

	await fetchAuthSession.refetch()

	if (isEmailEnabled.value && skippedApproval) {
		showToastMessage(
			"success",
			t(
				"settings.action-modals.email-change.success-message-default-admin.title",
			),
			t(
				"settings.action-modals.email-change.success-message-default-admin.description",
				{ email: values.email },
			),
		)
	} else if (isEmailEnabled.value) {
		showToastMessage(
			"success",
			t("settings.action-modals.email-change.success-message.title"),
			t("settings.action-modals.email-change.success-message.description", {
				current: previousEmail,
				email: values.email,
			}),
		)
	} else {
		showToastMessage(
			"success",
			t(
				"settings.action-modals.email-change.success-message-without-email.title",
			),
			t(
				"settings.action-modals.email-change.success-message-without-email.description",
				{ email: values.email },
			),
		)
	}

	emit("close")
})
</script>
<template>
	<div class="flex flex-col">
		<ShadcnUiDialogDescription class="text-2sm">
			{{
				!isEmailEnabled
					? $t("settings.action-modals.email-change.description-without-email")
					: isDefaultAdmin
						? $t(
								"settings.action-modals.email-change.description-default-admin",
							)
						: $t("settings.action-modals.email-change.description")
			}}
		</ShadcnUiDialogDescription>
		<form
			class="mt-5 flex w-full flex-col items-center gap-5"
			autocomplete="off"
			@submit="onSubmit"
		>
			<ShadcnUiFormField
				v-slot="{ componentField }"
				name="email"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{ $t("settings.action-modals.email-change.new-email-label") }}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="email"
							:placeholder="
								$t('settings.action-modals.email-change.new-email-placeholder')
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
			<ShadcnUiFormField
				v-if="!isEmailEnabled && hasPassword"
				v-slot="{ componentField }"
				name="password"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{ $t("settings.action-modals.email-change.password-label") }}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="password"
							autocomplete="current-password"
							:placeholder="
								$t('settings.action-modals.email-change.password-placeholder')
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
					{{ $t("settings.action-modals.email-change.submit-button") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="sm"
					variant="secondary"
					:disabled="loading"
					class="text-2sm"
					@click="emit('close')"
				>
					{{ $t("settings.action-modals.email-change.cancel-button") }}
				</ShadcnUiButton>
			</div>
		</form>
	</div>
</template>
