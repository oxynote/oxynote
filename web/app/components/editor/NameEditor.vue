<script lang="ts" setup>
import { ref } from "vue"
import { useEditor as useTiptapEditor, EditorContent } from "@tiptap/vue-3"
import { cn } from "@/lib/utils"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import Collaboration from "@tiptap/extension-collaboration"
import CollaborationCaret from "@tiptap/extension-collaboration-caret"
import type * as Y from "yjs"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import { type Editor, Extension } from "@tiptap/core"
import IconPicker from "./IconPicker.vue"
import DiffTitle from "./diff/DiffTitle.vue"
import { defaultNamePlaceholder } from "./placeholder"
import { renderCollaborationCaret } from "./collaboration"
import IconStack, { type IconMetadata } from "./IconStack.vue"
import HooksMenuContent from "./hooks/HookMenuContent.vue"
import MaintainerList from "./MaintainerList.vue"
import DocumentTagList from "./DocumentTagList.vue"
import ReviewerList from "./ReviewerList.vue"
import { prepareHookMetadata } from "./hooks"
import { showToastMessage } from "../toast"

enum ReviewableAction {
	ApproveUnapprove = "approve-unapprove",
	Merge = "merge",
}

const props = defineProps<{
	documentHooks: DocumentHook[]
	activeBranchYdoc: Y.Doc
	activeBranchProvider: HocuspocusProvider
	targetBranchProvider?: HocuspocusProvider | null | undefined
	contentEditor: Editor | null | undefined
	userCaretDetails: { name: string; color: string }
	timestamps: DocumentTimestamps
}>()
const emit = defineEmits<{
	(e: "updated-live-icon" | "updated-live-name", val: string): void
	(e: "branch-merged", deleted: boolean): void
	(e: "editor-ready", val: Editor): void
	(e: "open-settings", target: "github"): void
	// eslint-disable-next-line @typescript-eslint/unified-signatures
	(e: "diff-mode-changed", active: boolean): void
}>()

const { t } = useI18n({ useScope: "global" })
const {
	useFetchBranchReviewers,
	useFetchDocumentBranchesByDocId,
	mergeDocumentBranches,
	updateBranchApproval,
} = useDocumentAPI()
const editorStore = useEditorStore()
const fetchBranches = useFetchDocumentBranchesByDocId(
	() => editorStore.activeDocumentId,
)
const fetchCurrentBranchReviewers = useFetchBranchReviewers(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)
const { fetchAuthSession } = useAuthSession()

const isReviewable = computed(() => {
	// NOTE this will be removed in the future
	const branches = fetchBranches.state.value.data
	return !!branches && branches.length > 1
})

const activeBranchIcon = ref<string | null>(null)
const targetBranchIcon = ref<string | null>(null)

const activeBranchIconFragment =
	props.activeBranchProvider.document.getText("icon")
const targetBranchIconFragment =
	props.targetBranchProvider?.document.getText("icon") ?? null

const showTitleDiff = computed(
	() => editorStore.reviewableDiffActive && !!props.targetBranchProvider,
)

const hookMetadata = computed<IconMetadata[]>(() => {
	return props.documentHooks
		.filter((h) => {
			return h.blockId === null
		})
		.map((h) => {
			const res = prepareHookMetadata(h.id, h.type, t)
			if (res) {
				return {
					...res,
					staleHook: Number(h.score) === 0,
					hookUpdatedAt: h.updatedAt ?? new Date(0),
				}
			}

			return {
				id: h.id,
				name: h.type,
				icon: "lucide:help-circle",
				staleHook: Number(h.score) === 0,
				hookUpdatedAt: h.updatedAt ?? new Date(0),
			}
		})
})
const hookStatus = computed(() => {
	return props.documentHooks
		.filter((h) => h.blockId === null)
		.some((h) => Number(h.score) === 0)
		? "stale"
		: "fresh"
})
const hookMenuOpen = ref(false)
const { isEditable } = useEditorMeta()
const safeHookMenuOpen = computed(() => {
	return !isEditable.value ? false : hookMenuOpen.value
})
const selectedReviewableAction = ref<ReviewableAction>(
	ReviewableAction.ApproveUnapprove,
)
const processedReviewableAction = computed<ReviewableAction | null>({
	get: () => {
		if (!editorStore.branchReviewableActionsActive) {
			return null
		}

		return selectedReviewableAction.value
	},
	set: (val: ReviewableAction | null) => {
		if (!editorStore.branchReviewableActionsActive) {
			val = ReviewableAction.ApproveUnapprove
		}

		val ??= ReviewableAction.ApproveUnapprove

		selectedReviewableAction.value = val
	},
})
const reviewableActionLoading = ref(false)
const approvedByActiveUser = computed(() => {
	const currentUserId = fetchAuthSession.state.value.data?.data?.user.id
	if (!currentUserId) {
		return false
	}

	const currentUserReviewer =
		fetchCurrentBranchReviewers.state.value.data?.find(
			(r) => r.userId === currentUserId,
		)

	return currentUserReviewer?.currentlyApproved ?? false
})

