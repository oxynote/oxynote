<script lang="ts" setup>
import EmailChangeAction from "./EmailChangeAction.vue"
import WorkspaceInvitationAction from "./WorkspaceInvitationAction.vue"
import WorkspaceMemberRemovalAction from "./WorkspaceMemberRemovalAction.vue"
import DataSourceUpsertAction from "./DataSourceUpsertAction.vue"
import DataSourceRemovalAction from "./DataSourceRemovalAction.vue"
import WorkspaceURLChangeAction from "./WorkspaceURLChangeAction.vue"
import type { OrganizationMember } from "./workspace"
import AccountDeletionAction from "./AccountDeletionAction.vue"

const props = defineProps<{
	opts?: OrganizationMember | DataSourceType | DataSource | null
}>()
const openType = defineModel<
	| "email-change"
	| "account-deletion"
	| "url-change"
	| "workspace-invitation"
	| "workspace-member-removal"
	| "data-source-creation"
	| "data-source-update"
	| "data-source-removal"
	| null
>({
	default: null,
})
const emit = defineEmits<{
	(e: "refresh-organization-slug"): void
}>()

const open = computed({
	get: () => openType.value !== null,
	set: (val: boolean) => {
		if (!val) {
			openType.value = null
		}
	},
})
const { t } = useI18n({ useScope: "global" })
const { fetchOrganization } = useAuthSession()
const isMaxMembersReached = computed(() => {
	const org = fetchOrganization.state.value.data?.data
	if (!org) {
		return false
	}

	return (
		org.members.length +
			org.invitations.filter((inv) => inv.status === "pending").length >=
		ORGANIZATION_MAX_MEMBERS
	)
})

const action = computed(() => {
	switch (openType.value) {
		case "email-change":
			return {
				title: t("settings.action-modals.email-change.title"),
				component: EmailChangeAction,
			}
		case "account-deletion":
			return {
				title: t("settings.action-modals.account-deletion.title"),
				component: AccountDeletionAction,
			}
		case "url-change":
			return {
				title: t("settings.action-modals.workspace-url-change.title"),
				component: WorkspaceURLChangeAction,
			}
		case "workspace-invitation":
			return {
				title: isMaxMembersReached.value
					? t(
							"settings.action-modals.workspace-invitation.title-max-members-reached",
						)
					: t("settings.action-modals.workspace-invitation.title"),
				component: WorkspaceInvitationAction,
			}
		case "workspace-member-removal":
			if (!props.opts || !(props.opts as OrganizationMember).user) {
				return null
			}

			return {
				title: t("settings.action-modals.workspace-member-removal.title"),
				component: WorkspaceMemberRemovalAction,
				opts: {
					member: props.opts as OrganizationMember,
				},
			}
		case "data-source-creation":
			if (!props.opts) {
				return null
			}

			return {
				title: t(
					`settings.action-modals.data-source-upsert.title.creation.${props.opts}`,
				),
				component: DataSourceUpsertAction,
				opts: {
					creationType: props.opts as DataSourceType,
				},
			}
		case "data-source-update":
			if (!props.opts) {
				return null
			}

			return {
				title: t(
					`settings.action-modals.data-source-upsert.title.update.${(props.opts as DataSource).type}`,
				),
				component: DataSourceUpsertAction,
				opts: {
					updateTarget: props.opts as DataSource,
				},
			}
		case "data-source-removal":
			if (!props.opts) {
				return null
			}

			return {
				title: t(
					`settings.action-modals.data-source-removal.title.${(props.opts as DataSource).type}`,
				),
				component: DataSourceRemovalAction,
				opts: {
					data: props.opts as DataSource,
				},
			}
		default:
			return null
	}
})
</script>
<template>
	<ShadcnUiDialog v-model:open="open">
		<ShadcnUiDialogContent
			class="max-h-[90dvh] w-[85dvw] overflow-y-auto p-0 text-foreground sm:w-110 md:max-h-[80dvh]"
			@interact-outside="
				(event) => {
					const target = event.target as HTMLElement
					if (target?.closest('[data-sonner-toaster]')) {
						return event.preventDefault()
					}
				}
			"
		>
			<div class="flex flex-col gap-8 p-6">
				<div class="flex min-h-0 flex-col gap-3">
					<template v-if="openType && action">
						<ShadcnUiDialogHeader>
							<ShadcnUiDialogTitle class="text-base">
								{{ action.title }}
							</ShadcnUiDialogTitle>
							<ShadcnUiButton
								variant="ghost-plain"
								class="absolute top-1/2 right-0 shrink-0 -translate-y-1/2 p-0"
								@click="open = false"
							>
								<Icon name="lucide:x" size="1rem" />
								<span class="sr-only">
									{{ $t("general.modal-close-screen-reader-hint") }}
								</span>
							</ShadcnUiButton>
						</ShadcnUiDialogHeader>
						<component
							:is="action.component"
							v-bind="action.opts"
							@close="open = false"
							@refresh-organization-slug="emit('refresh-organization-slug')"
						/>
					</template>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
