<script lang="ts" setup>
import {
	documentTreeBreadcrumbs,
	extractDocumentTreeElement,
} from "~/components/sidebar"
import {
	redirectToDocId,
	redirectToFirst,
	redirectToLogin,
	redirectToOrgRoot,
} from "~/plugins/03.api-fetch"
import { showToastMessage } from "~/components/toast"
import type { Editor } from "@tiptap/core"

const DOC_CREATION_MIN_SPINNER_TIME_MS = 500

definePageMeta({
	name: "document-editor",
	key(route) {
		// prevent page reload on slug (org name or doc name) change
		return route.name as string
	},
	middleware: async (to) => {
		const nuxtApp = useNuxtApp()
		const { fetchDocumentTree } = useDocumentAPI()
		const { fetchOrganization } = useAuthSession()
		const orgRealName = (await fetchOrganization.refresh()).data?.data?.slug

		if (!orgRealName) {
			return redirectToLogin(to.fullPath, true, nuxtApp)
		}

		// resolved before the page renders so capability-gated markup is
		// already correct server-side and nothing appears then vanishes on
		// hydration. Instantiated through runWithContext because the vue app
		// context is gone after an await, and the query injects its option
		// defaults
		await nuxtApp.runWithContext(() =>
			useCapabilitiesAPI().fetchCapabilities.refresh(),
		)

		if (!to.params.documentSlug || typeof to.params.documentSlug !== "string") {
			const docTree = await fetchDocumentTree.refresh()
			if (!docTree.data?.length) {
				// if no documents exist, just return and show the "no document" view
				return
			}

			return await redirectToFirst(
				orgRealName,
				fetchOrganization,
				fetchDocumentTree,
				null,
			)
		}

		// the slug can be either name-id or id
		const docSlugInfo = extractDocInfoFromSlug(to.params.documentSlug)
		if (!docSlugInfo) {
			return await redirectToFirst(
				orgRealName,
				fetchOrganization,
				fetchDocumentTree,
				null,
			)
		}

		const orgSlugName = to.params.organizationSlug as string | undefined
		const docTree = await fetchDocumentTree.refresh()
		const docSlugName = docSlugInfo.name
		const docRealName = docNameByIdInDocumentTree(
			docTree.data ?? [],
			docSlugInfo.id,
		)

		// null means the document doesn't exist in the tree
		if (docRealName === null) {
			return await redirectToFirst(
				orgRealName,
				fetchOrganization,
				fetchDocumentTree,
				null,
			)
		}

		if (
			!orgSlugName ||
			!equalNameSlugs(orgSlugName, orgRealName) ||
			(docRealName &&
				(!docSlugName || !equalNameSlugs(docSlugName, docRealName)))
		) {
			return navigateTo(
				{
					path: `/${createNameSlug(orgRealName)}/${createNameSlugWithId(
						docRealName,
						docSlugInfo.id,
					)}`,
					query: to.query,
					hash: to.hash,
				},
				{
					replace: true,
				},
			)
		}
	},
})

useShortcut() // use for side effects
const pageRoute = useRoute()
const pageRouter = useRouter()
const {
	fetchDocumentTree,
	updateDocumentTreeElementCache,
	createDocument,
	deleteDocument,
	duplicateDocument,
	useFetchDocumentBranchesByDocId,
} = useDocumentAPI()
const { deleteTag, updateTagTreeDocumentCache } = useTagAPI()
const { fetchOrganization } = useAuthSession()
const { t } = useI18n({ useScope: "global" })
const { isAssistantEnabled } = useCapabilitiesAPI()
const wsState = useWebSocketStateStore()
let unsubWsDocMetadataChange: (() => void) | null | undefined = null

// we use this to track whether all sections have loaded their initial data
// (only applies to data that is loaded immediately after page load)
const loadedSections = ref({
	sidebar: false,
	documentMetadata: false,
	documentEditor: false,
})
const allSectionsLoaded = computed(() =>
	Object.values(loadedSections.value).every((v) => v),
)

const createLoading = ref(false)
const settingModalOpen = ref<
	boolean | "org-members" | "github" | "metric-data-sources"
