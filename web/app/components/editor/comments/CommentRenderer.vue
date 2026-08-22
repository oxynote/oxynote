<script setup lang="ts">
import { useEditor, EditorContent, posToDOMRect } from "@tiptap/vue-3"
import type { Editor } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import type { EditorState, Transaction } from "@tiptap/pm/state"
import { computePosition, flip, offset, shift } from "@floating-ui/dom"
import { cn } from "@/lib/utils"
import Comment from "./Comment.vue"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Strike from "@tiptap/extension-strike"
import Blockquote from "@tiptap/extension-blockquote"
import Underline from "@tiptap/extension-underline"
import {
	CommentClass,
	CommentExtensions,
	createPendingCommentId,
	deleteCommentFromEditor,
	deletePendingCommentData,
	isPendingCommentId,
	pendingCommentBelongsToActiveUser,
} from "./utils"
import LinkBubbleMenu from "../link/LinkBubbleMenu.vue"
import { defaultContentPlaceholder } from "../placeholder"
import {
	findCommentMarkAtPos,
	findCommentMarkById,
	mergedOffsetToOriginalOffset,
	reinjectDeletedContentMarks,
	type CommentAttrs,
} from "./comment-mark"
import { isChangeOrigin } from "@tiptap/extension-collaboration"
import { compareAsc, formatDistanceToNowStrict } from "date-fns"
import { showToastMessage } from "~/components/toast"
import {
	findNodeCommentAtPos,
	findNodeCommentById,
	reinjectDeletedContentNodeComments,
	type NodeCommentMatch,
} from "./node-comment-extension"
import { CODE_BLOCK_NAME } from "../blocks/node-names"
import {
	COMMENT_MARK_NAME,
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "../mark-names"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import { DiffStatus, segmentSelectionByDiffStatus } from "../diff/position-map"
import type { PositionMap } from "../diff/position-map"
import type {
	DocumentCommentDiffDeletionContext,
	DocumentCommentCreateRequest,
} from "~/utils/api/comment"

// Use defineAsyncComponent to prevent vue-virtual-scroller from being
// processed during SSR build, as it accesses window/document on import
const DynamicScroller = defineAsyncComponent(() =>
	import("vue-virtual-scroller").then((m) => m.DynamicScroller),
)
const DynamicScrollerItem = defineAsyncComponent(() =>
	import("vue-virtual-scroller").then((m) => m.DynamicScrollerItem),
)
type DynamicScrollerType = ComponentPublicInstance & {
	scrollToBottom(): void
}

const NOW_TIME_LABEL_THRESHOLD_MS = 1000 * 60 // 1 minute
const LEFT_NODE_COMMENT_OFFSET = -6
const TOP_NODE_COMMENT_OFFSET_SIDE_TOP = -16
const TOP_NODE_COMMENT_OFFSET_SIDE_BOTTOM = 4

const props = defineProps<{
	contentEditor: Editor
	container: HTMLElement | null
	/** when provided, the comment renderer operates in diff mode */
	diffContext?: {
		diffEditor: Editor
		positionMap: PositionMap
		suppressNextRecompute: () => void
	}
}>()
defineExpose({
	isCommentPopoverOpen,
	selectComment,
	addNewComment,
})

const isDiffMode = computed(() => !!props.diffContext)
/** the editor where selection/anchor lives — diff editor in diff mode, content editor otherwise */
const activeEditor = computed(
	() => props.diffContext?.diffEditor ?? props.contentEditor,
)

const { t, locale } = useI18n({ useScope: "global" })
const editor = useEditor({
	editorProps: {
		attributes: {
			class: cn(
				CommentClass,
				"border rounded-md px-2 overflow-auto resize-y max-h-40 py-2 min-h-[2.125rem]",
			),
			spellcheck: "false",
		},
	},
	extensions: [
		...CommentExtensions,
		defaultContentPlaceholder(t, false, "comment"),
	],
	content: "",
})

const { fetchOrganization, fetchAuthSession } = useAuthSession()
const commentAPI = useDocumentCommentAPI()
const editorStore = useEditorStore()
const fetchComments = commentAPI.useFetchDocumentCommentsByDocId(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)
const wsState = useWebSocketStateStore()
let unsubWsCommentChange: (() => void) | null | undefined = null

const userData = computed(() => {
	const id = fetchAuthSession.state.value.data?.data?.user.id || null
	return {
		id: id,
	}
})
const activePendingId = ref<string | null>(null)
const popoverElem = useTemplateRef("comment-popover")
const popoverPosition = ref({
	left: 0,
	top: 0,
	placement: "bottom-start",
})
const containerElem = useTemplateRef("editor-container")
const scrollerElem = useTemplateRef<DynamicScrollerType>("comment-scroller")
const canScrollToBottom = ref(false)
const anchorElem = ref<Element | null>(null)
const selectedComment = ref<{
	id: string
	textComment?: {
		from: number
		to: number
		attrs: CommentAttrs
	}
	nodeComment?: NodeCommentMatch
} | null>(null)
const recentlyClosedNodeCommentIds = ref<string[]>([]) // needed to prevent immediate re-opening (especially with click)
// suppresses reinjectDeletedContentMarks while a diff comment save
// is in flight. the optimistic insert (nanoid) would otherwise
// overwrite the pending marks before updateNewSelectedCommentId
// can replace them with the real server ID.
let savingDiffComment = false
const savedContentEditorSelection = ref<{ from: number; to: number } | null>(
	null,
)
const loadedComment = computed(() => {
	if (!selectedComment.value) {
		return null
	}

	const comments = fetchComments.state.value.data
	if (!comments) {
		return null
	}

	const comment = comments.find((c) => c.id === selectedComment.value?.id)
	if (!comment) {
		return null
	}

	return comment
})
const loadedCommentReplies = computed(() => {
	if (!loadedComment.value) {
		return null
	}

	const exist = !!loadedComment.value.replies?.length

	return {
		data: (loadedComment.value.replies ?? []).reduce<
			Record<string, DocumentCommentReply>
		>((acc, reply) => {
			acc[reply.id] = reply
			return acc
		}, {}),
		exist,
	}
})
const loadedCommentThread = computed<
	{
		id: string
		author: {
			name: string
			self: boolean
			image?: string | null
		}
		content: Record<string, any>
		reply: boolean
		createdAt: Date
		updatedAt?: Date | null
		edited: boolean
	}[]
>(() => {
	if (!loadedComment.value) {
		return []
	}

	const orgMembers = fetchOrganization.state.value.data?.data?.members
	const user = fetchAuthSession.state.value.data?.data?.user

	if (!orgMembers?.length || !user) {
		return []
	}

	const origAuthor = orgMembers.find(
		(m) => m.userId === loadedComment.value?.userId,
	)
	const res = [
		{
			id: loadedComment.value.id,
			author: {
				name:
					origAuthor?.user.name || t("editor.comment-thread.unknown-author"),
				self: loadedComment.value.userId === user.id,
				image: origAuthor?.user.image || null,
			},
			content: loadedComment.value.content,
			reply: false,
			createdAt: new Date(loadedComment.value.createdAt),
			updatedAt: loadedComment.value.updatedAt
				? new Date(loadedComment.value.updatedAt)
				: null,
			edited: !!loadedComment.value.updatedAt,
		},
	]

	Object.values(loadedCommentReplies.value?.data ?? {})
		.sort((a, b) => {
			return compareAsc(new Date(a.createdAt), new Date(b.createdAt))
		})
		.forEach((reply) => {
			const replyAuthor = orgMembers.find((m) => m.userId === reply.userId)
			res.push({
				id: reply.id,
				author: {
					name:
						replyAuthor?.user.name || t("editor.comment-thread.unknown-author"),
					self: reply.userId === user.id,
					image: replyAuthor?.user.image || null,
				},
				content: reply.content,
				reply: true,
				createdAt: new Date(reply.createdAt),
				updatedAt: reply.updatedAt ? new Date(reply.updatedAt) : null,
				edited: !!reply.updatedAt,
			})
		})

	return res
})
const editTarget = ref<{
	id: string
	reply: boolean
} | null>(null)

import.meta.hot?.dispose(() => {
	removeEditorListeners()
})

onMounted(() => {
	if (!isDiffMode.value) {
		activeEditor.value.on("transaction", handleRemoteDocChange)
	}

	activeEditor.value.view.dom.addEventListener("click", handleEditorClick)
	window.addEventListener("scroll", handleScrollOrResize, true)
	window.addEventListener("resize", handleScrollOrResize)
})

onBeforeUnmount(() => {
	removeEditorListeners()
})

watch(
	[() => editorStore.reviewableDiffActive, () => editorStore.activeBranchId],
	() => {
		closePopover(true)
	},
)

watchDeep(
	() => editorStore.mermaidBlockShowCode,
	async () => {
		await nextTick()
		void selectComment("refresh")
	},
)

watchImmediate(
	() => editorStore.activeDocumentId,
	(newV) => {
		if (!newV) {
			return
		}

		if (unsubWsCommentChange) {
			unsubWsCommentChange()
			unsubWsCommentChange = null
		}

		unsubWsCommentChange = wsState.state?.subscribe(
			makeWsDocumentCommentChangeTopic(newV),
			() => {
				// TODO: refetch only the affected branch comments
				void fetchComments.refetch()
			},
		)
	},
)

watchDeep(
	[selectedComment, anchorElem],
	async () => {
		await nextTick()
		await updatePopoverPosition()
	},
	{ immediate: true },
)

watch(
	() => selectedComment.value?.id,
	(newId, oldId) => {
		if (oldId) {
			activeEditor.value.commands.setCommentMarkForcedHighlight(oldId, false)
			activeEditor.value.commands.setNodeCommentForcedHighlight(oldId, false)
		}

		if (newId) {
			const comment = selectedComment.value
			if (comment?.textComment) {
				activeEditor.value.commands.setCommentMarkForcedHighlight(newId, true)
			} else if (comment?.nodeComment) {
				activeEditor.value.commands.setNodeCommentForcedHighlight(newId, true)
			}
		}
	},
)

watchDeep([fetchComments.state, fetchComments.isPending], () => {
	if (isDiffMode.value && props.diffContext?.diffEditor) {
		deletePendingCommentData(
			props.diffContext.diffEditor,
			userData.value.id || "",
			undefined,
			true,
			activePendingId.value,
		)
	}

	deletePendingCommentData(
		props.contentEditor,
		userData.value.id || "",
		undefined,
		undefined,
		activePendingId.value,
	)
})

// re-inject deleted-content marks and node comments after each diff
// recompute (position map updates) or when comments data arrives.
// both are needed because the position map may already be set when
// comments are still loading (page reload, re-entering diff mode).
watchImmediate(
	[() => props.diffContext?.positionMap, () => fetchComments.state.value.data],
	() => {
		if (!props.diffContext) {
			return
		}

		if (savingDiffComment) {
			return
		}

		const comments = fetchComments.state.value.data
		if (!comments?.length) {
			return
		}

		reinjectDeletedContentMarks(
			props.diffContext.diffEditor,
			props.diffContext.positionMap,
			comments,
		)

		reinjectDeletedContentNodeComments(
			props.diffContext.diffEditor,
			props.diffContext.positionMap,
			comments,
		)
	},
)

watchDeep(loadedCommentThread, () => {
	updateCanScrollToBottom()
})

function removeEditorListeners() {
	// the editor view may already be destroyed when the diff editor
	// unmounts — in that case listeners are already cleaned up
	try {
		if (!isDiffMode.value) {
			activeEditor.value.off("transaction", handleRemoteDocChange)
		}

		activeEditor.value.view.dom.removeEventListener("click", handleEditorClick)
	} catch {
		// editor already destroyed, listeners are gone
	}

	window.removeEventListener("scroll", handleScrollOrResize, true)
	window.removeEventListener("resize", handleScrollOrResize)
}

// handles clicks on the editor to open/close text comment popovers.
function handleEditorClick() {
	const { from } = activeEditor.value.state.selection

	void selectComment({
		textComment: true,
		pos: from,
		closeIfOutside: true,
	})
}

/**
 * Handle remote document changes that may delete the currently viewed comment.
 */
function handleRemoteDocChange({ transaction }: { transaction: Transaction }) {
	// bail early if no comment is open
	if (!selectedComment.value) {
		return
	}

	// bail if doc didn't change
	if (!transaction.docChanged) {
		return
	}

	// bail if this is a local change (user's own edit)
	if (!isChangeOrigin(transaction)) {
		return
	}

	// check if the comment's DOM element still exists (fast querySelector)
	const id = selectedComment.value.id
	const editorDom = activeEditor.value.view.dom

	if (selectedComment.value.textComment) {
		if (!editorDom.querySelector(`[data-comment-id="${id}"]`)) {
			closePopover()
		}
	} else if (selectedComment.value.nodeComment) {
		if (!editorDom.querySelector(`[data-node-comment-id="${id}"]`)) {
			closePopover()
		}
	}
}

function handleScrollOrResize() {
	void selectComment("refresh")
}

function isCommentPopoverOpen(targetId?: string) {
	return (
		!!selectedComment.value &&
		selectedComment.value.id === targetId &&
		!!anchorElem.value
	)
}

async function selectComment(
	pos:
		| {
				textComment: true
				pos?: number
				id?: string
				closeIfOutside?: boolean
		  }
		| {
				textComment: false
				pos?: number
				id?: string
				clickUpdate?: boolean
		  }
		| "refresh",
) {
	if (pos !== "refresh") {
		// Save the current active editor selection before opening popover
		const { from, to } = activeEditor.value.state.selection
		savedContentEditorSelection.value = { from, to }

		if (pos.textComment) {
			let found: {
				from: number
				to: number
				attrs: CommentAttrs
			} | null = null

			if (pos.id) {
				found = findCommentMarkById(activeEditor.value.state, pos.id)
			} else if (pos.pos !== undefined) {
				found = findCommentMarkAtPos(activeEditor.value.state, pos.pos)
			}

			if (!found) {
				// we want to allow re-opening after going away from the
				// node comments
				cleanupRecentlyClosedNodeCommentIds()

				if (selectedComment.value?.textComment && pos.closeIfOutside) {
					closePopover(true)
				}

				return
			} else if (selectedComment.value?.id === found.attrs.commentId) {
				return
			} else if (
				isPendingCommentId(found.attrs.commentId) &&
				userData.value.id &&
				!pendingCommentBelongsToActiveUser(
					found.attrs.commentId,
					userData.value.id,
				)
			) {
				// pending comment belongs to another user, don't open it
				return
			}

			selectedComment.value = {
				id: found.attrs.commentId,
				textComment: found,
			}

			await nextTick()
		} else {
			let found: NodeCommentMatch | null = null

			if (pos.pos !== undefined) {
				found = findNodeCommentAtPos(activeEditor.value.state, pos.pos)
			} else if (pos.id) {
				found = findNodeCommentById(activeEditor.value.state, pos.id)
			}

			if (!found || selectedComment.value?.id === found.nodeCommentId) {
				return
			} else if (
				pos.clickUpdate &&
				recentlyClosedNodeCommentIds.value.includes(found.nodeCommentId)
			) {
				return
			} else if (
				isPendingCommentId(found.nodeCommentId) &&
				userData.value.id &&
				!pendingCommentBelongsToActiveUser(
					found.nodeCommentId,
					userData.value.id,
				)
			) {
				// pending comment belongs to another user, don't open it
				return
			}

			selectedComment.value = {
				id: found.nodeCommentId,
				nodeComment: found,
			}
			recentlyClosedNodeCommentIds.value =
				recentlyClosedNodeCommentIds.value.filter(
					(item) => item !== found.nodeCommentId,
				)
			cleanupRecentlyClosedNodeCommentIds()

			await nextTick()
		}
	}

	if (selectedComment.value?.textComment) {
		const { from, to } = selectedComment.value.textComment

		// close the popover if the comment mark's DOM element is hidden
		// (e.g. inside a collapsed mermaid block).
		const commentEl = activeEditor.value.view.dom.querySelector(
			`[data-comment-id="${selectedComment.value.id}"]`,
		)

		if (commentEl?.getClientRects().length === 0) {
			closePopover(true)
			return
		}

		const rect = posToDOMRect(activeEditor.value.view, from, to)

		anchorElem.value = {
			getBoundingClientRect: () => rect,
		} as Element

		await updatePopoverPosition()

		return
	} else if (selectedComment.value?.nodeComment) {
		const { pos } = selectedComment.value.nodeComment
		const nodeDOM = activeEditor.value.view.nodeDOM(pos)

		if (!nodeDOM || !(nodeDOM instanceof HTMLElement)) {
			return
		}

		const rect = nodeDOM.getBoundingClientRect()

		anchorElem.value = {
			getBoundingClientRect: () => rect,
		} as Element

		await updatePopoverPosition()

		return
	}
}

// pos values:
// - number: node position
// - 'text-selection': text comment at selection position
async function addNewComment(pos: number | "text-selection") {
	if (!userData.value.id) {
		return
	}

	const pendingId = createPendingCommentId(userData.value.id)
	activePendingId.value = pendingId

	if (pos === "text-selection") {
		if (isDiffMode.value) {
			await addNewDiffTextComment(pendingId)
		} else {
			const { from } = activeEditor.value.state.selection
			const res = activeEditor.value.commands.addCommentMark({
				commentId: pendingId,
			})
			if (res) {
				// collapse ProseMirror selection and clear the native
				// browser selection to prevent visual snap to mark
				// boundaries
				activeEditor.value.commands.setTextSelection(from)
				window.getSelection()?.removeAllRanges()

				await selectComment({
					textComment: true,
					pos: from,
				})
			}
		}

		return
	}

	if (isDiffMode.value) {
		await addNewDiffNodeComment(pos, pendingId)
	} else {
		const res = activeEditor.value.commands.addNodeComment(pos, {
			nodeCommentId: pendingId,
		})
		if (res) {
			await selectComment({ textComment: false, pos: pos })
		}
	}
}

function updateNewSelectedCommentId(serverComment: DocumentComment) {
	if (!activePendingId.value) {
		return
	}

	const pendingId = activePendingId.value
	activePendingId.value = null
	const newId = serverComment.id

	// patch the optimistic cache entry with the real server data so
	// that loadedComment stays populated during the ID transition.
	// Without this, the computed finds no match (optimistic entry has
	// a nanoid ID, not the server ID) and the thread briefly renders
	// empty.
	if (editorStore.activeDocumentId && editorStore.activeBranchId) {
		commentAPI.patchOptimisticCommentEntry(
			editorStore.activeDocumentId,
			editorStore.activeBranchId,
			serverComment,
		)
	}

	if (selectedComment.value?.textComment) {
		// pre-set the forced highlight for the new ID so there is no
		// visual gap when the mark DOM element is recreated
		activeEditor.value.commands.setCommentMarkForcedHighlight(newId, true)

		if (props.diffContext) {
			updateCommentMarkIdInEditor(
				props.diffContext.diffEditor,
				pendingId,
				newId,
				true,
			)

			const hasExistingContent =
				!isRemovedDiffPosition(selectedComment.value.textComment.from) ||
				!isRemovedDiffPosition(selectedComment.value.textComment.to)

			if (hasExistingContent) {
				props.diffContext.suppressNextRecompute()
				props.contentEditor.commands.updateCommentMarkId(pendingId, newId)
			}
		} else {
			activeEditor.value.commands.updateCommentMarkId(pendingId, newId)
		}

		// swap the ID in-place so the popover and DynamicScroller
		// (keyed by selectedComment.id) stay mounted. The watcher on
		// selectedComment.id will transfer the forced highlight and
		// clean up the pending one.
		selectedComment.value = {
			...selectedComment.value,
			id: newId,
			textComment: {
				...selectedComment.value.textComment,
				attrs: {
					...selectedComment.value.textComment.attrs,
					commentId: newId,
				},
			},
		}

		return
	} else if (selectedComment.value?.nodeComment) {
		// pre-set the forced highlight for the new ID
		activeEditor.value.commands.setNodeCommentForcedHighlight(newId, true)

		if (props.diffContext) {
			updateNodeCommentIdInEditor(
				props.diffContext.diffEditor,
				pendingId,
				newId,
				true,
			)

			if (!isRemovedDiffPosition(selectedComment.value.nodeComment.pos)) {
				props.diffContext.suppressNextRecompute()
				props.contentEditor.commands.updateNodeCommentId(pendingId, newId)
			}
		} else {
			activeEditor.value.commands.updateNodeCommentId(pendingId, newId)
		}

		// swap the ID in-place
		selectedComment.value = {
			...selectedComment.value,
			id: newId,
			nodeComment: {
				...selectedComment.value.nodeComment,
				nodeCommentId: newId,
			},
		}

		return
	}
}

// recompute the diff deletion context from the current editor state
// and position map.
function computeDiffDeletionContext():
	| DocumentCommentDiffDeletionContext
	| undefined {
	if (!isDiffMode.value || !selectedComment.value || !props.diffContext) {
		return undefined
	}

	if (selectedComment.value.nodeComment) {
		const pos = selectedComment.value.nodeComment.pos
		if (isRemovedDiffPosition(pos)) {
			return {}
		}
	} else if (selectedComment.value.textComment) {
		const { from, to } = selectedComment.value.textComment

		const diffState = props.diffContext.diffEditor.state

		const segments = segmentSelectionByDiffStatus(
			props.diffContext.positionMap,
			from,
			to,
			diffState.doc,
			{
				removed: diffState.schema.marks[DIFF_TEXT_REMOVED_MARK_NAME],
				added: diffState.schema.marks[DIFF_TEXT_ADDED_MARK_NAME],
			},
		)
		const removedSegments = segments.filter(
			(s) => s.status === DiffStatus.Removed,
		)

		if (removedSegments.length > 0) {
			const diffDoc = diffState.doc

			return {
				textAnchors: removedSegments.flatMap((seg) => {
					// resolve the actual node that carries the diff status.
					// for top-level nodes this matches the position map entry;
					// for nested nodes (inside unwrapped/transparent parents)
					// this is the real textblock, not the top-level wrapper.
					const $from = diffDoc.resolve(seg.from)
					const $to = diffDoc.resolve(seg.to)

					let diffNode: PMNode | null = null
					let diffNodeStart = 0

					for (let d = $from.depth; d >= 1; d--) {
						const n = $from.node(d)
						if (n.attrs.diffStatus) {
							diffNode = n
							diffNodeStart = $from.before(d)
							break
						}
					}

					if (!diffNode) {
						return []
					}

					const uid = diffNode.attrs.uid as string | undefined
					if (!uid) {
						return []
					}

					const rawFrom = seg.from - diffNodeStart
					const rawTo = $to.pos - diffNodeStart

					if (diffNode.attrs.diffStatus === DiffStatus.Modified) {
						// inline-expanded node: convert merged-doc offsets
						// to original-text offsets for stability across
						// diff recomputes.
						// snap flags are set when there is a non-removed
						// segment on the corresponding side — those carry
						// source-editor marks that the reinjected textAnchor
						// mark needs to stay continuous with.
						const snapFrom = segments.some(
							(s) =>
								s.nodeUid === seg.nodeUid &&
								s.status !== DiffStatus.Removed &&
								s.to <= seg.from,
						)
						const snapTo = segments.some(
							(s) =>
								s.nodeUid === seg.nodeUid &&
								s.status !== DiffStatus.Removed &&
								s.from >= seg.to,
						)

						return [
							{
								nodeUid: uid,
								fromOffset: mergedOffsetToOriginalOffset(diffNode, rawFrom),
								toOffset: mergedOffsetToOriginalOffset(diffNode, rawTo),
								...(snapFrom ? { snapFrom: true } : {}),
								...(snapTo ? { snapTo: true } : {}),
							},
						]
					}

					return [
						{
							nodeUid: uid,
							fromOffset: rawFrom,
							toOffset: rawTo,
						},
					]
				}),
			}
		}
	}

	return undefined
}

async function saveComment() {
	if (!editorStore.activeDocumentId || !editorStore.activeBranchId) {
		return
	}

	const content = editor.value?.getJSON()
	if (!content || editTarget.value?.reply) {
		return
	}

	const anchorBlockId =
		getCommentAnchorBlockId() || loadedComment.value?.anchorBlockId || ""

	if (editTarget.value) {
		try {
			await commentAPI.updateDocumentCommentByCommentId.mutateAsync({
				docId: editorStore.activeDocumentId,
				branchId: editorStore.activeBranchId,
				commentId: editTarget.value.id,
				req: {
					content: content,
					anchorBlockID: anchorBlockId,
					branchId: editorStore.activeBranchId,
				},
			})
		} catch {
			showToastMessage("error", t("editor.comment-thread.errors.update-failed"))
			return
		}
	} else {
		try {
			const diffDeletionContext = computeDiffDeletionContext()

			const req: DocumentCommentCreateRequest = {
				content: content,
				anchorBlockID: anchorBlockId,
				branchId: editorStore.activeBranchId,
				...(diffDeletionContext ? { diffDeletionContext } : {}),
			}

			if (isDiffMode.value) {
				savingDiffComment = true
			}

			const res = await commentAPI.createDocumentCommentByDocId.mutateAsync({
				docId: editorStore.activeDocumentId,
				req,
			})
			if (res) {
				updateNewSelectedCommentId(res)
			}
		} catch {
			showToastMessage("error", t("editor.comment-thread.errors.create-failed"))
			return
		} finally {
			savingDiffComment = false
		}
	}

	resetEditor()
}

async function deleteSelectedComment() {
	if (
		!selectedComment.value ||
		!loadedComment.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	const commentId = loadedComment.value.id
	const comment = loadedComment.value

	if (!comment.replies?.length) {
		closePopover()
	}

	try {
		await commentAPI.deleteDocumentCommentByCommentId.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			commentId: commentId,
		})
	} catch {
		showToastMessage("error", t("editor.comment-thread.errors.delete-failed"))
		return
	}
}