// toJSON is the typed alias of YText.toString, which the bundled yjs
// declarations omit
activeBranchIcon.value = activeBranchIconFragment.toJSON()
const activeBranchIconObserverCallback = () => {
	activeBranchIcon.value = activeBranchIconFragment.toJSON()
	emit("updated-live-icon", activeBranchIcon.value)
}
activeBranchIconFragment.observe(activeBranchIconObserverCallback)

let targetBranchIconObserverCallback: (() => void) | null = null
if (targetBranchIconFragment) {
	targetBranchIcon.value = targetBranchIconFragment.toJSON()
	targetBranchIconObserverCallback = () => {
		targetBranchIcon.value = targetBranchIconFragment.toJSON()
	}

	targetBranchIconFragment.observe(targetBranchIconObserverCallback)
}

const nameEditor = useTiptapEditor({
	editorProps: {
		attributes: {
			class: cn(
				"focus:outline-none w-full max-w-none",
				"text-2xl font-semibold",
			),
			spellcheck: "false",
		},
	},
	extensions: [
		Document,
		Text,
		Paragraph.configure({
			HTMLAttributes: {
				class: cn("break-all whitespace-normal"),
			},
		}),
		defaultNamePlaceholder(t),
		Collaboration.configure({
			// https://tiptap.dev/docs/hocuspocus/provider/examples#tiptap
			document: props.activeBranchYdoc,
			field: "name",
		}),
		CollaborationCaret.configure({
			provider: props.activeBranchProvider,
			user: props.userCaretDetails,
			render: renderCollaborationCaret,
		}),
		Extension.create({
			name: "focusChange",
			addKeyboardShortcuts() {
				return {
					Enter: () => {
						props.contentEditor?.chain().focus("start").run()
						return true
					},
					Tab: () => {
						props.contentEditor?.chain().focus("start").run()
						return true
					},
				}
			},
		}),
	],
	onCreate: ({ editor }) => {
		emit("editor-ready", editor)
	},
	onUpdate: (ev) => {
		emit("updated-live-name", ev.editor.getText())
	},
	onContentError: ({ editor, disableCollaboration }) => {
		editor.setEditable(false)
		disableCollaboration()
		showToastMessage("error", t("editor.errors.content-load-failed"))
	},
})

onBeforeUnmount(() => {
	activeBranchIconFragment.unobserve(activeBranchIconObserverCallback)

	if (targetBranchIconFragment && targetBranchIconObserverCallback) {
		targetBranchIconFragment.unobserve(targetBranchIconObserverCallback)
	}
})

watch(
	[
		() => editorStore.activeDocumentId,
		() => editorStore.activeBranchId,
		() => editorStore.targetBranchId,
		() => editorStore.branchReviewableActionsActive,
	],
	() => {
		selectedReviewableAction.value = ReviewableAction.ApproveUnapprove
		editorStore.setReviewableDiffActive(false)
	},
)

function updateDiffMode(active: boolean) {
	// store the scroll position because the window will scroll to the top
	// because one editor disappears and the other appears when toggling diff
	// mode, and this might not happen at the same time.
	const scrollY = window.scrollY

	editorStore.setReviewableDiffActive(active)
	emit("diff-mode-changed", active)

	void nextTick(() => {
		requestAnimationFrame(() => {
			window.scrollTo({ top: scrollY })
		})
	})
}

