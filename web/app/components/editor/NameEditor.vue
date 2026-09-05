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
import HooksMenuContent from "./hooks/HookMenuContent.vue"
import MaintainerList from "./MaintainerList.vue"
import DocumentTagList from "./DocumentTagList.vue"
import ReviewerList from "./ReviewerList.vue"
import { showToastMessage } from "../toast"
import {
	DEFAULT_HIGHLIGHT_OVERLAY_PADDING,
	type HighlightOverlayRect,
	useHighlightOverlay,
} from "./highlight-overlay"

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

// a page with no hooks of its own has no status to show, which leaves the
// handle its neutral colour rather than tinting it as healthy
const hookStatus = computed(() => {
	const hooks = props.documentHooks.filter((h) => h.blockId === null)
	if (!hooks.length) {
		return null
	}

	return hooks.some((h) => Number(h.score) === 0) ? "stale" : "fresh"
})
const hookMenuOpen = ref(false)
const hoveringHookHandle = ref(false)
const rootElem = useTemplateRef<HTMLElement>("name-editor-root")
const { isEditable } = useEditorMeta()
const { isScrolling } = useWindowScroll()
const { show: showHighlight, hide: hideHighlight } = useHighlightOverlay()
// the hook handle edits the page's hooks, so it follows the same rule as
// the block handles: gone while the page cannot be edited, and gone in
// diff mode, where the title is a rendering of two branches rather than
// an editor
const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
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
	hideHighlight()
	activeBranchIconFragment.unobserve(activeBranchIconObserverCallback)

	if (targetBranchIconFragment && targetBranchIconObserverCallback) {
		targetBranchIconFragment.unobserve(targetBranchIconObserverCallback)
	}
})

// the panel marks what the handle points at, so it follows the handle's
// hover and outlasts it for as long as the menu it opened is up
watch([hoveringHookHandle, hookMenuOpen], ([hovering, menuOpen]) => {
	if (!hovering && !menuOpen) {
		hideHighlight()
		return
	}

	const rect = documentRect()
	if (!rect) {
		return
	}

	showHighlight(rect, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)
})

// the panel is placed from a viewport rect, so a scroll would leave it
// behind the title it covers
whenever(isScrolling, hideHighlight)

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

// a page's hooks belong to the whole document, so the panel spans the
// title, the metadata under it and the content below, rather than the one
// row the handle sits beside.
//
// Only the bottom edge comes from the content editor: its element runs the
// full width of the page and insets its blocks with margins matching this
// row, so its own box would carry the page's side spacing into the panel
function documentRect(): HighlightOverlayRect | null {
	const header = rootElem.value?.getBoundingClientRect()
	if (!header) {
		return null
	}

	const content = props.contentEditor?.view.dom.getBoundingClientRect()
	const bottom = Math.max(header.bottom, content?.bottom ?? header.bottom)

	return {
		left: header.left,
		top: header.top,
		width: header.width,
		height: bottom - header.top,
	}
}

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
		ref="name-editor-root"
		class="relative mt-[0.8rem] mb-2 w-[calc(100%-2.5rem)] lg:mt-8 lg:mb-5 lg:w-[calc(100%-6.25rem)]"
	>
		<div
			v-show="hookStatus === 'stale'"
			class="pointer-events-none absolute top-1/2 -left-5 h-full w-1.25 -translate-y-1/2 rounded-r-lg bg-hook-decoration lg:-left-12.5 lg:h-[calc(100%+2rem)]"
		/>
		<div
			class="flex flex-col-reverse items-start gap-2 sm:flex-row sm:justify-between"
		>
			<div
				class="group/name-row relative flex flex-1 items-start gap-2 text-foreground"
			>
				<ShadcnUiDropdownMenu
					v-if="!isEditingDisabled"
					@update:open="(v: boolean) => (hookMenuOpen = v)"
				>
					<ShadcnUiDropdownMenuTrigger as-child>
						<div
							:data-menu-open="hookMenuOpen ? '' : undefined"
							:class="
								cn(
									'group/hook-handle absolute top-0.5 right-full flex h-7 items-center pr-1 lg:pr-1.5',
									'pointer-events-none opacity-0 transition-opacity duration-100',
									'group-hover/name-row:pointer-events-auto group-hover/name-row:opacity-100',
									'data-menu-open:pointer-events-auto data-menu-open:opacity-100',
								)
							"
							@mouseenter="hoveringHookHandle = true"
							@mouseleave="hoveringHookHandle = false"
						>
							<div
								:class="
									cn(
										'flex size-4 cursor-pointer items-center justify-center rounded-md text-foreground/50 lg:size-5.5',
										'group-data-menu-open/hook-handle:bg-sidebar-accent/50 hover:bg-sidebar-accent/50 active:bg-sidebar-accent',
									)
								"
							>
								<Icon
									:data-hook-status="hookStatus"
									name="mingcute:leaf-line"
									class="mt-0.25 size-3.5 data-[hook-status=fresh]:text-hook-status-fresh data-[hook-status=stale]:text-hook-status-stale lg:size-4.5"
								/>
							</div>
							<span class="sr-only">
								{{ $t("editor.hook-handle.screen-reader-hint") }}
							</span>
						</div>
					</ShadcnUiDropdownMenuTrigger>
					<ShadcnUiDropdownMenuContent side="right" align="start" loop>
						<HooksMenuContent
							:document-hooks="documentHooks"
							:node-id="null"
							@open-settings="(v) => emit('open-settings', v)"
						/>
					</ShadcnUiDropdownMenuContent>
				</ShadcnUiDropdownMenu>
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
