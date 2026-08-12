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

const { fetchAuthSession, hasPassword, changePassword, requestPasswordReset } =
	useAuthSession()
const config = useRuntimeConfig()
const { t } = useI18n({ useScope: "global" })

const formSchema = toTypedSchema(
	z
		.object({
			currentPassword: z.string().min(1, {
				message: t(
					"settings.action-modals.password-change.errors.current-password-required",
				),
			}),
			newPassword: z
				.string()
				.min(16)
				.max(128)
				.regex(/[0-9]/, {
					message: t(
						"settings.action-modals.password-change.errors.password-number",
					),
				})
				.regex(/[^a-zA-Z0-9]/, {
					message: t(
						"settings.action-modals.password-change.errors.password-symbol",
					),
				}),
			confirmPassword: z.string(),
		})
		.refine((data) => data.newPassword === data.confirmPassword, {
			message: t(
				"settings.action-modals.password-change.errors.password-mismatch",
			),
			path: ["confirmPassword"],
		}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref(false)

const onChangeSubmit = form.handleSubmit(async (values) => {
	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const { error } = await changePassword({
		currentPassword: values.currentPassword,
		newPassword: values.newPassword,
		revokeOtherSessions: true,
	})
	if (error) {
		loading.value = false

		if ((error as any).code === "INVALID_PASSWORD") {
			form.setFieldError(
				"currentPassword",
				t(
					"settings.action-modals.password-change.errors.invalid-current-password",
				),
			)
			return
		}

		showToastMessage(
			"error",
			t("settings.action-modals.password-change.errors.change-failed.title"),
			t(
				"settings.action-modals.password-change.errors.change-failed.description",
			),
		)

		return
	}

	showToastMessage(
		"success",
		t("settings.action-modals.password-change.success-message.changed.title"),
		t(
			"settings.action-modals.password-change.success-message.changed.description",
		),
	)
	emit("close")
})

// oauth-only accounts have no current password to prove, so better-auth
// keeps set-password off the client API — the password-reset email flow is
// the intended way to attach one, and its reset endpoint creates the
// credential account when it's missing.
async function onSetSubmit() {
	const email = fetchAuthSession.state.value.data?.data?.user.email
	if (!email) {
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const { error } = await requestPasswordReset({
		email,
		redirectTo: `${config.public.appBaseURL}/reset-password`,
	})
	if (error) {
		loading.value = false
		showToastMessage(
			"error",
			t("settings.action-modals.password-change.errors.link-failed.title"),
			t(
				"settings.action-modals.password-change.errors.link-failed.description",
			),
		)

		return
	}

	showToastMessage(
		"success",
		t("settings.action-modals.password-change.success-message.link-sent.title"),
		t(
			"settings.action-modals.password-change.success-message.link-sent.description",
			{ email },
		),
	)
	emit("close")
}
</script>
<template>
	<div class="flex flex-col">
		<template v-if="hasPassword">
			<p class="text-2sm text-muted-foreground">
				{{ $t("settings.action-modals.password-change.description") }}
			</p>
			<form
				class="mt-5 flex w-full flex-col gap-5"
				autocomplete="off"
				@submit="onChangeSubmit"
			>
				<ShadcnUiFormField
					v-slot="{ componentField }"
					name="currentPassword"
					class="w-full"
				>
					<ShadcnUiFormItem class="w-full">
						<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
							{{
								$t(
									"settings.action-modals.password-change.current-password-label",
								)
							}}
						</ShadcnUiFormLabel>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="current-password"
								:placeholder="
									$t(
										'settings.action-modals.password-change.current-password-placeholder',
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
				<ShadcnUiFormField
					v-slot="{ componentField, meta }"
					name="newPassword"
					:validate-on-model-update="false"
					:validate-on-input="false"
					:validate-on-change="false"
					:validate-on-blur="true"
				>
					<ShadcnUiFormItem class="w-full">
						<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
							{{
								$t("settings.action-modals.password-change.new-password-label")
							}}
						</ShadcnUiFormLabel>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="new-password"
								:placeholder="
									$t(
										'settings.action-modals.password-change.new-password-placeholder',
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
						<ShadcnUiFormMessage
							v-if="meta.touched || meta.validated"
							class="text-2xs"
						/>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField
					v-slot="{ componentField, meta }"
					name="confirmPassword"
					:validate-on-model-update="false"
					:validate-on-input="false"
					:validate-on-change="false"
					:validate-on-blur="true"
				>
					<ShadcnUiFormItem class="w-full">
						<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
							{{
								$t(
									"settings.action-modals.password-change.confirm-password-label",
								)
							}}
						</ShadcnUiFormLabel>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="new-password"
								:placeholder="
									$t(
										'settings.action-modals.password-change.confirm-password-placeholder',
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
						<ShadcnUiFormMessage
							v-if="meta.touched || meta.validated"
							class="text-2xs"
						/>
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
						{{ $t("settings.action-modals.password-change.submit-button") }}
					</ShadcnUiButton>
					<ShadcnUiButton
						type="button"
						size="sm"
						variant="secondary"
						:disabled="loading"
						class="text-2sm"
						@click="emit('close')"
					>
						{{ $t("settings.action-modals.password-change.cancel-button") }}
					</ShadcnUiButton>
				</div>
			</form>
		</template>
		<template v-else>
			<i18n-t
				keypath="settings.action-modals.password-change.description-set"
				tag="p"
				class="text-2sm text-muted-foreground"
			>
				<template #email>
					<span class="font-semibold">
						{{ fetchAuthSession.state.value.data?.data?.user.email }}
					</span>
				</template>
			</i18n-t>
			<div class="mt-5 flex w-full gap-2">
				<ShadcnUiButton
					type="button"
					size="sm"
					:disabled="loading"
					class="text-2sm"
					@click="onSetSubmit"
				>
					<Icon
						v-show="loading"
						name="svg-spinners:blocks-shuffle-3"
						class="size-3"
					/>
					{{ $t("settings.action-modals.password-change.send-link-button") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="sm"
					variant="secondary"
					:disabled="loading"
					class="text-2sm"
					@click="emit('close')"
				>
					{{ $t("settings.action-modals.password-change.cancel-button") }}
				</ShadcnUiButton>
			</div>
		</template>
	</div>
</template>
