<script lang="ts" setup>
import * as Y from "yjs"
import { HocuspocusProvider } from "@hocuspocus/provider"
import type { Editor } from "@tiptap/core"
import ContentEditor from "./ContentEditor.vue"
import NameEditor from "./NameEditor.vue"
import DiffEditor from "./diff/DiffEditor.vue"
import { redirectToLogin } from "~/plugins/03.api-fetch"
import { refreshGapDecorationsInBackground } from "./drag-handle/gap-decorations"
import { editorCaretColors } from "~/assets/css"

type BranchProviderEntry = {
	provider: HocuspocusProvider
	ydoc: Y.Doc
	synced: Ref<boolean>
}

// NOTE: this component is remounted every time the document ID
// is changed (page change)
const props = defineProps<{
	allInitialSectionsLoaded: boolean
	timestamps: DocumentTimestamps
}>()
const emit = defineEmits<{
	(e: "updated-live-icon" | "updated-live-name", val: string): void
	(e: "initial-load-complete"): void
	(e: "branch-merged", deleted: boolean): void
	(e: "open-settings", target: "github"): void
	(e: "editor-ready" | "name-editor-ready", val: Editor): void
}>()

const config = useRuntimeConfig()
const documentHookAPI = useDocumentHookAPI()
const { t } = useI18n({ useScope: "global" })
const { fetchAuthSession } = useAuthSession()
const editorStore = useEditorStore()
const { useFetchDocumentBranchesByDocId } = useDocumentAPI()
const fetchBranches = useFetchDocumentBranchesByDocId(
	() => editorStore.activeDocumentId,
)
const { isEditable, setEditable } = useEditorMeta()
const route = useRoute()
const nuxtApp = useNuxtApp()
const fetchDocumentHooks = documentHookAPI.useFetchDocumentHooksByDocID(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)

const userCaretDetails = computed(() => {
	const caretColors = editorCaretColors()
	const randomColor =
		caretColors[Math.floor(Math.random() * caretColors.length)]!

	return {
		name:
			fetchAuthSession.state.value.data?.data?.user.name ||
			t("editor.unknown-user-caret"),
		color: randomColor.value,
	}
})

const activeBranch = computed(() => {
	const branches = fetchBranches.state.value.data
	if (!branches || !editorStore.activeBranchId) {
		return null // still loading branches
	}

	return branches.find((b) => b.branchId === editorStore.activeBranchId) ?? null
})

const branchProviders = shallowReactive(new Map<string, BranchProviderEntry>())
const activeBranchProvider = computed<BranchProviderEntry | null>(() => {
	const bid = editorStore.activeBranchId
	if (bid && branchProviders.has(bid)) {
		return branchProviders.get(bid)!
	}

	return null
})
const targetBranchProvider = computed<BranchProviderEntry | null>(() => {
	const bid = editorStore.targetBranchId
	if (bid && branchProviders.has(bid)) {
		return branchProviders.get(bid)!
	}

	return null
})

const nameEditor = ref<Editor | null>(null)
const contentEditor = ref<Editor | null>(null)

import.meta.hot?.accept(() => {
	refreshGapDecorationsInBackground(contentEditor as Ref<Editor | null>)
})

onBeforeUnmount(() => {
	for (const [, entry] of branchProviders) {
		entry.provider.destroy()
	}

	branchProviders.clear()
})

watchImmediate(
	[
		() => editorStore.preloadedBranchIds,
		() => editorStore.activeBranchId,
		() => editorStore.targetBranchId,
	],
	(
		[preloadedBranchIds, activeBranchId, targetBranchId],
		[_1, oldActiveBranchId],
	) => {
		if (!editorStore.activeDocumentId) {
			return
		}

		preloadedBranchIds.forEach((branchId) => {
			if (!branchProviders.has(branchId)) {
				const entry = createBranchProvider(
					`${editorStore.activeDocumentId}-${branchId}`,
					activeBranchId === branchId && !oldActiveBranchId, // treat as initial load if this is the active branch
				)
				branchProviders.set(branchId, entry)
			}
		})

		if (activeBranchId && !branchProviders.has(activeBranchId)) {
			const entry = createBranchProvider(
				`${editorStore.activeDocumentId}-${activeBranchId}`,
				oldActiveBranchId ? false : true, // only treat as initial load if there was no previous branch, otherwise it's a branch switch
			)
			branchProviders.set(activeBranchId, entry)
		}

		if (targetBranchId && !branchProviders.has(targetBranchId)) {
			const entry = createBranchProvider(
				`${editorStore.activeDocumentId}-${targetBranchId}`,
				false,
			)
			branchProviders.set(targetBranchId, entry)
		}

		// TODO: old branches need to be cleaned up, but doing that
		// immediately causes issues when switching between branches quickly,
		// as the provider is destroyed before the new one is ready. We
		// should probably add some delay before destroying the old provider,
		// and if the same branch becomes active again during that delay,
		// we cancel the destruction.
		/*
		if (oldActiveBranchId && oldActiveBranchId !== activeBranchId) {
			const entry = branchProviders.get(oldActiveBranchId)
			if (entry) {
				entry.provider.destroy()
				branchProviders.delete(oldActiveBranchId)
			}
		}

		if (oldTargetBranchId && oldTargetBranchId !== targetBranchId) {
			const entry = branchProviders.get(oldTargetBranchId)
			if (entry) {
				entry.provider.destroy()
				branchProviders.delete(oldTargetBranchId)
			}
		}
		*/
	},
)