>(false)
const shortcutsModalOpen = ref(false)
const pendingDocDeletion = ref<{
	deleteDocument: () => Promise<void>
	name: string
} | null>(null)
const pendingTagDeletion = ref<{
	deleteTag: () => Promise<void>
	name: string
} | null>(null)
const notificationSidebarOpen = ref(false)
// the url hash that links a block. Every replace of the url takes its hash
// from here, because the route still holds a dropped hash until the
// replace that dropped it lands
const linkedBlockHash = ref(pageRoute.hash)
const contentEditorRef = shallowRef<Editor | null>(null)
const nameEditorRef = shallowRef<Editor | null>(null)
const editorStore = useEditorStore()
const mainArea = useTemplateRef("main")
const { width: mainAreaWidth } = useElementSize(mainArea)
watchImmediate(mainAreaWidth, editorStore.updateMainAreaWidth)
const pageReady = usePageReady()
const activeDocMetadata = computed(() => {
	const docInfo = extractDocInfoFromSlug(
		pageRoute.params.documentSlug as string,
	)
	if (!docInfo) {
		return null
	}

	const elem = extractDocumentTreeElement(
		fetchDocumentTree.data.value ?? [],
		docInfo.id,
	)
	if (!elem) {
		return null
	}

	return {
		id: elem.id,
		name: elem.documentName,
		icon: elem.icon,
		protected: elem.protected,
	}
})
const fetchBranches = useFetchDocumentBranchesByDocId(
	() => activeDocMetadata.value?.id,
)
const timestamps = ref<DocumentTimestamps>({})
const breadcrumbs = computed(() => {
	// this data should be already synced by this point (in the page meta or
	// other places like the sidebar)
	if (!fetchDocumentTree.data.value || !activeDocMetadata.value) {
		return []
	}

	return documentTreeBreadcrumbs(
		fetchDocumentTree.data.value,
		activeDocMetadata.value.id,
	)
})

useHead({
	title: () =>
		activeDocMetadata.value
			? t("general.document-page-title", {
					suffix: activeDocMetadata.value.name,
				})
			: t("general.default-page-title"),
})

onUnmounted(() => {
	unsubWsDocMetadataChange?.()
})

watch(fetchDocumentTree.state, (newV) => {
	if (!activeDocMetadata.value?.id) {
		return
	}

	// handle the case where the current document got deleted or the user lost
	// access to it, by redirecting to the first available document
	if (!isIdInDocumentTree(newV.data ?? [], activeDocMetadata.value.id)) {
		void redirectToFirst(null, fetchOrganization, fetchDocumentTree, null)
	}
})

watchImmediate(
	() => activeDocMetadata.value?.id,
	(newV) => {
		editorStore.updateActiveDocumentId(newV ?? null)
		pageReady.value = false

		if (import.meta.dev) {
			startLoadProgress()
		}
	},
)

if (import.meta.dev) {
	watchOnce(
		() => loadedSections.value.sidebar,
		() => {
			logSectionReady("sidebar")
		},
	)

	watchOnce(
		() => loadedSections.value.documentMetadata,
		() => {
			logSectionReady("header")
		},
	)
}

// a document url opens the branch its ?branch=<id> names, or the default
// branch when it names none the document has. This runs only when the
// query changes or the branch list loads for the first time. A refetch
// of the list is skipped. The header switches the branch before the url
// is updated, so a refetch in between would read the old branch from
// the url and switch back to it
watchImmediate(
	[() => pageRoute.query.branch, fetchBranches.state],
	([queryBranch, branchState], [oldQueryBranch, oldBranchState]) => {
		const branches = branchState.data

		if (!branches?.length) {
			return
		}

		if (queryBranch === oldQueryBranch && oldBranchState?.data?.length) {
			return
		}

		const defaultBranch = branches.find((b) => b.default)
		const branch =
			branches.find((b) => b.branchId === queryBranch) ?? defaultBranch

		// a query naming the default branch, or none the document has, is
		// rewritten here. The watcher below only runs when the branch changes
		if (!branch || branch.branchId === editorStore.activeBranchId) {
			syncBranchQuery()
			return
		}

		editorStore.updateActiveBranchId(branch.branchId)
		editorStore.updateTargetBranchId(
			branch.default ? null : (defaultBranch?.branchId ?? null),
		)
	},
)

