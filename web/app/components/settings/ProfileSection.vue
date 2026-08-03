<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import { showToastMessage } from "../toast"

const emit = defineEmits<{
	(event: "email-change" | "account-deletion"): void
}>()

const { fetchAuthSession, fetchOrganization, updateUser } = useAuthSession()
const {
	fetchSlackConnectionStatus,
	fetchSlackUserLinkSettings,
	updateSlackUserLinkSettings,
} = useSlackAPI()
const { t } = useI18n({ useScope: "global" })

const formSchema = toTypedSchema(
	z.object({
		imageUrl: z
			.string()
			.trim()
			.default(fetchAuthSession.state.value.data?.data?.user.image || ""),
		username: z
			.string()
			.trim()
			.min(2)
			.max(50)
			.regex(/^[a-zA-Z0-9-_]+$/, {
				message: t("settings.profile.errors.username-regex"),
			})
			.default(fetchAuthSession.state.value.data?.data?.user.name || ""),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const { uploadUserImage } = useUserAPI()
const loading = ref<"avatar" | null>(null)
const avatarInput = useTemplateRef("avatarInput")
const slackNotificationsEnabled = ref(false)
const slackNotificationsSwitchActive = refAutoReset(true, 500) // false - disabled (we use this to have a smoother loading state/animation)

const onSubmit = form.handleSubmit(async (values) => {
	const data: { image?: string; name?: string } = {}

	if (values.imageUrl !== fetchAuthSession.state.value.data?.data?.user.image) {
		data.image = values.imageUrl
	}

	if (values.username !== fetchAuthSession.state.value.data?.data?.user.name) {
		data.name = values.username
	}

	if (Object.keys(data).length === 0) {
		return
	}

	const { error } = await updateUser(data)
	if (error) {
		loading.value = null

		if (data.image) {
			showToastMessage(
				"error",
				t("settings.profile.errors.toasts.image.title"),
				t("settings.profile.errors.toasts.image.description"),
			)
		}

		if (data.name) {
			showToastMessage(
				"error",
				t("settings.profile.errors.toasts.username.title"),
				t("settings.profile.errors.toasts.username.description"),
			)
		}

		return
	}

	await fetchAuthSession.refetch()

	// We must also refetch organization as we're using members object which
	// directly maps to users.
	await fetchOrganization.refetch()

	loading.value = null

	if (data.image) {
		showToastMessage(
			"success",
			t("settings.profile.success-messages.image.title"),
		)
	}

	if (data.name) {
		showToastMessage(
			"success",
			t("settings.profile.success-messages.username.title"),
		)
	}
})

const { ignoreUpdates } = watchIgnorable(
	slackNotificationsEnabled,
	async (newValue, oldValue) => {
		if (updateSlackUserLinkSettings.isLoading.value) {
			return
		}

		slackNotificationsSwitchActive.value = false

		try {
			await updateSlackUserLinkSettings.mutateAsync({
				notifications: newValue,
			})
			showToastMessage(
				"success",
				t("settings.profile.success-messages.slack-notifications.title"),
			)
		} catch {
			ignoreUpdates(() => {
				slackNotificationsEnabled.value = oldValue
			})
			showToastMessage(
				"error",
				t("settings.profile.errors.toasts.slack-notifications.title"),
				t("settings.profile.errors.toasts.slack-notifications.description"),
			)
		}
	},
	{ flush: "post" },
)

async function handleAvatarChange(event: Event) {
	const input = event.target as HTMLInputElement | null
	const file = input?.files?.[0]

	if (input) {
		input.value = ""
	}

	if (!file) {
		return
	}

	loading.value = "avatar"

	try {
		const uploadedUrl = await uploadUserImage.mutateAsync(file)
		form.setFieldValue("imageUrl", uploadedUrl)
		await onSubmit()
	} catch (error) {
		if (!handleStorageError(error, t)) {
			showToastMessage(
				"error",
				t("settings.profile.errors.toasts.image.title"),
				t("settings.profile.errors.toasts.image.description"),
			)
		}
	} finally {
		loading.value = null
	}
}

function handleAvatarClick() {
	if (loading.value === "avatar") {
		return
	}

	avatarInput.value?.click()
}
</script>
<template>
	<form class="flex flex-col" autocomplete="off" @submit.prevent="onSubmit">
		<ShadcnUiFormField v-slot="{ componentField }" name="imageUrl">
			<ShadcnUiFormItem class="flex w-full items-center justify-between gap-2">
				<div class="flex flex-col gap-0.5">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2base">
						{{ $t("settings.profile.image-label") }}
					</ShadcnUiFormLabel>
					<div class="text-xs text-muted-foreground">
						{{ $t("settings.profile.image-description") }}
					</div>
				</div>
				<ShadcnUiFormControl>
					<ShadcnUiAvatar
						class="group/avatar size-8.5 cursor-pointer border"
						@click="handleAvatarClick"
					>
						<ShadcnUiAvatarImage
							:src="componentField.modelValue"
							:alt="$t('settings.profile.image-alt')"
						/>
						<ShadcnUiAvatarFallback>
							{{ extractInitials(form.values.username || "", 2) }}
						</ShadcnUiAvatarFallback>
						<div
							:data-loading="loading === 'avatar' ? '' : undefined"
							class="absolute top-1/2 left-1/2 z-10 flex size-full -translate-x-1/2 -translate-y-1/2 items-center justify-center bg-black/40 opacity-0 transition group-hover/avatar:opacity-100 data-loading:opacity-100 dark:bg-white/50"
						>
							<Icon
								:name="
									loading === 'avatar'
										? 'svg-spinners:blocks-shuffle-3'
										: 'lucide:pencil'
								"
								class="text-muted"
								size="1rem"
							/>
						</div>
					</ShadcnUiAvatar>
					<input
						ref="avatarInput"
						type="file"
						accept="image/png,image/jpeg,image/webp"
						class="hidden"
						@change="handleAvatarChange"
					/>
				</ShadcnUiFormControl>
			</ShadcnUiFormItem>
		</ShadcnUiFormField>
		<div class="my-3.5 h-px w-full bg-border" />
		<div
			class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
		>
			<div class="flex flex-col gap-0.5">
				<div class="text-2base">
					{{ $t("settings.profile.email-label") }}
				</div>
				<div class="text-xs text-muted-foreground">
					{{ $t("settings.profile.email-description") }}
				</div>
			</div>
			<div
				class="flex w-full items-center justify-between gap-1.5 sm:max-w-44 sm:min-w-44 sm:justify-end md:max-w-52 md:min-w-0 lg:max-w-70"
			>
				<div :class="cn('min-w-0 truncate text-2sm')">
					{{ fetchAuthSession.state.value.data?.data?.user.email }}
				</div>
				<ShadcnUiButton
					type="button"
					class="size-6 shrink-0 bg-transparent p-0"
					variant="outline"
					@click="emit('email-change')"
				>
					<Icon name="lucide:pencil" size="0.75rem" />
					<span class="sr-only">
						{{ $t("settings.profile.email-change-button-screen-reader-hint") }}
					</span>
				</ShadcnUiButton>
			</div>
		</div>
		<div class="my-3.5 h-px w-full bg-border" />
		<ShadcnUiFormField v-slot="{ componentField }" name="username">
			<ShadcnUiFormItem
				class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
			>
				<div class="flex flex-col gap-0.5">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2base">
						{{ $t("settings.profile.username-label") }}
					</ShadcnUiFormLabel>
					<div class="text-xs text-muted-foreground">
						{{ $t("settings.profile.username-description") }}
					</div>
				</div>
				<div class="w-full shrink-0 sm:w-44 md:w-52">
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:placeholder="$t('settings.profile.username-placeholder')"
							disable-focus-effect
							disable-destructive-effect
							:class="cn('h-8 w-full text-2sm!')"
							v-bind="{
								...componentField,
							}"
							@blur="onSubmit()"
						/>
					</ShadcnUiFormControl>
					<ShadcnUiFormMessage class="text-2xs" />
				</div>
			</ShadcnUiFormItem>
		</ShadcnUiFormField>
		<div class="my-3.5 h-px w-full bg-border" />
		<div
			class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
		>
			<div class="flex flex-col gap-0.5">
				<div class="text-2base">
					{{ $t("settings.profile.account-deletion-label") }}
				</div>
				<div class="text-xs text-muted-foreground">
					{{ $t("settings.profile.account-deletion-description") }}
				</div>
			</div>
			<div
				class="flex w-full items-center justify-between gap-1.5 sm:max-w-44 sm:min-w-44 sm:justify-end md:max-w-52 md:min-w-0 lg:max-w-70"
			>
				<ShadcnUiButton
					type="button"
					class="shrink-0 text-2sm"
					variant="ghost-plain-destructive"
					@click="emit('account-deletion')"
				>
					<div>
						{{ $t("settings.profile.account-deletion-button") }}
					</div>
				</ShadcnUiButton>
			</div>
		</div>
		<template v-if="fetchSlackConnectionStatus.state.value.data?.connected">
			<div class="my-3.5 h-px w-full bg-border" />
			<div
				class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
			>
				<div class="flex flex-col gap-0.5">
					<div class="text-2base">
						{{ $t("settings.profile.slack-notification-label") }}
					</div>
					<div
						v-if="fetchSlackUserLinkSettings.state.value.data"
						class="text-xs text-muted-foreground"
					>
						{{ $t("settings.profile.slack-notification-description") }}
					</div>
					<i18n-t
						v-else
						keypath="settings.profile.slack-notification-unlinked-description.main"
						tag="div"
						class="text-xs text-muted-foreground"
					>
						<template #command>
							<span class="font-semibold">
								{{
									$t(
										"settings.profile.slack-notification-unlinked-description.command",
									)
								}}
							</span>
						</template>
					</i18n-t>
				</div>
				<div
					class="flex w-full items-center justify-between gap-1.5 sm:max-w-44 sm:min-w-44 sm:justify-end md:max-w-52 md:min-w-0 lg:max-w-70"
				>
					<ShadcnUiSwitch
						v-model="slackNotificationsEnabled"
						class="mt-0.5"
						:disabled="
							!fetchSlackUserLinkSettings.state.value.data ||
							!slackNotificationsSwitchActive
						"
					/>
				</div>
			</div>
		</template>
	</form>
</template>
