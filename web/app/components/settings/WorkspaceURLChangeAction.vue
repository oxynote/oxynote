<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import { showToastMessage } from "../toast"

const emit = defineEmits<{
	(event: "close" | "refresh-organization-slug"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { fetchOrganization, checkOrganizationSlug, updateOrganization } =
	useAuthSession()
const formSchema = toTypedSchema(
	z.object({
		url: z
			.string()
			.trim()
			.min(2)
			.max(50)
			.regex(/^[a-zA-Z0-9-_]+$/, {
				message: t(
					"settings.action-modals.workspace-url-change.errors.url-regex",
				),
			})
			.default(fetchOrganization.state.value.data?.data?.slug || ""),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref(false)

const onSubmit = form.handleSubmit(async (values) => {
	if (values.url === fetchOrganization.state.value.data?.data?.slug) {
		emit("close")
		return
	}

	loading.value = true
	await delay(300) // show loading spinner for at least a moment

	const res = (await checkOrganizationSlug({
		slug: values.url,
	})) as AuthResponse
	if (res.error) {
		loading.value = false

		if (res.error.status === 400) {
			form.setFieldError(
				"url",
				t(
					"settings.action-modals.workspace-url-change.errors.workspace-url-taken",
				),
			)
			return
		}

		form.setFieldError(
			"url",
			t(
				"settings.action-modals.workspace-url-change.errors.workspace-url-invalid",
			),
		)

		return
	}

	const { error } = (await updateOrganization({
		data: {
			slug: values.url,
		},
		organizationId: fetchOrganization.state.value.data?.data?.id || "",
	})) as AuthResponse
	if (error) {
		form.setErrors({ url: error.message })
		loading.value = false

		return
	}

	await fetchOrganization.refetch()

	showToastMessage(
		"success",
		t("settings.action-modals.workspace-url-change.success-message.title"),
	)
	emit("close")
	emit("refresh-organization-slug")
})
</script>
<template>
	<div class="flex flex-col">
		<p class="text-2sm text-muted-foreground">
			{{ $t("settings.action-modals.workspace-url-change.description") }}
		</p>
		<form
			class="mt-5 flex w-full flex-col items-center gap-5"
			autocomplete="off"
			@submit="onSubmit"
		>
			<ShadcnUiFormField v-slot="{ componentField }" name="url" class="w-full">
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{
							$t("settings.action-modals.workspace-url-change.new-url-label")
						}}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<div class="relative">
							<div
								class="absolute top-1/2 left-3 -translate-y-1/2 text-2sm text-muted-foreground"
							>
								{{ $t("settings.workspace.url-prefix") }}
							</div>
							<ShadcnUiInput
								type="text"
								:placeholder="fetchOrganization.state.value.data?.data?.slug"
								disable-focus-effect
								disable-destructive-effect
								:class="
									cn('h-8 pl-19.75 text-2sm!', loading && 'pointer-events-none')
								"
								v-bind="{
									...componentField,
								}"
							/>
						</div>
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
					{{ $t("settings.action-modals.workspace-url-change.submit-button") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="sm"
					variant="secondary"
					:disabled="loading"
					class="text-2sm"
					@click="emit('close')"
				>
					{{ $t("settings.action-modals.workspace-url-change.cancel-button") }}
				</ShadcnUiButton>
			</div>
		</form>
	</div>
</template>