async function resolveComment() {
	if (
		!selectedComment.value ||
		!loadedComment.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	const commentId = loadedComment.value.id
	const isTextComment = !!selectedComment.value.textComment
	closePopover()

	try {
		await commentAPI.updateDocumentCommentResolveByCommentId.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			commentId: commentId,
		})
	} catch {
		showToastMessage("error", t("editor.comment-thread.errors.delete-failed"))
		return
	}

	if (isDiffMode.value && props.diffContext?.diffEditor) {
		deleteCommentFromEditor(
			props.diffContext.diffEditor,
			commentId,
			isTextComment,
			true,
		)
	}

	deleteCommentFromEditor(props.contentEditor, commentId, isTextComment)
	resetEditor()
}

async function saveReply() {
	if (!editorStore.activeDocumentId || !editorStore.activeBranchId) {
		return
	}

	const content = editor.value?.getJSON()
	if (
		!content ||
		(editTarget.value && !editTarget.value.reply) ||
		!selectedComment.value
	) {
		return
	}

	if (editTarget.value) {
		try {
			const mutationPromise =
				commentAPI.updateDocumentCommentReplyByReplyId.mutateAsync({
					docId: editorStore.activeDocumentId,
					branchId: editorStore.activeBranchId,
					commentId: selectedComment.value.id,
					replyId: editTarget.value.id,
					req: { content: content },
				})
			// Reset immediately after optimistic update (onMutate runs synchronously)
			resetEditor()
			await mutationPromise
		} catch {
			showToastMessage(
				"error",
				t("editor.comment-thread.errors.reply-update-failed"),
			)
			return
		}
	} else {
		try {
			const mutationPromise =
				commentAPI.createDocumentCommentReplyByCommentId.mutateAsync({
					docId: editorStore.activeDocumentId,
					branchId: editorStore.activeBranchId,
					commentId: selectedComment.value.id,
					req: { content: content },
				})
			// Scroll and reset immediately after optimistic update (onMutate runs synchronously)
			scrollToBottom()
			resetEditor()
			await mutationPromise
		} catch {
			showToastMessage(
				"error",
				t("editor.comment-thread.errors.reply-create-failed"),
			)
			return
		}
	}
}