watchImmediate(
	[
		fetchBranches.state,
		() => editorStore.activeBranchId,
		() => editorStore.targetBranchId,
		() => editorStore.mappedDefaultBranchId,
	],
	([branchState]) => {
		if (!activeDocMetadata.value?.id || !branchState.data?.length) {
			return
		}

		const branches = branchState.data

		// NOTE: for now we preload all branches of the document as soon as
		// we have them, to avoid loading states when switching between
		// branches. In the future, we might want to have a more on-demand
		// loading strategy
		editorStore.updatePreloadedBranchIds(branches.map((b) => b.branchId))

		const activeBranch = branches.find(
			(b) => b.branchId === editorStore.activeBranchId,
		)
		const targetBranch = branches.find(
			(b) => b.branchId === editorStore.targetBranchId,
		)
		const mappedDefaultBranch = branches.find(
			(b) => b.branchId === editorStore.mappedDefaultBranchId,
		)
		const defaultBranch = branches.find((b) => b.default)

		if (!activeBranch) {
			editorStore.updateActiveBranchId(defaultBranch?.branchId ?? null)
		}

		if (
			!targetBranch ||
			editorStore.targetBranchId === editorStore.activeBranchId
		) {
			if (editorStore.activeBranchId !== defaultBranch?.branchId) {
				editorStore.updateTargetBranchId(defaultBranch?.branchId ?? null)
			} else {
				editorStore.updateTargetBranchId(null)
			}
		}

		if (!mappedDefaultBranch) {
			editorStore.updateMappedDefaultBranchId(defaultBranch?.branchId ?? null)
		}

		// NOTE: for now, reviewable actions are available only when there are
		// multiple branches and the active branch is not the default branch. In
		// the future, we might want to have more granular control over this
		// (e.g. allow reviewable actions on the default branch, or have a
		// separate setting to enable/disable reviewable actions regardless of
		// the number of branches)
		editorStore.setBranchReviewableActionsActive(
			branches.length > 1 && !activeBranch?.default,
		)
	},
)

// the address bar names the active branch, so a copied url opens it. A
// switch from one branch to another also drops the linked block, which
// belongs to the branch being left
watchImmediate(
	() => editorStore.activeBranchId,
	(_branchId, oldBranchId) => {
		syncBranchQuery(!!oldBranchId)
	},
)

watch(
	() => pageRoute.hash,
	(hash) => {
		linkedBlockHash.value = hash
	},
)

if (import.meta.client) {
	watchImmediate(
		() => activeDocMetadata.value?.id,
		(newId) => {
			if (!loadedSections.value.documentMetadata) {
				loadedSections.value.documentMetadata = true
			}

			unsubWsDocMetadataChange?.()
			if (!newId) {
				return
			}

			unsubWsDocMetadataChange = wsState.state?.subscribe(
				makeWsDocumentMetadataChangeTopic(newId),
				(rawPayload) => {
					const payload = rawPayload as WSDocumentMetadataPayload

					updateDocumentTreeBranchProtected(payload.branchId, payload.protected)

					const branchTimestamp: DocumentTimestamps[string] = {
						created: {
							at: new Date(payload.createdAt),
							user: null,
						},
						updated: {
							at: new Date(payload.updatedAt ?? payload.createdAt),
							user: null,
						},
					}

					if (payload.createdBy) {
						const member = fetchOrganization.data.value?.data?.members.find(
							(m) => m.user.id === payload.createdBy,
						)

						if (member) {
							branchTimestamp.created.user = {
								id: member.user.id,
								name: member.user.name,
							}
						}
					}

					if (payload.lastUpdatedBy) {
						const member = fetchOrganization.data.value?.data?.members.find(
							(m) => m.user.id === payload.lastUpdatedBy,
						)

						if (member) {
							branchTimestamp.updated.user = {
								id: member.user.id,
								name: member.user.name,
							}
						}
					}

					timestamps.value = {
						...timestamps.value,
						[payload.branchId]: branchTimestamp,
					}
				},
			)
		},
	)
}

function toggleNotificationSidebar() {
	notificationSidebarOpen.value = !notificationSidebarOpen.value
}

// the query of this page's url with ?branch= naming the active branch.
// The default branch is left unnamed. Every replace of the url must take
// its query from here. A replace with the route's old query would cancel
// one that syncBranchQuery started in the same tick
function branchQuery() {
	const activeBranch = fetchBranches.state.value.data?.find(
		(b) => b.branchId === editorStore.activeBranchId,
	)
	if (!activeBranch) {
		return pageRoute.query
	}

	return {
		...pageRoute.query,
		branch: activeBranch.default ? undefined : activeBranch.branchId,
	}
}

function syncBranchQuery(dropLinkedBlock = false) {
	const query = branchQuery()
	if (query.branch === pageRoute.query.branch) {
		return
	}

	if (dropLinkedBlock) {
		linkedBlockHash.value = ""
		clearTiptapScrollElementHighlightOverlays()
	}

	void pageRouter.replace({
		path: pageRoute.path,
		query: query,
		hash: linkedBlockHash.value,
	})
}

