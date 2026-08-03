<script lang="ts" setup>
import { extractInitials } from "~/utils/object"
import {
	processDocumentTree,
	type PlainActionFn,
	type SidebarItem,
	type SidebarItemCreate,
	type SidebarItemDelete,
	type SidebarItemDuplicate,
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
const { fetchOrganization, safeSignOut } = useAuthSession()
const { fetchGitHubConnectionStatus, fetchGitHubInstallURL } = useGitHubAPI()
const { fetchSlackConnectionStatus, fetchSlackInstallURL } = useSlackAPI()
const { useFetchNotificationCount } = useNotificationAPI()
const fetchNotificationCount = useFetchNotificationCount({ read: false })
const editorStore = useEditorStore()
const { openExternalLink } = useExternalLinks()
const isSearchModalOpen = ref(false)
const wsState = useWebSocketStateStore()
let unsubWsTreeChange: (() => void) | null | undefined = null
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
	items: SidebarItem[]
}

const sections = computed<Section[]>(() => {
	const rootItems: Section[] = [
		{
			// no group heading
			items: [],
		},
		{
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
				fetchDocumentTree.data.value || [],
				editorStore.activeDocumentId,
				t("sidebar.item-placeholder-name"),
				fetchOrganization.data?.value?.data?.name || "",
			),
			isEmptyAfterLoad:
				!fetchDocumentTree.data.value?.length &&
				!fetchDocumentTree.isPending.value,
		},
	]
	const topSectionItems: SidebarItem[] = [
		{
			id: "search-button",
			name: t("sidebar.sections.top.search-button"),
			icon: "lucide:search",
			onClick: () => {
				isSearchModalOpen.value = true
			},
			partOfDocumentTree: false,
			active: false,
			draggable: false,
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
			partOfDocumentTree: false,
			active: props.notificationSidebarOpen,
			draggable: false,
			shortcutTooltip: SHORTCUT_ACTIONS.toggleInbox,
			children: null,
		},
	]

	rootItems[0]!.items = topSectionItems
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
			partOfDocumentTree: false,
			active: false,
			draggable: false,
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (!fetchGitHubConnectionStatus.data.value?.connected) {
		nextStepsItems.push({
			id: "connect-github",
			name: t("sidebar.sections.next-steps.items.connect-github"),
			icon: "simple-icons:github",
			onClick: async () => {
				await installGitHub()
			},
			partOfDocumentTree: false,
			active: false,
			draggable: false,
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (!fetchSlackConnectionStatus.data.value?.connected) {
		nextStepsItems.push({
			id: "connect-slack",
			name: t("sidebar.sections.next-steps.items.connect-slack"),
			icon: "simple-icons:slack",
			onClick: async () => {
				await installSlack()
			},
			partOfDocumentTree: false,
			active: false,
			draggable: false,
			prefetchUrlOnInteraction: false,
			localOptimisticInsert: false,
			children: null,
		})
	}

	if (nextStepsItems.length) {
		rootItems.push({
			heading: t("sidebar.sections.next-steps.heading"),
			items: nextStepsItems,
		})
	}

	return rootItems
})

onBeforeMount(() => {
	fetchDocumentTree.refresh()
	fetchOrganization.refresh()
	fetchNotificationCount.refresh()
})
onMounted(() => {
	unsubWsTreeChange = wsState.state?.subscribe(
		WS_DOCUMENT_TREE_CHANGE_TOPIC,
		() => {
			fetchDocumentTree.refetch()
		},
	)
})
onUnmounted(() => {
	unsubWsTreeChange?.()
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
	const res = await safeSignOut()
	if (res.error) {
		showToastMessage("error", t("sidebar.errors.signout-failed"))
		return
	}

	navigateTo({ name: "login" })
}

function handleCreate(data: SidebarItemCreate) {
	emit("create-document", data.parentId)
}

function handleDuplicate(data: SidebarItemDuplicate) {
	emit("duplicate-document", data.id)
}

async function handleLocationUpdate(data: SidebarItemLocationUpdate) {
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

async function handleDelete(data: SidebarItemDelete) {
	emit("delete-document", data.id)
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
					<ShadcnUiSidebarGroup
						v-for="(section, index) in sections"
						:key="index"
					>
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
						<ShadcnUiSidebarGroupContent>
							<ShadcnUiSidebarMenu>
								<SidebarNestedGroup
									v-model="section.items"
									:item-id="null"
									@create="handleCreate"
									@duplicate="handleDuplicate"
									@update-location="handleLocationUpdate"
									@delete="handleDelete"
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