async function deleteReply(id: string) {
	if (
		!selectedComment.value ||
		!editorStore.activeDocumentId ||
		!editorStore.activeBranchId
	) {
		return
	}

	try {
		await commentAPI.deleteDocumentCommentReplyByReplyId.mutateAsync({
			docId: editorStore.activeDocumentId,
			branchId: editorStore.activeBranchId,
			commentId: selectedComment.value.id,
			replyId: id,
		})
	} catch {
		showToastMessage(
			"error",
			t("editor.comment-thread.errors.reply-delete-failed"),
		)
		return
	}
}

function closePopover(excludeRecentlyClosed = false) {
	activePendingId.value = null

	if (isDiffMode.value && props.diffContext?.diffEditor) {
		deletePendingCommentData(
			props.diffContext.diffEditor,
			userData.value.id || "",
			undefined,
			true,
		)
	}

	deletePendingCommentData(props.contentEditor, userData.value.id || "")

	if (!excludeRecentlyClosed && !isDiffMode.value) {
		if (selectedComment.value?.nodeComment) {
			recentlyClosedNodeCommentIds.value.push(selectedComment.value.id)
		}
	}

	selectedComment.value = null
	anchorElem.value = null

	// Restore focus and selection to active editor without scrolling
	if (savedContentEditorSelection.value) {
		const { from, to } = savedContentEditorSelection.value

		activeEditor.value
			.chain()
			.focus(undefined, { scrollIntoView: false })
			.setTextSelection({ from, to })
			.run()
		savedContentEditorSelection.value = null
	}
}

