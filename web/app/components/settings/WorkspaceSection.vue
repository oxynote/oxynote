<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import type { OrganizationMember } from "./workspace"
import { showToastMessage } from "../toast"

const emit = defineEmits<{
	(event: "url-change" | "invitation"): void
	(event: "member-removal", member: OrganizationMember): void
}>()

const { fetchOrganization, fetchAuthSession, updateOrganization } =
	useAuthSession()
const { t } = useI18n({ useScope: "global" })
const { uploadOrganizationLogo } = useOrganizationAPI()

const formSchema = toTypedSchema(
	z.object({
		logoUrl: z
			.string()
			.trim()
			.default(fetchOrganization.state.value.data?.data?.logo || ""),
		name: z
			.string()
			.trim()
			.min(2)
			.max(50)
			.regex(/^[a-zA-Z0-9-_]+$/, {
				message: t("settings.workspace.errors.name-regex"),
			})
			.default(fetchOrganization.state.value.data?.data?.name || ""),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const members = computed<OrganizationMember[]>(() => {
	const members: OrganizationMember[] =
		fetchOrganization.state.value.data?.data?.members.map((m) => {
			return {
				id: m.id,
				organizationId: m.organizationId,
				createdAt: m.createdAt,
				userId: m.userId,
				role: m.role,
				user: { name: m.user.name, email: m.user.email, image: m.user.image },
			}
		}) ?? []
	const invs: OrganizationMember[] =
		fetchOrganization.state.value.data?.data?.invitations
			.filter((inv) => inv.status === "pending")
			.map((inv) => {
				return {
					id: inv.id,
					organizationId: inv.organizationId,
					invitationPending: true,
					role: inv.role,
					user: { name: extractNameFromEmail(inv.email), email: inv.email },
				}
			}) ?? []

	return members.concat(invs)
})
const loading = ref<"logo" | null>(null)
const logoInput = useTemplateRef<HTMLInputElement>("logoInput")

const onSubmit = form.handleSubmit(async (values) => {
	const data: { logo?: string; name?: string } = {}
	if (values.logoUrl !== fetchOrganization.state.value.data?.data?.logo) {
		data.logo = values.logoUrl
	}

	if (values.name !== fetchOrganization.state.value.data?.data?.name) {
		data.name = values.name
	}

	if (Object.keys(data).length === 0) {
		return
	}

	const { error } = (await updateOrganization({
		data: data,
		organizationId: fetchOrganization.state.value.data?.data?.id || "",
	})) as AuthResponse
	if (error) {
		loading.value = null

		if (data.logo) {
			showToastMessage(
				"error",
				t("settings.workspace.errors.toasts.logo.title"),
				t("settings.workspace.errors.toasts.logo.description"),
			)
		}

		if (data.name) {
			showToastMessage(
				"error",
				t("settings.workspace.errors.toasts.name.title"),
				t("settings.workspace.errors.toasts.name.description"),
			)
		}

		return
	}

	await fetchOrganization.refetch()

	loading.value = null

	if (data.logo) {
		showToastMessage(
			"success",
			t("settings.workspace.success-messages.logo.title"),
		)
	}

	if (data.name) {
		showToastMessage(
			"success",
			t("settings.workspace.success-messages.name.title"),
		)
	}
})

async function handleLogoChange(event: Event) {
	const input = event.target as HTMLInputElement | null
	const file = input?.files?.[0]

	if (input) {
		input.value = ""
	}

	if (!file) {
		return
	}

	loading.value = "logo"

	try {
		const uploadedUrl = await uploadOrganizationLogo.mutateAsync(file)
		form.setFieldValue("logoUrl", uploadedUrl)
		await onSubmit()
	} catch (error) {
		if (!handleStorageError(error, t)) {
			showToastMessage(
				"error",
				t("settings.workspace.errors.toasts.logo.title"),
				t("settings.workspace.errors.toasts.logo.description"),
			)
		}
	} finally {
		loading.value = null
	}
}

function handleAvatarClick() {
	if (loading.value === "logo") {
		return
	}

	logoInput.value?.click()
}

function handleMemberDelete(member: OrganizationMember) {
	emit("member-removal", member)
}
</script>
<template>
	<form class="flex flex-col" autocomplete="off" @submit.prevent="onSubmit">
		<ShadcnUiFormField v-slot="{ componentField }" name="logoUrl">
			<ShadcnUiFormItem class="flex w-full items-center justify-between gap-2">
				<div class="flex flex-col gap-0.5">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2base">
						{{ $t("settings.workspace.logo-label") }}
					</ShadcnUiFormLabel>
					<div class="text-xs text-muted-foreground">
						{{ $t("settings.workspace.logo-description") }}
					</div>
				</div>
				<ShadcnUiFormControl>
					<ShadcnUiAvatar
						class="group/avatar size-8.5 cursor-pointer rounded-md border"
						@click="handleAvatarClick"
					>
						<ShadcnUiAvatarImage
							:src="componentField.modelValue"
							:alt="$t('settings.workspace.logo-alt')"
						/>
						<ShadcnUiAvatarFallback class="rounded-md">
							{{ extractInitials(form.values.name || "", 2) }}
						</ShadcnUiAvatarFallback>
						<div
							:data-loading="loading === 'logo' ? '' : undefined"
							class="absolute top-1/2 left-1/2 z-10 flex size-full -translate-x-1/2 -translate-y-1/2 items-center justify-center bg-black/40 opacity-0 transition group-hover/avatar:opacity-100 data-loading:opacity-100 dark:bg-white/50"
						>
							<Icon
								:name="
									loading === 'logo'
										? 'svg-spinners:blocks-shuffle-3'
										: 'lucide:pencil'
								"
								class="text-muted"
								size="1rem"
							/>
						</div>
					</ShadcnUiAvatar>
					<input
						ref="logoInput"
						type="file"
						accept="image/png,image/jpeg,image/webp"
						class="hidden"
						@change="handleLogoChange"
					/>
				</ShadcnUiFormControl>
			</ShadcnUiFormItem>
		</ShadcnUiFormField>
		<div class="my-3.5 h-px w-full bg-border" />
		<ShadcnUiFormField v-slot="{ componentField }" name="name">
			<ShadcnUiFormItem
				class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
			>
				<div class="flex flex-col gap-0.5">
					<ShadcnUiFormLabel disable-destructive-effect class="text-2base">
						{{ $t("settings.workspace.name-label") }}
					</ShadcnUiFormLabel>
					<div class="text-xs text-muted-foreground">
						{{ $t("settings.workspace.name-description") }}
					</div>
				</div>
				<div class="w-full shrink-0 sm:w-44 md:w-52">
					<ShadcnUiFormControl>
						<ShadcnUiInput
							type="text"
							:placeholder="$t('settings.workspace.name-placeholder')"
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
					{{ $t("settings.workspace.url-label") }}
				</div>
				<div class="text-xs text-muted-foreground">
					{{ $t("settings.workspace.url-description") }}
				</div>
			</div>
			<div
				class="flex w-full items-center justify-between gap-1.5 sm:max-w-44 sm:min-w-44 sm:justify-end md:max-w-52 md:min-w-0 lg:max-w-70"
			>
				<div class="mb-1 flex min-w-0 items-center">
					<div class="flex-1 text-2sm text-muted-foreground">
						{{ $t("settings.workspace.url-prefix") }}
					</div>
					<div class="truncate text-2sm">
						{{ fetchOrganization.state.value.data?.data?.slug }}
					</div>
				</div>
				<ShadcnUiButton
					type="button"
					class="size-6 shrink-0 bg-transparent p-0"
					variant="outline"
					@click="emit('url-change')"
				>
					<Icon name="lucide:pencil" size="0.75rem" />
					<span class="sr-only">
						{{ $t("settings.workspace.url-change-button-screen-reader-hint") }}
					</span>
				</ShadcnUiButton>
			</div>
		</div>
		<div class="my-3.5 h-px w-full bg-border" />
		<div class="flex w-full flex-col gap-0.75">
			<div class="flex items-center justify-between">
				<i18n-t
					scope="global"
					keypath="settings.workspace.members-label"
					tag="div"
					class="text-2base"
				>
					<template #current>{{ members.length }}</template>
					<template #max>{{ ORGANIZATION_MAX_MEMBERS }}</template>
				</i18n-t>

				<ShadcnUiButton
					type="button"
					variant="ghost-plain"
					:class="cn('gap-1.5 p-0 text-sm')"
					@click="emit('invitation')"
				>
					<Icon name="lucide:user-round-plus" />
					{{ $t("settings.workspace.invitation-button") }}
				</ShadcnUiButton>
			</div>
			<table class="w-full table-fixed">
				<colgroup>
					<col class="w-auto md:w-25 lg:w-30" />
					<col class="hidden md:table-column md:w-9 lg:w-14" />
					<col class="w-5 lg:w-7" />
				</colgroup>
				<tbody class="divide-y divide-border/70 dark:divide-border/50">
					<tr v-for="member in members" :key="member.userId">
						<td class="py-2">
							<div class="flex min-w-0 items-center gap-2">
								<ShadcnUiAvatar
									class="mt-0.5 size-7 shrink-0 border"
									@click="handleAvatarClick"
								>
									<ShadcnUiAvatarImage
										v-if="member.user.image"
										:src="member.user.image"
										:alt="$t('settings.profile.image-alt')"
									/>
									<ShadcnUiAvatarFallback>
										{{ extractInitials(member.user.name || "", 2) }}
									</ShadcnUiAvatarFallback>
								</ShadcnUiAvatar>
								<div class="flex min-w-0 flex-col">
									<div class="truncate text-sm">
										{{ member.user.name }}
									</div>
									<div class="truncate text-2sm text-muted-foreground">
										{{ member.user.email }}
									</div>
								</div>
							</div>
						</td>
						<td class="hidden md:table-cell">
							<div class="flex min-w-0 items-center gap-2">
								<div class="flex min-w-0 flex-col">
									<div class="text-2sm text-muted-foreground">
										{{
											member.invitationPending
												? $t("settings.workspace.invited-label")
												: $t("settings.workspace.joined-label")
										}}
									</div>
									<div
										v-if="!member.invitationPending && member.createdAt"
										class="text-2sm text-muted-foreground"
									>
										{{ $d(member.createdAt, "short") }}
									</div>
								</div>
							</div>
						</td>
						<td>
							<div
								v-if="
									member.userId !==
									fetchAuthSession.state.value.data?.data?.user.id
								"
								class="flex h-full items-center justify-end whitespace-nowrap"
							>
								<ShadcnUiDropdownMenu>
									<ShadcnUiDropdownMenuTrigger as-child>
										<ShadcnUiButton
											type="button"
											variant="ghost"
											size="icon"
											:class="cn('size-5')"
										>
											<Icon name="lucide:ellipsis" />
											<span class="sr-only">
												{{
													$t(
														"settings.workspace.members-option-button-screen-reader-hint",
													)
												}}
											</span>
										</ShadcnUiButton>
									</ShadcnUiDropdownMenuTrigger>
									<ShadcnUiDropdownMenuContent
										side="bottom"
										align="end"
										loop
										inside-modal
									>
										<ShadcnUiDropdownMenuItem
											@click="handleMemberDelete(member)"
										>
											<Icon name="lucide:user-round-x" />
											<span>
												{{
													$t("settings.workspace.member-options.delete.title")
												}}
											</span>
										</ShadcnUiDropdownMenuItem>
									</ShadcnUiDropdownMenuContent>
								</ShadcnUiDropdownMenu>
							</div>
						</td>
					</tr>
				</tbody>
			</table>
		</div>
	</form>
</template>
