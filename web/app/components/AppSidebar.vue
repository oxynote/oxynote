<script lang="ts" setup>
import { extractInitials } from "~/utils/object"
import {
	processDocumentTree,
	processTagTree,
	type PlainActionFn,
	type SidebarItem,
	type SidebarItemAction,
	type SidebarItemCreate,
	type SidebarItemLocationUpdate,
} from "./sidebar"
import SidebarNestedGroup from "./SidebarNestedGroup.vue"
import { showToastMessage } from "./toast"
import { useSidebar } from "./shadcn/ui/sidebar"

const props = defineProps<{
	allInitialSectionsLoaded: boolean
	notificationSidebarOpen: boolean
}>()
const emit = defineEmits<{
	(e: "initial-load-complete" | "toggle-notifications"): void
	(e: "open-settings", target: "org-members" | null): void
	(e: "delete-document" | "duplicate-document", id: string): void
	(e: "create-document", parentId: string | null): void
}>()

const { t } = useI18n({ useScope: "global" })
const { fetchDocumentTree, updateDocumentTree } = useDocumentAPI()
const {
	fetchTagTree,
	updateTagTree,
	updateTagVisibility,
	deleteTag,
	unassignBranchTag,
} = useTagAPI()
const { fetchOrganization, safeSignOut } = useAuthSession()
const { fetchGitHubConnectionStatus, gitHubConfigured, fetchGitHubInstallURL } =
	useGitHubAPI()
const { fetchSlackConnectionStatus, slackConfigured, fetchSlackInstallURL } =
	useSlackAPI()
const { useFetchNotificationCount } = useNotificationAPI()
const fetchNotificationCount = useFetchNotificationCount({ read: false })
const editorStore = useEditorStore()
const { openExternalLink } = useExternalLinks()
const isSearchModalOpen = ref(false)
const wsState = useWebSocketStateStore()
let unsubWsTreeChange: (() => void) | null | undefined = null
let unsubWsTagTreeChange: (() => void) | null | undefined = null
let initialLoadComplete = false

const { toggleSidebar } = useSidebar()
useShortcut(SHORTCUT_ACTIONS.toggleSidebar.keyboardKey, () => {
	toggleSidebar()
})
useShortcut(SHORTCUT_ACTIONS.searchForDocuments.keyboardKey, () => {
	isSearchModalOpen.value = !isSearchModalOpen.value
})
useShortcut(SHORTCUT_ACTIONS.toggleInbox.keyboardKey, () => {
	emit("toggle-notifications")
})
useShortcut(SHORTCUT_ACTIONS.createNewDocument.keyboardKey, () => {
	handleCreate({ parentId: null })
})
useShortcut(SHORTCUT_ACTIONS.toggleSettings.keyboardKey, () => {
	toggleSettings()
})

interface Section {
	id: string
	heading?: string
	headingAction?: {
		title: string
		icon: string
		shortcutTooltip?: {
			keyboardKey: { macOS: string; other: string }
			i18nKey: string | null
		}
		fn: PlainActionFn
	}
	isEmptyAfterLoad?: boolean
	// shown in place of the rows while the section has none
	emptyMessage?: string
	items: SidebarItem[]
	onUpdateLocation?: (data: SidebarItemLocationUpdate) => void | Promise<void>
}