function cleanupRecentlyClosedNodeCommentIds() {
	if (recentlyClosedNodeCommentIds.value.length === 0) {
		return
	}

	const cursorPos = activeEditor.value.state.selection.from
	const state = activeEditor.value.state

	recentlyClosedNodeCommentIds.value =
		recentlyClosedNodeCommentIds.value.filter((nodeCommentId) => {
			const match = findNodeCommentById(state, nodeCommentId)
			if (!match) {
				return false
			}

			const { pos, node } = match
			const nodeEnd = pos + node.nodeSize
			// Keep the ID only if cursor is still inside the node
			return cursorPos >= pos && cursorPos < nodeEnd
		})
}

function resetEditor() {
	editor.value?.commands.setContent({
		type: "doc",
		content: [{ type: "paragraph" }],
	})
	editTarget.value = null
}

function scrollToBottom() {
	void nextTick(() => {
		scrollerElem.value?.scrollToBottom()
		// Call again after a short delay to account for dynamic height recalculation
		setTimeout(() => {
			scrollerElem.value?.scrollToBottom()
		}, 50)
	})
}

function handleScrollerScroll() {
	const el = scrollerElem.value?.$el as HTMLElement | undefined
	if (!el) {
		canScrollToBottom.value = false
		return
	}

	const threshold = 20
	const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
	canScrollToBottom.value = distanceFromBottom > threshold
}