function applyDocumentNameChange(name: string) {
	if (!activeDocMetadata.value) {
		return
	}

	if (!name) {
		name = t("editor.new-document-name")
	}

	updateDocumentTreeElementCache(activeDocMetadata.value.id, {
		name: name,
		icon: activeDocMetadata.value.icon,
		protected: activeDocMetadata.value.protected,
	})
	updateTagTreeDocumentCache(activeDocMetadata.value.id, {
		name: name,
		icon: activeDocMetadata.value.icon,
		protected: activeDocMetadata.value.protected,
	})

	void pageRouter.replace({
		path: `/${createNameSlug(fetchOrganization.data.value?.data?.slug || "")}/${createNameSlugWithId(
			name,
			activeDocMetadata.value.id,
		)}`,
		query: branchQuery(),
		hash: linkedBlockHash.value,
	})
}

function refreshOrganizationRouteSlug() {
	if (!activeDocMetadata.value) {
		return
	}

	void pageRouter.replace({
		path: `/${createNameSlug(fetchOrganization.data.value?.data?.slug || "")}/${createNameSlugWithId(
			activeDocMetadata.value.name,
			activeDocMetadata.value.id,
		)}`,
		query: branchQuery(),
		hash: linkedBlockHash.value,
	})
}

function applyIconChange(icon: string) {
	if (!activeDocMetadata.value) {
		return
	}

	updateDocumentTreeElementCache(activeDocMetadata.value.id, {
		name: activeDocMetadata.value.name,
		icon: icon,
		protected: activeDocMetadata.value.protected,
	})
	updateTagTreeDocumentCache(activeDocMetadata.value.id, {
		name: activeDocMetadata.value.name,
		icon: icon,
		protected: activeDocMetadata.value.protected,
	})
}

async function handleDocumentCreation(
	parentId: string | null,
	skipOptimistic = false,
) {
	pendingDocDeletion.value = null
	createLoading.value = true

	try {
		await Promise.all([
			createDocument.mutateAsync({
				name: t("editor.new-document-name"),
				icon: "mingcute:document-2-fill",
				parentId: parentId,
				skipLocalOptimisticInsert: skipOptimistic,
			}),
			delay(DOC_CREATION_MIN_SPINNER_TIME_MS), // slight delay to prevent spinner flicker
		])

		if (fetchDocumentTree.state.value.data?.length === 1) {
			await until(fetchDocumentTree.isLoading).toBe(false)
			await redirectToFirst(null, fetchOrganization, fetchDocumentTree, null)
		}
	} catch {
		showToastMessage("error", t("sidebar.errors.create-document-failed"))
		return
	} finally {
		createLoading.value = false
	}
}

// null/undefined deletes the active document.
function handleDocumentDeletion(id: string | null | undefined) {
	id = id ?? activeDocMetadata.value?.id
	if (!id) {
		return
	}

	const name = extractDocumentTreeElement(
		fetchDocumentTree.data.value ?? [],
		id,
	)?.documentName
	if (!name) {
		return
	}

	pendingDocDeletion.value = {
		name: name,
		deleteDocument: async () => {
			if (id === activeDocMetadata.value?.id) {
				await redirectToFirst(null, fetchOrganization, fetchDocumentTree, id)
			}

			try {
				await deleteDocument.mutateAsync(id)

				if (!fetchDocumentTree.data.value?.length) {
					await redirectToOrgRoot(fetchOrganization)
				}

				showToastMessage(
					"success",
					t("editor.document-deletion-modal.deletion-success", { name }),
				)
			} catch {
				showToastMessage("error", t("editor.errors.deletion-failed"))
			}
		},
	}
}

function handleTagDeletion(tag: { id: string; name: string }) {
	pendingTagDeletion.value = {
		name: tag.name,
		deleteTag: async () => {
			try {
				await deleteTag.mutateAsync(tag.id)
			} catch {
				showToastMessage("error", t("sidebar.errors.delete-tag-failed"))
			}
		},
	}
}

async function handleDocumentDuplication(id?: string) {
	if (!id) {
		id = activeDocMetadata.value?.id
	}

	if (!id) {
		return
	}

	createLoading.value = true

	try {
		const doc = await duplicateDocument.mutateAsync(id)
		if (doc) {
			await redirectToDocId(doc.id, fetchOrganization)
		}
	} catch {
		showToastMessage("error", t("editor.errors.duplication-failed"))
	} finally {
		createLoading.value = false
	}
}