const sections = computed<Section[]>(() => {
	const topSection: Section = {
		id: "top",
		// no group heading
		items: [],
	}
	const rootItems: Section[] = [
		topSection,
		{
			id: "workspace",
			heading: t("sidebar.sections.main-workspace.heading"),
			headingAction: {
				title: t("sidebar.sections.main-workspace.heading-action-title"),
				icon: "lucide:plus",
				shortcutTooltip: SHORTCUT_ACTIONS.createNewDocument,
				fn: () => {
					handleCreate({ parentId: null })
				},
			},
			items: processDocumentTree(
				fetchDocumentTree.data.value ?? [],
				editorStore.activeDocumentId,
				t("sidebar.item-placeholder-name"),
				fetchOrganization.data.value?.data?.name || "",
				documentActions,
			),
			isEmptyAfterLoad:
				!fetchDocumentTree.data.value?.length &&
				!fetchDocumentTree.isPending.value,
			onUpdateLocation: handleDocumentLocationUpdate,
		},
	]
	const tagItems = processTagTree(
		fetchTagTree.data.value ?? [],
		editorStore.activeDocumentId,
		fetchOrganization.data.value?.data?.name || "",
		tagActions,
		taggedDocumentActions,
	)

	// the section stays as long as the organization has any tag at all: it
	// carries the control that brings a hidden one back, and hiding the
	// last visible tag would otherwise take that control with it.
	if (fetchTagTree.data.value?.length) {
		rootItems.push({
			id: "tags",
			heading: t("sidebar.sections.tags.heading"),
			emptyMessage: t("sidebar.sections.tags.all-hidden"),
			items: tagItems,
			onUpdateLocation: handleTagLocationUpdate,
		})
	}

	const topSectionItems: SidebarItem[] = [
		{
			id: "search-button",
			name: t("sidebar.sections.top.search-button"),
			icon: "lucide:search",
			onClick: () => {
				isSearchModalOpen.value = true
			},
			acceptsChildren: false,
			active: false,
			draggable: false,
			actions: [],
			shortcutTooltip: SHORTCUT_ACTIONS.searchForDocuments,
			children: null,
		},
		{
			id: "inbox",
			name: t("sidebar.sections.top.inbox"),
			count: fetchNotificationCount.data.value?.count || 0,
			icon: "lucide:inbox",
			onClick: () => {
				emit("toggle-notifications")
			},
			acceptsChildren: false,
			active: props.notificationSidebarOpen,
			draggable: false,
			actions: [],
			shortcutTooltip: SHORTCUT_ACTIONS.toggleInbox,
			children: null,
		},
	]

	topSection.items = topSectionItems
	const nextStepsItems: SidebarItem[] = []

	if (
		!fetchOrganization.state.value.data?.data?.members ||
		fetchOrganization.state.value.data.data.members.length <= 1
	) {
		nextStepsItems.push({
			id: "invite-team-members",
			name: t("sidebar.sections.next-steps.items.invite-team-members"),
			icon: "mingcute:user-add-fill",
			onClick: () => {
				emit("open-settings", "org-members")
			},
			acceptsChildren: false,
			active: false,
			draggable: false,
			actions: [],
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (
		gitHubConfigured.value &&
		!fetchGitHubConnectionStatus.data.value?.connected
	) {
		nextStepsItems.push({
			id: "connect-github",
			name: t("sidebar.sections.next-steps.items.connect-github"),
			icon: "simple-icons:github",
			onClick: async () => {
				await installGitHub()
			},
			acceptsChildren: false,
			active: false,
			draggable: false,
			actions: [],
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (
		slackConfigured.value &&
		!fetchSlackConnectionStatus.data.value?.connected
	) {
		nextStepsItems.push({
			id: "connect-slack",
			name: t("sidebar.sections.next-steps.items.connect-slack"),
			icon: "simple-icons:slack",
			onClick: async () => {
				await installSlack()
			},
			acceptsChildren: false,
			active: false,
			draggable: false,
			actions: [],
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (nextStepsItems.length) {
		rootItems.push({
			id: "next-steps",
			heading: t("sidebar.sections.next-steps.heading"),
			items: nextStepsItems,
		})
	}

	return rootItems
})

onBeforeMount(() => {
	void fetchDocumentTree.refresh()
	void fetchTagTree.refresh()
	void fetchOrganization.refresh()
	void fetchNotificationCount.refresh()
})
onMounted(() => {
	unsubWsTreeChange = wsState.state?.subscribe(
		WS_DOCUMENT_TREE_CHANGE_TOPIC,
		() => {
			void fetchDocumentTree.refetch()
		},
	)
	unsubWsTagTreeChange = wsState.state?.subscribe(
		WS_TAG_TREE_CHANGE_TOPIC,
		() => {
			void fetchTagTree.refetch()
		},
	)
})
onUnmounted(() => {
	unsubWsTreeChange?.()
	unsubWsTagTreeChange?.()
})

if (import.meta.client) {
	watchImmediate(
		() =>
			sections.value.every(
				(section) => section.items.length > 0 || section.isEmptyAfterLoad,
			),
		(v) => {
			if (v && !initialLoadComplete) {
				emit("initial-load-complete")
				initialLoadComplete = true
			}
		},
	)
}

function toggleSettings() {
	emit("open-settings", null)
}

async function handleLogout() {
	const res = (await safeSignOut()) as AuthResponse
	if (res.error) {
		showToastMessage("error", t("sidebar.errors.signout-failed"))
		return
	}

	void navigateTo({ name: "login" })
}

function handleCreate(data: SidebarItemCreate) {
	emit("create-document", data.parentId)
}

function documentActions(documentId: string): SidebarItemAction[] {
	return [
		{
			id: "duplicate-page",
			name: t("sidebar.item-dropdown-menu-buttons.duplicate-page"),
			icon: "mingcute:copy-2-line",
			fn: () => {
				emit("duplicate-document", documentId)
			},
		},
		{
			id: "add-sub-page",
			name: t("sidebar.item-dropdown-menu-buttons.add-sub-page"),
			icon: "lucide:file-plus",
			fn: () => {
				emit("create-document", documentId)
			},
		},
		{
			id: "delete-page",
			name: t("sidebar.item-dropdown-menu-buttons.delete-page"),
			icon: "lucide:trash-2",
			fn: () => {
				emit("delete-document", documentId)
			},
		},
	]
}

function handleTagVisibilityToggle(tag: TagTreeElement) {
	void handleTagVisibilityUpdate(tag.id, { hidden: !tag.hidden })
}

function tagActions(tag: TagTreeElement): SidebarItemAction[] {
	return [
		{
			id: "hide-tag",
			name: t("sidebar.item-dropdown-menu-buttons.hide-tag"),
			icon: "lucide:eye-off",
			fn: async () => {
				await handleTagVisibilityUpdate(tag.id, { hidden: true })
			},
		},
		{
			id: "delete-tag",
			name: t("sidebar.item-dropdown-menu-buttons.delete-tag"),
			icon: "lucide:trash-2",
			fn: async () => {
				await handleTagDelete(tag.id)
			},
		},
	]
}

function taggedDocumentActions(
	documentId: string,
	defaultBranchId: string,
	tagId: string,
): SidebarItemAction[] {
	return [
		{
			id: "remove-tag",
			name: t("sidebar.item-dropdown-menu-buttons.remove-tag"),
			icon: "lucide:x",
			fn: async () => {
				await handleDocumentTagRemoval(documentId, defaultBranchId, tagId)
			},
		},
	]
}

async function handleTagVisibilityUpdate(id: string, req: { hidden: boolean }) {
	try {
		await updateTagVisibility.mutateAsync({ id: id, req: req })
	} catch {
		showToastMessage("error", t("sidebar.errors.update-tag-failed"))
	}
}

async function handleTagDelete(id: string) {
	try {
		await deleteTag.mutateAsync(id)
	} catch {
		showToastMessage("error", t("sidebar.errors.delete-tag-failed"))
	}
}

// the tree lists a document under a tag by its default branch, so that is
// the branch the row's action detaches
async function handleDocumentTagRemoval(
	documentId: string,
	defaultBranchId: string,
	tagId: string,
) {
	try {
		await unassignBranchTag.mutateAsync({
			documentId: documentId,
			branchId: defaultBranchId,
			tagId: tagId,
		})
	} catch {
		showToastMessage("error", t("sidebar.errors.remove-document-tag-failed"))
	}
}

async function handleDocumentLocationUpdate(data: SidebarItemLocationUpdate) {
	try {
		await updateDocumentTree.mutateAsync({
			id: data.id,
			parentId: data.parentId,
			insertBeforeId: data.insertBeforeId,
		})
	} catch {
		showToastMessage("error", t("sidebar.errors.update-tree-failed"))
	}
}

async function handleTagLocationUpdate(data: SidebarItemLocationUpdate) {
	try {
		await updateTagTree.mutateAsync({
			id: data.id,
			insertBeforeId: data.insertBeforeId,
		})
	} catch {
		showToastMessage("error", t("sidebar.errors.update-tags-failed"))
	}
}

async function installGitHub() {
	try {
		const result = await fetchGitHubInstallURL()
		openExternalLink(result.url)
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.github.errors.install.title"),
			t("settings.apps.github.errors.install.description"),
		)
	}
}

async function installSlack() {
	try {
		const result = await fetchSlackInstallURL()
		openExternalLink(result.url)
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.slack.errors.install.title"),
			t("settings.apps.slack.errors.install.description"),
		)
	}
}
</script>
<template>
	<ShadcnUiSidebar :disable-resize-handle="!props.allInitialSectionsLoaded">
		<Transition v-bind="defaultTransitionProps">
			<div
				v-if="props.allInitialSectionsLoaded"
				class="h-full w-full overflow-y-auto"
			>
				<ShadcnUiSidebarHeader class="pb-0">
					<ShadcnUiSidebarMenu>
						<AppSidebarHeader
							:workspace-name="fetchOrganization.data?.value?.data?.name"
							:avatar="{
								src: fetchOrganization.data?.value?.data?.logo || '',
								alt: $t('sidebar.logo-alt'),
								initials: extractInitials(
									fetchOrganization.data?.value?.data?.name || '',
									2,
								),
							}"
							@create-new-item="handleCreate({ parentId: null })"
							@log-out="handleLogout"
							@open-settings="toggleSettings"
						/>
					</ShadcnUiSidebarMenu>
				</ShadcnUiSidebarHeader>
				<ShadcnUiSidebarContent>
					<ShadcnUiSidebarGroup v-for="section in sections" :key="section.id">
						<ShadcnUiSidebarGroupLabel v-if="section.heading">
							{{ section.heading }}
						</ShadcnUiSidebarGroupLabel>
						<ShortcutTooltip
							v-if="section.heading && section.headingAction"
							side="bottom"
							:shortcut="section.headingAction.shortcutTooltip"
						>
							<ShadcnUiSidebarGroupAction @click="section.headingAction.fn">
								<Icon :name="section.headingAction.icon" />
								<span class="sr-only">{{ section.headingAction.title }}</span>
							</ShadcnUiSidebarGroupAction>
						</ShortcutTooltip>
						<SidebarTagVisibility
							v-if="section.id === 'tags'"
							:tags="fetchTagTree.data.value ?? []"
							@toggle="handleTagVisibilityToggle"
						/>
						<ShadcnUiSidebarGroupContent>
							<div
								v-if="section.emptyMessage && !section.items.length"
								class="px-2 py-1.25 text-2sm text-sidebar-foreground/60"
							>
								{{ section.emptyMessage }}
							</div>
							<ShadcnUiSidebarMenu v-else>
								<SidebarNestedGroup
									v-model="section.items"
									:item-id="null"
									@create="handleCreate"
									@update-location="section.onUpdateLocation"
								/>
							</ShadcnUiSidebarMenu>
						</ShadcnUiSidebarGroupContent>
					</ShadcnUiSidebarGroup>
				</ShadcnUiSidebarContent>
			</div>
		</Transition>
		<SearchModal v-model="isSearchModalOpen" />
	</ShadcnUiSidebar>
</template>