function updateCanScrollToBottom() {
	void nextTick(() => {
		handleScrollerScroll()
	})
}

function getNodeUid(node?: PMNode | null) {
	const uid = node?.attrs.uid as string | null | undefined
	return uid !== null && uid !== undefined && uid !== "" ? uid : ""
}

function findNearestUidAtPos(
	state: EditorState,
	pos: number,
	node?: PMNode | null,
) {
	const nodeUid = getNodeUid(node)
	if (nodeUid) {
		return nodeUid
	}

	if (pos < 0 || pos > state.doc.content.size) {
		return ""
	}

	const $pos = state.doc.resolve(pos)
	for (let depth = $pos.depth; depth >= 0; depth--) {
		const uid = getNodeUid($pos.node(depth))
		if (uid) {
			return uid
		}
	}

	return ""
}

function getCommentAnchorBlockId() {
	// in diff mode, use the diff editor state (UIDs are preserved in merged doc)
	const state = activeEditor.value.state
	const selected = selectedComment.value

	if (selected?.nodeComment) {
		return findNearestUidAtPos(
			state,
			selected.nodeComment.pos,
			selected.nodeComment.node,
		)
	}

	if (selected?.textComment) {
		return findNearestUidAtPos(state, selected.textComment.from)
	}

	const fallbackPos =
		savedContentEditorSelection.value?.from ?? state.selection.from

	return findNearestUidAtPos(state, fallbackPos)
}

function startEdit(replyId?: string) {
	if (!editor.value || !selectedComment.value) {
		return
	}

	const content = replyId
		? loadedCommentReplies.value?.data[replyId]?.content
		: loadedComment.value?.content
	if (!content) {
		return
	}

	editor.value.commands.setContent(content)
	editTarget.value = {
		id: replyId || selectedComment.value.id,
		reply: !!replyId,
	}
}

async function updatePopoverPosition() {
	if (!popoverElem.value || !anchorElem.value || !selectedComment.value) {
		return
	}

	// Check if anchor is taller than 70% of viewport height
	const anchorRect = anchorElem.value.getBoundingClientRect()
	const viewportHeight = window.innerHeight
	const isLargeAnchor = anchorRect.height > viewportHeight * 0.7

	const { x, y, placement } = await computePosition(
		anchorElem.value,
		popoverElem.value,
		{
			placement: "bottom-start",
			strategy: "absolute",
			middleware: [
				// For large anchors, offset upward by anchor height to align top corners
				offset(
					isLargeAnchor
						? { mainAxis: -anchorRect.height, alignmentAxis: 7 }
						: { mainAxis: 5 },
				),
				flip({
					boundary: props.container ?? undefined,
					fallbackPlacements: ["bottom", "bottom-end"],
					padding: 8,
				}),
				shift({
					boundary: props.container ?? undefined,
					padding: 8,
				}),
			],
		},
	)
	popoverPosition.value = { left: x, top: y, placement: placement }
}

// --- diff mode helpers ---

// find the position of the node carrying the given uid in the source
// (content) editor document.
function findSourcePosByUid(uid: string): number | null {
	let result: number | null = null

	props.contentEditor.state.doc.descendants((node, pos) => {
		if (result !== null) {
			return false
		}

		if (node.attrs.uid === uid) {
			result = pos
			return false
		}
	})

	return result
}

