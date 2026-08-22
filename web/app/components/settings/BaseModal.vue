<script lang="ts" setup>
import AppsSection from "./AppsSection.vue"
import McpSection from "./McpSection.vue"
import ProfileSection from "./ProfileSection.vue"
import type { OrganizationMember } from "./workspace"
import WorkspaceSection from "./WorkspaceSection.vue"
import DataSourceSection from "./DataSourceSection.vue"

const ORG_SECTION_ID = "settings-org"
const APP_SECTION_ID = "settings-apps"
const DATA_SOURCE_SECTION_ID = "settings-data-sources"

const open = defineModel<
	boolean | "org-members" | "github" | "metric-data-sources"
>({
	default: false,
})
const emit = defineEmits<{
	(e: "refresh-organization-slug"): void
}>()

const { t } = useI18n({ useScope: "global" })
const openAction = ref<
	| null
	| "email-change"
	| "password-change"
	| "account-deletion"
	| "url-change"
	| "workspace-invitation"
	| "workspace-member-removal"
	| "data-source-creation"
	| "data-source-update"
	| "data-source-removal"
>(null)
const actionOpts = ref<OrganizationMember | DataSourceType | DataSource | null>(
	null,
)
const sections = [
	{
		title: t("settings.profile.title"),
		// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		component: ProfileSection,
	},
	{
		title: t("settings.workspace.title"),
		// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		component: WorkspaceSection,
		id: ORG_SECTION_ID,
	},
	{
		title: t("settings.apps.title"),
		// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		component: AppsSection,
		id: APP_SECTION_ID, // for scrolling into view when opened
	},
	{
		title: t("settings.mcp.title"),
		// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		component: McpSection,
	},
	{
		title: t("settings.data-sources.title"),
		// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		component: DataSourceSection,
		id: DATA_SOURCE_SECTION_ID,
	},
]

whenever(open, async (val) => {
	if (val === "org-members") {
		await nextTick()
		document.getElementById(ORG_SECTION_ID)?.scrollIntoView({
			behavior: "instant",
			block: "start",
		})
	} else if (val === "github") {
		await nextTick()
		document.getElementById(APP_SECTION_ID)?.scrollIntoView({
			behavior: "instant",
			block: "start",
		})
	} else if (val === "metric-data-sources") {
		await nextTick()
		document.getElementById(DATA_SOURCE_SECTION_ID)?.scrollIntoView({
			behavior: "instant",
			block: "start",
		})
	}
})
</script>
<template>
	<ShadcnUiDialog :open="!!open" @update:open="(val) => (open = val)">
		<ShadcnUiDialogContent
			class="max-h-[90dvh] w-[90dvw] overflow-y-auto p-0 text-foreground outline-none md:max-h-[80dvh] lg:w-240"
			@open-auto-focus.prevent
			@interact-outside="
				(event) => {
					const target = event.target as HTMLElement
					if (target?.closest('[data-sonner-toaster]')) {
						return event.preventDefault()
					}
				}
			"
		>
			<SettingsActionModal
				v-model="openAction"
				:opts="actionOpts"
				@refresh-organization-slug="emit('refresh-organization-slug')"
			/>
			<div class="flex flex-col gap-8 p-6">
				<div class="flex min-h-0 flex-col gap-8">
					<ShadcnUiDialogHeader>
						<ShadcnUiDialogTitle class="text-2xl">
							{{ $t("settings.title") }}
						</ShadcnUiDialogTitle>
						<ShadcnUiDialogDescription class="sr-only">
							{{ $t("settings.description") }}
						</ShadcnUiDialogDescription>
						<ShadcnUiButton
							variant="ghost-plain"
							class="absolute top-1/2 right-0 -translate-y-1/2 p-0"
							@click="open = false"
						>
							<Icon name="lucide:x" size="1.3rem" />
							<span class="sr-only">
								{{ $t("general.modal-close-screen-reader-hint") }}
							</span>
						</ShadcnUiButton>
					</ShadcnUiDialogHeader>
					<div
						v-for="section in sections"
						:key="section.title"
						class="flex flex-col gap-4"
					>
						<h3 class="text-2base">{{ section.title }}</h3>
						<div class="flex flex-col rounded-md border bg-muted/50 px-4 py-3">
							<component
								:is="section.component"
								:id="section.id"
								@email-change="openAction = 'email-change'"
								@password-change="openAction = 'password-change'"
								@account-deletion="openAction = 'account-deletion'"
								@url-change="openAction = 'url-change'"
								@invitation="openAction = 'workspace-invitation'"
								@member-removal="
									(member: OrganizationMember) => {
										openAction = 'workspace-member-removal'
										actionOpts = member
									}
								"
								@data-source-creation="
									(type: DataSourceType) => {
										openAction = 'data-source-creation'
										actionOpts = type
									}
								"
								@data-source-update="
									(ds: DataSource) => {
										openAction = 'data-source-update'
										actionOpts = ds
									}
								"
								@data-source-removal="
									(ds: DataSource) => {
										openAction = 'data-source-removal'
										actionOpts = ds
									}
								"
							/>
						</div>
					</div>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
