<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "#layers/ui/app/components/shadcn/ui/form"
import { cn } from "#layers/ui/app/lib/utils"
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
		caCertificate: z.string().trim().max(65536).optional(),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref(false)
const resetCredentials = ref<DataSourceCredentialKey[]>([])
// stored credentials belong to the stored URL, and a new URL clears them
const urlChanged = computed(
	() =>
		!isCreation.value && form.values.url?.trim() !== props.updateTarget?.url,
)
const hasStoredCredentials = computed(
	() => !isCreation.value && !urlChanged.value,
)
const readOnlyWarning = computed(() => {
	const tp = isCreation.value ? props.creationType : props.updateTarget?.type
	let active = false
	let link = ""

	switch (tp) {
		case DataSourceType.PostgreSQL:
			active = true
			link = config.public.postgresqlReadOnlyUserSetupGuideURL
			break
		case DataSourceType.Supabase:
			active = true
			link = config.public.supabaseReadOnlyUserSetupGuideURL
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
		const credentials: DataSourceCredentials = {
			username: values.username || "",
			password: values.password || "",
		}

		if (values.caCertificate) {
			credentials.caCertificate = values.caCertificate
		}

		try {
			await createDataSource.mutateAsync({
				type: props.creationType,
				name: values.name,
				url: values.url,
				credentials: credentials,
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

		for (const key of ["username", "password", "caCertificate"] as const) {
			const value = values[key]
			if (value) {
				req.credentials ??= {}
				req.credentials[key] = value
			} else if (
				hasStoredCredentials.value &&
				resetCredentials.value.includes(key)
			) {
				req.credentials ??= {}
				req.credentials[key] = ""
			}
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

// the server says which credentials are stored but never sends them, so a
// stored one stays locked until it is reset
function isCredentialLocked(key: DataSourceCredentialKey) {
	return (
		hasStoredCredentials.value &&
		!!props.updateTarget?.configured?.[key] &&
		!resetCredentials.value.includes(key)
	)
}

function resetCredential(key: DataSourceCredentialKey) {
	resetCredentials.value.push(key)
}
</script>
<template>
	<div class="flex flex-col">
		<ShadcnUiDialogDescription class="text-2sm">
			{{
				isCreation
					? $t(
							`settings.action-modals.data-source-upsert.description.creation.${dataSourceType}`,
						)
					: $t(
							`settings.action-modals.data-source-upsert.description.update.${dataSourceType}`,
						)
			}}
		</ShadcnUiDialogDescription>
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
					<div class="flex items-center justify-between">
						<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
							{{
								$t(
									`settings.action-modals.data-source-upsert.url-field.${dataSourceType}.label`,
								)
							}}
						</ShadcnUiFormLabel>
						<ShadcnUiPopover v-if="dataSourceType === DataSourceType.Supabase">
							<ShadcnUiPopoverTrigger as-child>
								<ShadcnUiButton
									type="button"
									variant="dim"
									size="custom"
									class="text-2xs"
								>
									{{
										$t("settings.action-modals.data-source-upsert.help-button")
									}}
								</ShadcnUiButton>
							</ShadcnUiPopoverTrigger>
							<ShadcnUiPopoverContent
								class="flex w-130 max-w-[85dvw] flex-col gap-1.5 px-2 pt-1 pb-1.5 text-2sm leading-4.5 md:max-w-[50dvw]"
								side="bottom"
								side-flip
								inside-modal
								align="end"
							>
								<i18n-t
									scope="global"
									keypath="settings.action-modals.data-source-upsert.url-field.supabase-help.steps"
									tag="p"
								>
									<template #connect>
										<span class="font-semibold">
											{{
												$t(
													"settings.action-modals.data-source-upsert.url-field.supabase-help.placeholders.connect",
												)
											}}
										</span>
									</template>
									<template #pooler>
										<span class="font-semibold">
											{{
												$t(
													"settings.action-modals.data-source-upsert.url-field.supabase-help.placeholders.pooler",
												)
											}}
										</span>
									</template>
									<template #credentials>
										<code class="inline-code">
											{{
												$t(
													"settings.action-modals.data-source-upsert.url-field.supabase-help.placeholders.credentials",
												)
											}}
										</code>
									</template>
									<template #url>
										<code class="inline-code">
											{{
												$t(
													"settings.action-modals.data-source-upsert.url-field.supabase-help.placeholders.url",
												)
											}}
										</code>
									</template>
									<template #user>
										<code class="inline-code">
											{{
												$t(
													"settings.action-modals.data-source-upsert.url-field.supabase-help.placeholders.user",
												)
											}}
										</code>
									</template>
								</i18n-t>
							</ShadcnUiPopoverContent>
						</ShadcnUiPopover>
					</div>
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
					<p v-if="urlChanged" class="text-2xs text-muted-foreground">
						{{
							$t(
								"settings.action-modals.data-source-upsert.url-field.credentials-reset",
							)
						}}
					</p>
				</ShadcnUiFormItem>
			</ShadcnUiFormField>
			<ShadcnUiFormField
				v-slot="{ componentField }"
				name="username"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<div class="flex items-center justify-between">
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
						<ShadcnUiButton
							v-if="isCredentialLocked('username')"
							type="button"
							variant="ghost-plain-destructive"
							size="custom"
							class="text-2xs"
							@click="resetCredential('username')"
						>
							{{ $t("settings.action-modals.data-source-upsert.reset-button") }}
						</ShadcnUiButton>
					</div>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:disabled="isCredentialLocked('username')"
							:placeholder="
								isCredentialLocked('username')
									? $t(
											'settings.action-modals.data-source-upsert.hidden-placeholder',
										)
									: $t(
											`settings.action-modals.data-source-upsert.creds-username-field.${dataSourceType}.placeholder`,
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
					<div class="flex items-center justify-between">
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
						<ShadcnUiButton
							v-if="isCredentialLocked('password')"
							type="button"
							variant="ghost-plain-destructive"
							size="custom"
							class="text-2xs"
							@click="resetCredential('password')"
						>
							{{ $t("settings.action-modals.data-source-upsert.reset-button") }}
						</ShadcnUiButton>
					</div>
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="password"
							:disabled="isCredentialLocked('password')"
							:placeholder="
								isCredentialLocked('password')
									? $t(
											'settings.action-modals.data-source-upsert.hidden-placeholder',
										)
									: $t(
											`settings.action-modals.data-source-upsert.creds-password-field.${dataSourceType}.placeholder`,
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
				name="caCertificate"
				class="w-full"
			>
				<ShadcnUiFormItem class="w-full">
					<div class="flex items-center justify-between">
						<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
							<span class="flex items-center">
								<span>
									{{
										$t(
											"settings.action-modals.data-source-upsert.creds-ca-field.label",
										)
									}}
								</span>
								<span class="ml-0.75 text-2xs text-muted-foreground">
									{{ $t("settings.optional-label") }}
								</span>
							</span>
						</ShadcnUiFormLabel>
						<div class="flex items-center gap-2">
							<ShadcnUiButton
								v-if="isCredentialLocked('caCertificate')"
								type="button"
								variant="ghost-plain-destructive"
								size="custom"
								class="text-2xs"
								@click="resetCredential('caCertificate')"
							>
								{{
									$t("settings.action-modals.data-source-upsert.reset-button")
								}}
							</ShadcnUiButton>
							<ShadcnUiPopover>
								<ShadcnUiPopoverTrigger as-child>
									<ShadcnUiButton
										type="button"
										variant="dim"
										size="custom"
										class="text-2xs"
									>
										{{
											$t(
												"settings.action-modals.data-source-upsert.help-button",
											)
										}}
									</ShadcnUiButton>
								</ShadcnUiPopoverTrigger>
								<ShadcnUiPopoverContent
									class="flex w-100 max-w-[85dvw] flex-col px-2 pt-1 pb-1.5 text-2sm leading-4.5"
									side="bottom"
									side-flip
									inside-modal
									align="end"
								>
									<p>
										{{
											$t(
												"settings.action-modals.data-source-upsert.creds-ca-field.help",
											)
										}}
									</p>
									<i18n-t
										v-if="dataSourceType === DataSourceType.Supabase"
										scope="global"
										keypath="settings.action-modals.data-source-upsert.creds-ca-field.supabase-help.steps"
										tag="p"
										class="mt-1.5"
									>
										<template #file>
											<code class="inline-code">
												{{
													$t(
														"settings.action-modals.data-source-upsert.creds-ca-field.supabase-help.placeholders.file",
													)
												}}
											</code>
										</template>
										<template #location>
											<span class="font-semibold">
												{{
													$t(
														"settings.action-modals.data-source-upsert.creds-ca-field.supabase-help.placeholders.location",
													)
												}}
											</span>
										</template>
									</i18n-t>
								</ShadcnUiPopoverContent>
							</ShadcnUiPopover>
						</div>
					</div>
					<ShadcnUiFormControl>
						<ShadcnUiTextarea
							:disabled="isCredentialLocked('caCertificate')"
							:placeholder="
								isCredentialLocked('caCertificate')
									? $t(
											'settings.action-modals.data-source-upsert.hidden-placeholder',
										)
									: $t(
											'settings.action-modals.data-source-upsert.creds-ca-field.placeholder',
										)
							"
							:class="
								cn(
									'max-h-32 min-h-14 text-2sm focus-visible:border-input focus-visible:ring-0 md:text-2sm',
									loading && 'pointer-events-none',
								)
							"
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
					scope="global"
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