/**
 * map a position from the diff editor to the source (content) editor.
 * uses uid-based matching to find the corresponding block, then applies
 * the same intra-block offset.
 */
function mapDiffPosToSourcePos(diffPos: number): number | null {
	if (!props.diffContext) {
		return null
	}

	const diffState = props.diffContext.diffEditor.state
	const sourceState = props.contentEditor.state

	if (diffPos < 0 || diffPos > diffState.doc.content.size) {
		return null
	}

	const $pos = diffState.doc.resolve(diffPos)

	// walk up the tree to find nearest node with a uid
	let nodeUid: string | null = null
	let nodeStartPos = 0

	for (let depth = $pos.depth; depth >= 0; depth--) {
		const node = $pos.node(depth)
		const uid = node.attrs.uid as string | undefined
		if (uid) {
			nodeUid = uid
			nodeStartPos = depth === 0 ? 0 : $pos.before(depth)
			break
		}
	}

	if (!nodeUid) {
		return null
	}

	const offsetInNode = diffPos - nodeStartPos

	// subtract the length of any diffTextRemoved marked text that
	// appears before diffPos within this node. inline diff expansion
	// injects deleted text as real content, shifting positions relative
	// to the source document.
	const diffNode = diffState.doc.nodeAt(nodeStartPos)
	const removedMarkType = diffState.schema.marks[DIFF_TEXT_REMOVED_MARK_NAME]
	let adjustedOffset = offsetInNode

	if (diffNode && removedMarkType) {
		const offsetInContent = offsetInNode - 1
		let deletedBefore = 0

		diffNode.descendants((child, pos) => {
			if (!child.isText) {
				return true
			}

			if (!child.marks.some((m) => m.type === removedMarkType)) {
				return false
			}

			const childEnd = pos + child.nodeSize

			if (childEnd <= offsetInContent) {
				deletedBefore += child.nodeSize
			} else if (pos < offsetInContent) {
				deletedBefore += offsetInContent - pos
			}

			return false
		})

		adjustedOffset -= deletedBefore
	}

	// find the same uid in the source editor
	const sourceNodePos = findSourcePosByUid(nodeUid)

	if (sourceNodePos === null) {
		return null
	}

	const sourceNode = sourceState.doc.nodeAt(sourceNodePos)
	if (!sourceNode) {
		return null
	}

	const maxOffset = sourceNode.nodeSize
	const clampedOffset = Math.min(adjustedOffset, maxOffset)

	return sourceNodePos + clampedOffset
}

/** check whether a diff position falls inside a removed block. */
function isRemovedDiffPosition(diffPos: number): boolean {
	if (!props.diffContext) {
		return false
	}

	// fast path: check top-level position map entries first
	for (const entry of props.diffContext.positionMap) {
		const endPos = entry.startPos + entry.nodeSize
		if (diffPos >= entry.startPos && diffPos < endPos) {
			if (entry.diffStatus === DiffStatus.Removed) {
				return true
			}

			break
		}
	}

	// for nested nodes inside unwrapped/transparent parents, the
	// position map only tracks the top-level block. walk up the
	// resolved position to check whether any ancestor (or the
	// node itself) has diffStatus === removed.
	const doc = props.diffContext.diffEditor.state.doc
	if (diffPos < 0 || diffPos >= doc.content.size) {
		return false
	}

	const $pos = doc.resolve(diffPos)

	// check the node directly at this position (for node-comment
	// positions that point to the node's start)
	const nodeAt = doc.nodeAt(diffPos)
	if (nodeAt?.attrs.diffStatus === DiffStatus.Removed) {
		return true
	}

	// transparent wrapper nodes (e.g. titledCodeBlock) have no
	// diffStatus themselves — check if all children are removed.
	if (nodeAt && !nodeAt.attrs.diffStatus && nodeAt.childCount > 0) {
		let allRemoved = true
		for (let i = 0; i < nodeAt.childCount; i++) {
			if (nodeAt.child(i).attrs.diffStatus !== DiffStatus.Removed) {
				allRemoved = false
				break
			}
		}

		if (allRemoved) {
			return true
		}
	}

	// walk ancestors from deepest to shallowest
	for (let depth = $pos.depth; depth >= 1; depth--) {
		const ancestor = $pos.node(depth)
		if (ancestor.attrs.diffStatus === DiffStatus.Removed) {
			return true
		}
	}

	return false
}

/** add a text comment mark in the diff editor (and optionally source editor). */
async function addNewDiffTextComment(pendingCommentId: string) {
	if (!props.diffContext) {
		return
	}

	const diffState = props.diffContext.diffEditor.state
	const { from, to } = diffState.selection
	if (from === to) {
		return
	}

	const segments = segmentSelectionByDiffStatus(
		props.diffContext.positionMap,
		from,
		to,
		diffState.doc,
		{
			removed: diffState.schema.marks[DIFF_TEXT_REMOVED_MARK_NAME],
			added: diffState.schema.marks[DIFF_TEXT_ADDED_MARK_NAME],
		},
	)
	if (segments.length === 0) {
		return
	}

	const existingSegments = segments.filter(
		(s) => s.status !== DiffStatus.Removed,
	)

	// add comment mark to the diff editor for the full selection range
	const diffMarkType = diffState.schema.marks[COMMENT_MARK_NAME]
	if (!diffMarkType) {
		return
	}

	const diffMark = diffMarkType.create({ commentId: pendingCommentId })
	const diffTr = diffState.tr
	diffTr.addMark(from, to, diffMark)
	diffTr.setMeta(DIFF_COMMENT_TX_META, true)
	props.diffContext.diffEditor.view.dispatch(diffTr)

	// clear the selection to avoid it snapping to mark boundaries when
	// ProseMirror (native selection needs to be cleared too)
	window.getSelection()?.removeAllRanges()
	props.diffContext.diffEditor.commands.setTextSelection(from)

	// for existing/added segments, also add marks in the source editor
	if (existingSegments.length > 0) {
		const sourceMarkType =
			props.contentEditor.state.schema.marks[COMMENT_MARK_NAME]
		if (sourceMarkType) {
			props.diffContext.suppressNextRecompute()
			const sourceTr = props.contentEditor.state.tr
			for (const seg of existingSegments) {
				const srcFrom = mapDiffPosToSourcePos(seg.from)
				const srcTo = mapDiffPosToSourcePos(seg.to)
				if (srcFrom !== null && srcTo !== null && srcFrom < srcTo) {
					const sourceMark = sourceMarkType.create({
						commentId: pendingCommentId,
					})
					sourceTr.addMark(srcFrom, srcTo, sourceMark)
				}
			}
			props.contentEditor.view.dispatch(sourceTr)
		}
	}

	await selectComment({
		textComment: true,
		pos: from,
	})
}

