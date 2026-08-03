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
const { fetchAuthSession, changeEmail } = useAuthSession()
const config = useRuntimeConfig()
const { t } = useI18n({ useScope: "global" })
const loading = ref(false)

const onSubmit = form.handleSubmit(async (values) => {
	if (values.email === fetchAuthSession.state.value.data?.data?.user.email) {
		emit("close")
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const { error } = await changeEmail({
		newEmail: values.email,
		callbackURL: postEmailVerificationUrl(
			config.public.appBaseURL,
			values.email,
		),
	})
	if (error) {
		form.setErrors({ email: error.message })
		loading.value = false

		return
	}

	await fetchAuthSession.refetch()

	showToastMessage(
		"success",
		t("settings.action-modals.email-change.success-message.title"),
		t("settings.action-modals.email-change.success-message.description", {
			email: values.email,
		}),
	)
	emit("close")
})
</script>
<template>
	<div class="flex flex-col">
		<p class="text-2sm text-muted-foreground">
			{{ $t("settings.action-modals.email-change.description") }}
		</p>
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
