<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"

definePageMeta({
	name: "reset-password",
	skipAuth: true,
	middleware: (to) => {
		if (!to.query.token || typeof to.query.token !== "string") {
			return navigateTo("/")
		}
	},
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.reset-password-page-title"),
})

const { resetPassword } = useAuthSession()

const route = useRoute()

const formSchema = toTypedSchema(
	z
		.object({
			password: z
				.string()
				.min(16)
				.max(128)
				.regex(/[0-9]/, {
					message: t("onboarding.reset-password.errors.password-number"),
				})
				.regex(/[^a-zA-Z0-9]/, {
					message: t("onboarding.reset-password.errors.password-symbol"),
				}),
			confirmPassword: z.string(),
		})
		.refine((data) => data.password === data.confirmPassword, {
			message: t("onboarding.reset-password.errors.password-mismatch"),
			path: ["confirmPassword"],
		}),
)
const form = useForm({
	validationSchema: formSchema,
})

const loading = ref(false)

const onSubmit = form.handleSubmit(async (values) => {
	loading.value = true

	const res = await resetPassword({
		newPassword: values.password,
		token: route.query.token as string,
	})

	if (res.error) {
		loading.value = false
		showToastMessage(
			"error",
			t("onboarding.reset-password.errors.reset-failed"),
		)

		return
	}

	// loading stays set so the spinner shows while redirecting
	void navigateTo({ path: "/login", query: { reset: "true" } })
})
</script>
<template>
	<main
		class="flex min-h-svh min-w-svw items-center justify-center bg-background text-foreground"
	>
		<div class="flex w-67 flex-col items-center gap-5">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-lg font-semibold">
					{{ $t("onboarding.reset-password.title") }}
				</div>
			</div>
			<form class="flex w-full flex-col gap-3" @submit="onSubmit">
				<ShadcnUiFormField
					v-slot="{ componentField, meta }"
					name="password"
					:validate-on-model-update="false"
					:validate-on-input="false"
					:validate-on-change="false"
					:validate-on-blur="true"
				>
					<ShadcnUiFormItem>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="new-password"
								:placeholder="
									$t('onboarding.reset-password.password-placeholder')
								"
								disable-focus-effect
								disable-destructive-effect
								:class="
									cn('h-10 text-2base!', loading && 'pointer-events-none')
								"
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
					<ShadcnUiFormItem>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="new-password"
								:placeholder="
									$t('onboarding.reset-password.confirm-placeholder')
								"
								disable-focus-effect
								disable-destructive-effect
								:class="
									cn('h-10 text-2base!', loading && 'pointer-events-none')
								"
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
				<ShadcnUiButton
					type="submit"
					size="lg"
					class="h-10 w-full"
					:disabled="loading"
				>
					<Icon
						v-show="loading"
						name="svg-spinners:blocks-shuffle-3"
						class="size-4"
					/>
					{{ $t("onboarding.reset-password.continue") }}
				</ShadcnUiButton>
			</form>
		</div>
	</main>
</template>