/**
 * update a comment mark id in a specific editor, optionally using the
 * DIFF_COMMENT_TX_META to bypass the content lock.
 */
function updateCommentMarkIdInEditor(
	targetEditor: Editor,
	oldId: string,
	newId: string,
	useDiffMeta: boolean,
): boolean {
	const state = targetEditor.state
	const markType = state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return false
	}

	const { tr } = state
	let found = false

	state.doc.descendants((node, pos) => {
		if (!node.isText) {
			return
		}

		const targetMark = node.marks.find(
			(mark) => mark.type === markType && mark.attrs.commentId === oldId,
		)

		if (targetMark) {
			const nodeEnd = pos + node.nodeSize
			const newAttrs = { ...targetMark.attrs, commentId: newId }
			const newMark = markType.create(newAttrs)
			tr.removeMark(pos, nodeEnd, targetMark)
			tr.addMark(pos, nodeEnd, newMark)
			found = true
		}
	})

	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- found is set inside the synchronous descendants callback, which TS cannot see
	if (!found) {
		return false
	}

	if (useDiffMeta) {
		tr.setMeta(DIFF_COMMENT_TX_META, true)
	}

	targetEditor.view.dispatch(tr)

	return true
}

/** add a node comment in the diff editor (and optionally source editor). */
async function addNewDiffNodeComment(pos: number, pendingCommentId: string) {
	if (!props.diffContext) {
		return
	}

	const removed = isRemovedDiffPosition(pos)

	const node = props.diffContext.diffEditor.state.doc.nodeAt(pos)
	if (!node) {
		return
	}

	// add node comment to the diff editor
	const diffTr = props.diffContext.diffEditor.state.tr
	diffTr.setNodeMarkup(pos, undefined, {
		...node.attrs,
		nodeCommentId: pendingCommentId,
	})
	diffTr.setMeta(DIFF_COMMENT_TX_META, true)
	props.diffContext.diffEditor.view.dispatch(diffTr)

	if (!removed) {
		// find the matching node in the source editor by uid
		const uid = node.attrs.uid as string | undefined
		if (uid) {
			const sourcePos = findSourcePosByUid(uid)

			if (sourcePos !== null) {
				const sourceNode = props.contentEditor.state.doc.nodeAt(sourcePos)

				if (sourceNode) {
					props.diffContext.suppressNextRecompute()
					const sourceTr = props.contentEditor.state.tr
					sourceTr.setNodeMarkup(sourcePos, undefined, {
						...sourceNode.attrs,
						nodeCommentId: pendingCommentId,
					})
					props.contentEditor.view.dispatch(sourceTr)
				}
			}
		}
	}

	await selectComment({ textComment: false, pos: pos })
}

/**
 * update a node comment id in a specific editor, optionally using the
 * DIFF_COMMENT_TX_META to bypass the content lock.
 */