watch(contentEditor, (v) => {
	if (v) {
		emit("editor-ready", v as Editor)
	}
})

watch(nameEditor, (v) => {
	if (v) {
		emit("name-editor-ready", v as Editor)
	}
})

watchImmediate([isEditable, activeBranch, nameEditor, contentEditor], () => {
	const editable = activeBranch.value ? !activeBranch.value?.protected : false
	setEditable(editable)

	if (nameEditor.value) {
		nameEditor.value.setEditable(editable)
	}

	if (contentEditor.value) {
		contentEditor.value.setEditable(editable)
	}
})

function createBranchProvider(
	streamName: string,
	isInitial: boolean,
): BranchProviderEntry {
	const ydoc = new Y.Doc()
	const synced = ref(false)
	const provider = new HocuspocusProvider({
		url: config.public.authRealtimeAPIBaseWsURL as string,
		name: streamName,
		document: ydoc,
		token: "notoken",
		onAuthenticated() {
			// TODO handle edit permissions properly?
		},
		onAuthenticationFailed() {
			redirectToLogin(route.fullPath, true, nuxtApp)
		},
		onStateless({ payload }) {
			const data = JSON.parse(payload)
			if (data.type === "error") {
				// TODO show toast
				console.error(`Error in provider ${streamName}:`, data)
			}
		},
		onConnect: () => {
			if (isInitial) {
				refreshGapDecorationsInBackground(contentEditor as Ref<Editor | null>)
			}
		},
		onSynced: () => {
			synced.value = true
			if (isInitial) {
				emit("initial-load-complete")
				refreshGapDecorationsInBackground(contentEditor as Ref<Editor | null>)
			}
		},
	})

	return { provider, ydoc, synced }
}

function updateEditor(editor: Editor) {
	contentEditor.value = editor

	// force placeholder decorations to recompute
	contentEditor.value?.chain().run()
}

function handleDiffModeChange() {
	// force placeholder decorations to recompute
	contentEditor.value?.chain().run()
}

function showSettings(target: "github") {
	emit("open-settings", target)
}
</script>
<template>
	<Transition v-bind="defaultTransitionProps">
		<div
			v-if="
				editorStore.activeBranchId &&
				props.allInitialSectionsLoaded &&
				activeBranchProvider?.synced.value
			"
			class="flex h-full"
		>
			<!--
			a wrapper is needed to prevent transitions on :key change;
			also, we want to remount editors when the active branch changes,
			to avoid issues with the editors not properly updating when
			the Y.Doc/data provider changes
			-->
			<div
				:key="editorStore.activeBranchId"
				:class="[
					SCROLL_HIGHLIGHT_EDITOR_CLASS,
					'relative flex h-full w-full flex-col items-center',
				]"
			>
				<NameEditor
					:document-hooks="fetchDocumentHooks.state.value.data || []"
					:timestamps="props.timestamps"
					:active-branch-ydoc="activeBranchProvider.ydoc"
					:active-branch-provider="activeBranchProvider.provider"
					:target-branch-provider="targetBranchProvider?.provider"
					:content-editor="contentEditor as Editor"
					:user-caret-details="userCaretDetails"
					@updated-live-icon="(v) => emit('updated-live-icon', v)"
					@updated-live-name="(v) => emit('updated-live-name', v)"
					@editor-ready="(v) => (nameEditor = v)"
					@branch-merged="(deleted) => emit('branch-merged', deleted)"
					@open-settings="(target: 'github') => showSettings(target)"
				/>
				<ContentEditor
					v-show="!editorStore.reviewableDiffActive"
					:active-branch-provider="activeBranchProvider.provider"
					:active-branch-ydoc="activeBranchProvider.ydoc"
					:document-hooks="fetchDocumentHooks.state.value.data || []"
					:name-editor="nameEditor as Editor"
					:user-caret-details="userCaretDetails"
					@editor-ready="(v) => updateEditor(v)"
					@open-settings="(target: 'github') => showSettings(target)"
				/>
				<DiffEditor
					v-if="
						editorStore.reviewableDiffActive &&
						targetBranchProvider &&
						activeBranchProvider !== targetBranchProvider
					"
					:target-branch-ydoc="targetBranchProvider.ydoc"
					:active-branch-ydoc="activeBranchProvider.ydoc"
					:content-editor="contentEditor as Editor"
					@diff-mode-changed="handleDiffModeChange"
				/>
			</div>
		</div>
	</Transition>
</template>