function selectIcon(v: string) {
	activeBranchIconFragment.delete(0, activeBranchIconFragment.length)
	activeBranchIconFragment.insert(0, v)
}

async function executeReviewableAction() {
	if (
		!processedReviewableAction.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId ||
		!editorStore.targetBranchId
	) {
		return
	}

	reviewableActionLoading.value = true
	await delay(300) // small delay to show loading state

	// TODO make editor read only during loading (styles shouldn't changed)

	switch (processedReviewableAction.value) {
		case ReviewableAction.ApproveUnapprove: {
			const newStatus = !approvedByActiveUser.value

			try {
				await updateBranchApproval.mutateAsync({
					docId: editorStore.activeDocumentId,
					branchId: editorStore.activeBranchId,
					approved: newStatus,
				})
				showToastMessage(
					"success",
					newStatus
						? t("editor.name-editor.review-workflow.approve.success")
						: t("editor.name-editor.review-workflow.unapprove.success"),
				)
			} catch {
				showToastMessage(
					"error",
					newStatus
						? t("editor.name-editor.review-workflow.approve.error")
						: t("editor.name-editor.review-workflow.unapprove.error"),
				)
			}

			break
		}
		case ReviewableAction.Merge:
			try {
				await mergeDocumentBranches.mutateAsync({
					docId: editorStore.activeDocumentId,
					fromBranchId: editorStore.activeBranchId,
					toBranchId: editorStore.targetBranchId,
				})
				emit("branch-merged", false)
				showToastMessage(
					"success",
					t("editor.name-editor.review-workflow.merge.success"),
				)
			} catch {
				showToastMessage(
					"error",
					t("editor.name-editor.review-workflow.merge.error"),
				)
			}

			break
	}

	reviewableActionLoading.value = false
}
</script>
<template>
	<div
		class="relative mt-[0.8rem] mb-2 w-[calc(100%-2.5rem)] lg:mt-8 lg:mb-5 lg:w-[calc(100%-6.25rem)]"
	>
		<div
			v-show="hookStatus === 'stale'"
			class="pointer-events-none absolute top-1/2 -left-5 h-full w-1.25 -translate-y-1/2 rounded-r-lg bg-hook-decoration lg:-left-12.5 lg:h-[calc(100%+2rem)]"
		/>
		<div
			class="flex flex-col-reverse items-start gap-2 sm:flex-row sm:justify-between"
		>
			<div class="flex flex-1 items-start gap-2 text-foreground">
				<IconPicker
					:icon="activeBranchIcon"
					:is-modified="
						targetBranchIcon !== null &&
						targetBranchIcon !== activeBranchIcon &&
						editorStore.reviewableDiffActive
					"
					@select="selectIcon"
				/>
				<EditorContent
					v-show="!showTitleDiff"
					class="group/name-editor flex-1"
					:editor="nameEditor"
				/>
				<DiffTitle
					v-if="showTitleDiff"
					:target-branch-ydoc="props.targetBranchProvider!.document"
					:active-branch-ydoc="props.activeBranchProvider.document"
					class="group/name-editor flex-1"
				/>
			</div>
			<div
				v-if="processedReviewableAction && editorStore.activeBranchId"
				class="mt-0.5 flex w-full items-center justify-between gap-2 sm:w-auto"
			>
				<div
					v-if="props.timestamps[editorStore.activeBranchId]"
					class="block text-2base whitespace-nowrap text-muted-foreground sm:hidden"
				>
					{{
						$t("editor.navbar.timestamp.short-edited", {
							time: $d(
								props.timestamps[editorStore.activeBranchId]!.updated.at,
								"month-day-short",
							),
						})
					}}
				</div>
				<ShadcnUiButtonGroup>
					<ShadcnUiButton
						size="2sm"
						:disabled="reviewableActionLoading"
						@click="executeReviewableAction"
					>
						{{
							processedReviewableAction === "approve-unapprove"
								? !approvedByActiveUser
									? $t(`editor.name-editor.review-workflow.approve.title`)
									: $t(`editor.name-editor.review-workflow.unapprove.title`)
								: $t(`editor.name-editor.review-workflow.merge.title`)
						}}
					</ShadcnUiButton>
					<ShadcnUiButtonGroupSeparator />
					<ShadcnUiDropdownMenu>
						<ShadcnUiDropdownMenuTrigger as-child>
							<ShadcnUiButton
								size="2sm"
								class="w-7.5 px-0"
								:disabled="reviewableActionLoading"
							>
								<Icon
									v-if="!reviewableActionLoading"
									name="lucide:chevron-down"
									size="0.9rem"
									class="mt-0.5"
								/>
								<Icon
									v-else
									name="svg-spinners:blocks-shuffle-3"
									size="0.8rem"
									class="mt-0.5"
								/>
							</ShadcnUiButton>
						</ShadcnUiDropdownMenuTrigger>
						<ShadcnUiDropdownMenuContent
							align="end"
							side="bottom"
							loop
							class="max-w-75"
						>
							<ShadcnUiDropdownMenuItem
								@click="
									processedReviewableAction = ReviewableAction.ApproveUnapprove
								"
							>
								<div class="flex flex-col gap-0.5">
									<div class="text-2sm font-medium">
										{{
											!approvedByActiveUser
												? $t(`editor.name-editor.review-workflow.approve.title`)
												: $t(
														`editor.name-editor.review-workflow.unapprove.title`,
													)
										}}
									</div>
									<div class="text-xs text-muted-foreground">
										{{
											!approvedByActiveUser
												? $t(
														`editor.name-editor.review-workflow.approve.description`,
													)
												: $t(
														`editor.name-editor.review-workflow.unapprove.description`,
													)
										}}
									</div>
								</div>
							</ShadcnUiDropdownMenuItem>
							<div class="mx-1.75 my-0.5 h-px bg-border" />
							<ShadcnUiDropdownMenuItem
								@click="processedReviewableAction = ReviewableAction.Merge"
							>
								<div class="flex flex-col gap-0.5">
									<div class="text-2sm font-medium">
										{{ $t(`editor.name-editor.review-workflow.merge.title`) }}
									</div>
									<div class="text-xs text-muted-foreground">
										{{
											$t(`editor.name-editor.review-workflow.merge.description`)
										}}
									</div>
								</div>
							</ShadcnUiDropdownMenuItem>
						</ShadcnUiDropdownMenuContent>
					</ShadcnUiDropdownMenu>
				</ShadcnUiButtonGroup>
			</div>
		</div>
		<div class="mt-2.5 flex items-center justify-between gap-2">
			<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
				<DocumentTagList />
				<ShadcnUiDropdownMenu
					:open="safeHookMenuOpen"
					@update:open="(v: boolean) => (hookMenuOpen = v)"
				>
					<ShadcnUiDropdownMenuTrigger as-child>
						<IconStack
							:data-disabled="!isEditable ? '' : undefined"
							class="cursor-pointer data-disabled:cursor-default"
							:title="$t('editor.name-editor.hooks')"
							:icons="hookMetadata"
							:disabled="!isEditable"
						/>
					</ShadcnUiDropdownMenuTrigger>
					<ShadcnUiDropdownMenuContent side="bottom" align="start" loop>
						<HooksMenuContent
							:document-hooks="documentHooks"
							:node-id="null"
							@open-settings="(v) => emit('open-settings', v)"
						/>
					</ShadcnUiDropdownMenuContent>
				</ShadcnUiDropdownMenu>
				<div
					v-if="editorStore.branchReviewableActionsActive"
					class="flex items-center gap-1"
				>
					<ShadcnUiLabel
						for="show-diff"
						class="text-sm font-medium text-muted-foreground"
					>
						{{ $t("editor.name-editor.review-workflow.show-diff") }}
					</ShadcnUiLabel>
					<ShadcnUiSwitch
						id="show-diff"
						:model-value="editorStore.reviewableDiffActive"
						class="mt-0.5"
						@update:model-value="updateDiffMode"
					/>
				</div>
			</div>
			<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
				<MaintainerList />
				<ReviewerList v-if="isReviewable" />
			</div>
		</div>
	</div>
</template>