function updateNodeCommentIdInEditor(
	targetEditor: Editor,
	oldId: string,
	newId: string,
	useDiffMeta: boolean,
): boolean {
	const state = targetEditor.state
	const { tr } = state
	let found = false

	state.doc.descendants((node, pos) => {
		if (found) {
			return false
		}
		if (node.attrs.nodeCommentId === oldId) {
			tr.setNodeMarkup(pos, undefined, {
				...node.attrs,
				nodeCommentId: newId,
			})
			found = true
			return false
		}
	})

	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- found is set inside the synchronous descendants callback, which TS cannot see
	if (!found) {
		return false
	}

	if (useDiffMeta) {
		tr.setMeta(DIFF_COMMENT_TX_META, true)
	}

	targetEditor.view.dispatch(tr)

	return true
}
</script>
<template>
	<Transition v-bind="defaultTransitionProps">
		<div
			v-if="selectedComment && anchorElem"
			ref="comment-popover"
			class="absolute z-popover flex max-h-[50dvh] w-[min(35.5rem,calc(100%-1rem))] flex-col overflow-hidden rounded-lg border border-border bg-background shadow-md"
			:style="{
				top: `${popoverPosition.top + (selectedComment.nodeComment ? (popoverPosition.placement.includes('top') ? TOP_NODE_COMMENT_OFFSET_SIDE_TOP : TOP_NODE_COMMENT_OFFSET_SIDE_BOTTOM) : 0)}px`,
				left: `${popoverPosition.left + (selectedComment.nodeComment && popoverPosition.placement.includes('start') ? LEFT_NODE_COMMENT_OFFSET : 0)}px`,
			}"
		>
			<div
				v-if="loadedCommentThread.length"
				class="relative flex min-h-0 flex-1 flex-col"
			>
				<DynamicScroller
					:key="selectedComment?.id"
					ref="comment-scroller"
					:items="loadedCommentThread"
					:min-item-size="70"
					key-field="id"
					class="flex w-full flex-1 flex-col overflow-y-auto p-2"
					@scroll="handleScrollerScroll"
					@visible="handleScrollerScroll"
				>
					<template #default="{ item, index, active }">
						<DynamicScrollerItem
							:item="item"
							:active="active"
							:size-dependencies="[item.content, item.edited]"
							:data-index="index"
						>
							<div
								:class="
									cn('group/comment flex w-full flex-col rounded-md text-sm')
								"
							>
								<div class="flex items-center justify-between">
									<div class="flex flex-1 items-center gap-1.5">
										<ShadcnUiAvatar class="size-6 border">
											<ShadcnUiAvatarImage
												v-if="item.author.image"
												:src="item.author.image"
												:alt="$t('settings.workspace.logo-alt')"
											/>
											<ShadcnUiAvatarFallback class="rounded-md text-2xs">
												{{ extractInitials(item.author.name || "", 2) }}
											</ShadcnUiAvatarFallback>
										</ShadcnUiAvatar>
										<div class="text-2sm opacity-60">
											<i18n-t
												v-if="!item.edited"
												scope="global"
												keypath="editor.comment-thread.comment-header"
												tag="span"
											>
												<template #author>
													<span class="font-semibold">
														{{ item.author.name }}
													</span>
												</template>
												<template #time>
													{{
														new Date().getTime() - item.createdAt.getTime() <
														NOW_TIME_LABEL_THRESHOLD_MS
															? $t("editor.comment-thread.now-time-label")
															: formatDistanceToNowStrict(item.createdAt, {
																	addSuffix: true,
																	locale: convertDateFnsLocale(locale),
																})
													}}
												</template>
											</i18n-t>
											<i18n-t
												v-else
												scope="global"
												keypath="editor.comment-thread.edited-comment-header"
												tag="span"
											>
												<template #author>
													<span class="font-semibold">
														{{ item.author.name }}
													</span>
												</template>
												<template #time>
													{{
														new Date().getTime() - item.createdAt.getTime() <
														NOW_TIME_LABEL_THRESHOLD_MS
															? $t("editor.comment-thread.now-time-label")
															: formatDistanceToNowStrict(
																	new Date(item.createdAt),
																	{
																		addSuffix: true,
																		locale: convertDateFnsLocale(locale),
																	},
																)
													}}
												</template>
											</i18n-t>
										</div>
									</div>
									<div>
										<ShadcnUiDropdownMenu v-if="item.author.self">
											<ShadcnUiDropdownMenuTrigger as-child>
												<ShadcnUiButton
													variant="ghost-plain"
													size="icon-sm"
													:class="
														cn(
															!editTarget
																? ''
																: 'pointer-events-none invisible',
														)
													"
												>
													<Icon name="lucide:ellipsis" />
												</ShadcnUiButton>
											</ShadcnUiDropdownMenuTrigger>
											<ShadcnUiDropdownMenuContent
												side="bottom"
												align="start"
												loop
											>
												<ShadcnUiDropdownMenuItem
													@click="startEdit(item.reply ? item.id : undefined)"
												>
													<Icon name="lucide:pen-line" />
													<span>
														{{ $t("editor.comment-thread.edit-start-button") }}
													</span>
												</ShadcnUiDropdownMenuItem>
												<ShadcnUiDropdownMenuItem
													@click="
														item.reply
															? deleteReply(item.id)
															: deleteSelectedComment()
													"
												>
													<Icon name="lucide:trash-2" />
													<span>
														{{ $t("editor.comment-thread.delete-button") }}
													</span>
												</ShadcnUiDropdownMenuItem>
											</ShadcnUiDropdownMenuContent>
										</ShadcnUiDropdownMenu>
									</div>
								</div>
								<div class="ml-2.75 flex max-w-none min-w-0">
									<div class="flex min-w-0 flex-1 gap-3.5">
										<div class="w-0.5 bg-border" />
										<Comment
											:content="item.content"
											:class="
												cn(
													'mt-1 mb-2 min-w-0 flex-1 rounded-md border border-transparent p-1 transition-colors duration-150',
													editTarget?.id === item.id
														? 'border-border'
														: undefined,
												)
											"
										/>
									</div>
								</div>
							</div>
						</DynamicScrollerItem>
					</template>
				</DynamicScroller>
				<Transition v-bind="defaultTransitionProps">
					<ShadcnUiButton
						v-if="canScrollToBottom"
						variant="secondary"
						size="icon-sm"
						class="absolute right-2 bottom-2 shadow-md"
						@click="scrollToBottom"
					>
						<Icon name="lucide:arrow-down" />
					</ShadcnUiButton>
				</Transition>
			</div>
			<div
				:data-has-comments="!!loadedCommentThread.length"
				:class="
					cn(
						'flex shrink-0 items-center justify-between border-b bg-muted px-2 py-1 text-foreground',
						'data-[has-comments=true]:border-t',
					)
				"
			>
				<i18n-t
					v-if="editTarget"
					scope="global"
					keypath="editor.comment-thread.title-edit-existing-thread"
					class="text-xs font-semibold"
					tag="div"
				/>
				<i18n-t
					v-else-if="loadedCommentThread.length"
					scope="global"
					keypath="editor.comment-thread.title-reply-existing-thread"
					class="text-xs font-semibold"
					tag="div"
				/>
				<i18n-t
					v-else
					scope="global"
					keypath="editor.comment-thread.title-new-thread"
					class="text-xs font-semibold"
					tag="div"
				/>
				<div class="flex items-center justify-end">
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Bold.name)"
						@click="editor?.chain().focus().toggleBold().run()"
					>
						<Icon name="lucide:bold" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Italic.name)"
						@click="editor?.chain().focus().toggleItalic().run()"
					>
						<Icon name="lucide:italic" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Underline.name)"
						@click="editor?.chain().focus().toggleUnderline().run()"
					>
						<Icon name="lucide:underline" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Strike.name)"
						@click="editor?.chain().focus().toggleStrike().run()"
					>
						<Icon name="lucide:strikethrough" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Blockquote.name)"
						@click="editor?.chain().focus().toggleBlockquote().run()"
					>
						<Icon name="lucide:quote" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(Code.name)"
						@click="editor?.chain().focus().toggleCode().run()"
					>
						<Icon name="lucide:square-code" />
					</ShadcnUiButton>
					<ShadcnUiButton
						size="icon-sm"
						variant="ghost"
						:active="editor?.isActive(CODE_BLOCK_NAME)"
						@click="
							() => {
								if (!editor) {
									return
								}

								const { from, to } = editor.state.selection
								const selectedText = editor.state.doc.textBetween(
									from,
									to,
									'\n',
								)

								editor
									.chain()
									.focus()
									.insertContent({
										type: CODE_BLOCK_NAME,
										content: selectedText
											? [{ type: 'text', text: selectedText }]
											: [],
									})
									.run()
							}
						"
					>
						<Icon name="lucide:code" />
					</ShadcnUiButton>
				</div>
			</div>
			<div ref="editor-container" class="flex w-full flex-col gap-2 p-2">
				<template v-if="editor">
					<LinkBubbleMenu
						ref="link-bubble-menu"
						:editor="editor"
						:container="containerElem"
					/>
				</template>
				<EditorContent :editor="editor" />
				<div class="flex items-center justify-between gap-2">
					<div class="flex items-center gap-2">
						<template v-if="editTarget">
							<ShadcnUiButton
								:disabled="editor?.isEmpty"
								size="2sm"
								@click.stop="editTarget.reply ? saveReply() : saveComment()"
							>
								{{ $t("editor.comment-thread.edit-button") }}
							</ShadcnUiButton>
							<ShadcnUiButton
								variant="secondary"
								size="2sm"
								@click.stop="resetEditor"
							>
								{{ $t("editor.comment-thread.cancel-button") }}
							</ShadcnUiButton>
						</template>
						<template v-else-if="loadedCommentThread.length">
							<ShadcnUiButton
								:disabled="editor?.isEmpty"
								size="2sm"
								@click.stop="saveReply"
							>
								{{ $t("editor.comment-thread.reply-button") }}
							</ShadcnUiButton>
							<ShadcnUiButton
								variant="secondary"
								size="2sm"
								@click="closePopover()"
							>
								{{ $t("editor.comment-thread.close-button") }}
							</ShadcnUiButton>
						</template>
						<template v-else>
							<ShadcnUiButton
								:disabled="editor?.isEmpty"
								size="2sm"
								@click.stop="saveComment"
							>
								{{ $t("editor.comment-thread.comment-button") }}
							</ShadcnUiButton>
							<ShadcnUiButton
								variant="secondary"
								size="2sm"
								@click="closePopover()"
							>
								{{ $t("editor.comment-thread.close-button") }}
							</ShadcnUiButton>
						</template>
					</div>
					<div v-if="loadedComment" class="flex items-center">
						<ShadcnUiButton size="2sm" @click.stop="resolveComment">
							{{ $t("editor.comment-thread.resolve-button") }}
						</ShadcnUiButton>
					</div>
				</div>
			</div>
		</div>
	</Transition>
</template>