function updateDocumentTreeBranchProtected(
	branchId: string,
	protectedMode: boolean,
) {
	if (!activeDocMetadata.value) {
		return
	}

	// document tree elements contain the protected status of the default
	// branch
	if (editorStore.mappedDefaultBranchId !== branchId) {
		return
	}

	if (activeDocMetadata.value.protected === protectedMode) {
		return
	}

	updateDocumentTreeElementCache(activeDocMetadata.value.id, {
		name: activeDocMetadata.value.name,
		icon: activeDocMetadata.value.icon,
		protected: protectedMode,
	})
}

function handleBranchMerge() {
	// NOTE: for now this assumes that there are 2 branches but this will change
	// in the future
	if (!editorStore.mappedDefaultBranchId) {
		return
	}

	editorStore.updateActiveBranchId(editorStore.mappedDefaultBranchId)
	editorStore.updateTargetBranchId(null)
}

function clearLinkedBlock() {
	if (!linkedBlockHash.value) {
		return
	}

	clearTiptapScrollElementHighlightOverlays()
	linkedBlockHash.value = ""
	void pageRouter.replace({
		path: pageRoute.path,
		query: branchQuery(),
		hash: "",
	})
}
</script>
<template>
	<ShadcnUiSidebarProvider>
		<ShadcnUiTooltipProvider>
			<AppSidebar
				:all-initial-sections-loaded="allSectionsLoaded"
				:notification-sidebar-open="notificationSidebarOpen"
				@create-document="handleDocumentCreation"
				@duplicate-document="handleDocumentDuplication"
				@delete-document="handleDocumentDeletion"
				@delete-tag="handleTagDeletion"
				@open-settings="(target) => (settingModalOpen = target)"
				@toggle-settings="settingModalOpen = !settingModalOpen"
				@toggle-shortcuts="shortcutsModalOpen = !shortcutsModalOpen"
				@toggle-notifications="toggleNotificationSidebar"
				@initial-load-complete="loadedSections.sidebar = true"
			/>
			<NotificationSidebar v-model="notificationSidebarOpen" />
			<SettingsBaseModal
				v-model="settingModalOpen"
				@refresh-organization-slug="refreshOrganizationRouteSlug"
			/>
			<ShortcutModal v-model="shortcutsModalOpen" />
			<EditorBlocksMetricsConfigModalBaseModal
				@open-settings="() => (settingModalOpen = 'metric-data-sources')"
			/>
			<DocumentDeletionModal
				v-if="activeDocMetadata && allSectionsLoaded"
				v-model="pendingDocDeletion"
			/>
			<TagDeletionModal v-model="pendingTagDeletion" />
			<main ref="main" class="w-full min-w-0 bg-background">
				<EditorIconPickerProvider>
					<DocumentHeader
						:all-initial-sections-loaded="allSectionsLoaded"
						:breadcrumbs="breadcrumbs"
						:timestamps="timestamps"
						@delete-document="handleDocumentDeletion(null)"
						@duplicate-document="handleDocumentDuplication"
					/>
					<div class="relative h-auto">
						<Transition
							v-bind="
								defaultTransitionProps /*this transition is needed when switching between no document and document editor*/
							"
						>
							<EditorNoDocumentIndicator
								v-if="!activeDocMetadata"
								:all-sections-loaded="allSectionsLoaded"
								:create-loading="createLoading"
								class="absolute top-0 left-0 w-full"
								@create-document="() => handleDocumentCreation(null, true)"
								@load-complete="loadedSections.documentEditor = true"
							/>
							<div v-else>
								<!--a wrapper is needed to prevent transitions on :key change-->
								<EditorDocumentContainer
									:key="activeDocMetadata.id"
									:all-initial-sections-loaded="allSectionsLoaded"
									:timestamps="timestamps"
									@click="clearLinkedBlock"
									@updated-live-name="applyDocumentNameChange"
									@updated-live-icon="applyIconChange"
									@branch-merged="handleBranchMerge"
									@initial-load-complete="loadedSections.documentEditor = true"
									@fade-in-complete="pageReady = true"
									@open-settings="
										(target: 'github') => (settingModalOpen = target)
									"
									@editor-ready="(v: Editor) => (contentEditorRef = v)"
									@name-editor-ready="(v: Editor) => (nameEditorRef = v)"
								/>
							</div>
						</Transition>
					</div>
				</EditorIconPickerProvider>
			</main>
			<AIChatSidebar v-if="isAssistantEnabled" />
		</ShadcnUiTooltipProvider>
	</ShadcnUiSidebarProvider>
</template>
