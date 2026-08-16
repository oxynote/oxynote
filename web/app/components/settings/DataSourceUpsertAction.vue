<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import { showToastMessage } from "../toast"

const props = defineProps<{
	creationType?: DataSourceType
	updateTarget?: DataSource
}>()
const emit = defineEmits<{
	(event: "close"): void
}>()

const isCreation = computed(() => !!props.creationType && !props.updateTarget)
const { t } = useI18n({ useScope: "global" })
const config = useRuntimeConfig()
const formSchema = toTypedSchema(
	z.object({
		name: z
			.string()
			.trim()
			.min(1)
			.max(128)
			.default(isCreation.value ? "" : props.updateTarget?.name || ""),
		url: z
			.string()
			.trim()
			.url()
			.default(isCreation.value ? "" : props.updateTarget?.url || ""),
		username: z.string().trim().max(128).optional(),
		password: z.string().trim().max(128).optional(),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref(false)
const readOnlyWarning = computed(() => {
	const tp = isCreation.value ? props.creationType : props.updateTarget?.type
	let active = false
	let link = ""

	switch (tp) {
		case DataSourceType.PostgreSQL:
			active = true
			link = config.public.postgresqlReadOnlyUserSetupGuideURL
			break
		case DataSourceType.MySQL:
			active = true
			link = config.public.mysqlReadOnlyUserSetupGuideURL
			break
		case DataSourceType.MariaDB:
			active = true
			link = config.public.mariadbReadOnlyUserSetupGuideURL
			break
	}

	return {
		active: active,
		link: link,
	}
})
const dataSourceType = computed(() => {
	if (isCreation.value && props.creationType) {
		return props.creationType
	} else if (props.updateTarget) {
		return props.updateTarget.type
	}

	return null
})

const { createDataSource, updateDataSource } = useDataSourceAPI()

const onSubmit = form.handleSubmit(async (values) => {
	loading.value = true
	await delay(300) // slight delay to ensure loading spinner is visible

	if (isCreation.value && props.creationType) {
		try {
			await createDataSource.mutateAsync({
				type: props.creationType,
				name: values.name,
				url: values.url,
				credentials: {
					username: values.username || "",
					password: values.password || "",
				},
			})
		} catch (err) {
			loading.value = false

			// the mutation rejects with an ofetch FetchError, whose `data` holds the
			// core API's JSON body
			const { data } = err as { data?: { code?: string } }
			const statusParts = data?.code?.split(".")
			const status =
				statusParts && statusParts.length > 1 ? statusParts[1] : undefined

			if (
				status &&
				DataSourceErrorStatuses.map((v) => v as string).includes(status)
			) {
				showToastMessage(
					"error",
					t(
						`settings.action-modals.data-source-upsert.error-message.status.${status}.title.creation`,
						{
							name: values.name,
						},
					),
					t(
						`settings.action-modals.data-source-upsert.error-message.status.${status}.description`,
					),
				)

				return
			}

			showToastMessage(
				"error",
				t(
					"settings.action-modals.data-source-upsert.error-message.creation.generic.title",
					{
						name: values.name,
					},
				),
				t(
					"settings.action-modals.data-source-upsert.error-message.creation.generic.description",
					{
						name: values.name,
					},
				),
			)

			return
		}

		showToastMessage(
			"success",
			t(
				"settings.action-modals.data-source-upsert.success-message.creation.title",
				{
					name: values.name,
				},
			),
			t(
				"settings.action-modals.data-source-upsert.success-message.creation.description",
			),
		)
	} else if (props.updateTarget) {
		const req: DataSourceUpdateRequest = {}

		if (values.name !== props.updateTarget.name) {
			req.name = values.name
		}

		if (values.url !== props.updateTarget.url) {
			req.url = values.url
		}

		if (values.username) {
			req.credentials ??= {}
			req.credentials.username = values.username
		}

		if (values.password) {
			req.credentials ??= {}
			req.credentials.password = values.password
		}

		if (Object.keys(req).length === 0) {
			loading.value = false
			emit("close")

			return
		}

		try {
			await updateDataSource.mutateAsync({
				dataSourceId: props.updateTarget.id,
				req: req,
			})
		} catch (err) {
			loading.value = false

			// the mutation rejects with an ofetch FetchError, whose `data` holds the
			// core API's JSON body
			const { data } = err as { data?: { code?: string } }
			const statusParts = data?.code?.split(".")
			const status =
				statusParts && statusParts.length > 1 ? statusParts[1] : undefined

			if (
				status &&
				DataSourceErrorStatuses.map((v) => v as string).includes(status)
			) {
				showToastMessage(
					"error",
					t(
						`settings.action-modals.data-source-upsert.error-message.status.${status}.title.update`,
						{
							name: values.name,
						},
					),
					t(
						`settings.action-modals.data-source-upsert.error-message.status.${status}.description`,
					),
				)

				return
			}

			showToastMessage(
				"error",
				t(
					"settings.action-modals.data-source-upsert.error-message.update.generic.title",
					{
						name: values.name,
					},
				),
				t(
					"settings.action-modals.data-source-upsert.error-message.update.generic.description",
					{
						name: values.name,
					},
				),
			)

			return
		}

		showToastMessage(
			"success",
			t(
				"settings.action-modals.data-source-upsert.success-message.update.title",
				{
					name: values.name,
				},
			),
			t(
				"settings.action-modals.data-source-upsert.success-message.update.description",
			),
		)
	}

	emit("close")
})
</script>
<template>
	<div class="flex flex-col">
		<p class="text-2sm text-muted-foreground">
			{{
				isCreation
					? $t(
							`settings.action-modals.data-source-upsert.description.creation.${dataSourceType}`,
						)
					: $t(
							`settings.action-modals.data-source-upsert.description.update.${dataSourceType}`,
						)
			}}
		</p>
		<form
			class="mt-5 flex w-full flex-col items-center gap-5"
			autocomplete="off"
			@submit="onSubmit"
		>
			<ShadcnUiFormField v-slot="{ componentField }" name="name" class="w-full">
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{
							$t(
								`settings.action-modals.data-source-upsert.name-field.${dataSourceType}.label`,
							)
						}}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:placeholder="
								$t(
									`settings.action-modals.data-source-upsert.name-field.${dataSourceType}.placeholder`,
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
			<ShadcnUiFormField v-slot="{ componentField }" name="url" class="w-full">
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						{{
							$t(
								`settings.action-modals.data-source-upsert.url-field.${dataSourceType}.label`,
							)
						}}
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:placeholder="
								$t(
									`settings.action-modals.data-source-upsert.url-field.${dataSourceType}.placeholder`,
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
				v-slot="{ componentField }"
				name="username"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						<span class="flex items-center">
							<span>
								{{
									$t(
										`settings.action-modals.data-source-upsert.creds-username-field.${dataSourceType}.label`,
									)
								}}
							</span>
							<span class="ml-0.75 text-2xs text-muted-foreground">
								{{ $t("settings.optional-label") }}
							</span>
						</span>
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:placeholder="
								isCreation
									? $t(
											`settings.action-modals.data-source-upsert.creds-username-field.${dataSourceType}.placeholder`,
										)
									: $t(
											'settings.action-modals.data-source-upsert.hidden-placeholder',
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
				v-slot="{ componentField }"
				name="password"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
						<span class="flex items-center">
							<span>
								{{
									$t(
										`settings.action-modals.data-source-upsert.creds-password-field.${dataSourceType}.label`,
									)
								}}
							</span>
							<span class="ml-0.75 text-2xs text-muted-foreground">
								{{ $t("settings.optional-label") }}
							</span>
						</span>
					</ShadcnUiFormLabel>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="password"
							:placeholder="
								isCreation
									? $t(
											`settings.action-modals.data-source-upsert.creds-password-field.${dataSourceType}.placeholder`,
										)
									: $t(
											'settings.action-modals.data-source-upsert.hidden-placeholder',
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
					{{ $t("settings.action-modals.data-source-upsert.submit-button") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="sm"
					variant="secondary"
					:disabled="loading"
					class="text-2sm"
					@click="emit('close')"
				>
					{{ $t("settings.action-modals.data-source-upsert.cancel-button") }}
				</ShadcnUiButton>
			</div>
			<div
				v-if="readOnlyWarning.active"
				class="w-full rounded-md border border-status-warning/30 bg-status-warning/5 px-3 pt-1 pb-1.5"
			>
				<i18n-t
					:keypath="`settings.action-modals.data-source-upsert.read-only-warning.${dataSourceType}`"
					tag="div"
					class="text-2sm leading-4"
				>
					<template #link>
						<a
							:href="readOnlyWarning.link"
							target="_blank"
							rel="noopener noreferrer"
							class="link-primary"
						>
							{{
								t(
									`settings.action-modals.data-source-upsert.read-only-warning.${dataSourceType}-link`,
								)
							}}
						</a>
					</template>
				</i18n-t>
			</div>
		</form>
	</div>
</template>
