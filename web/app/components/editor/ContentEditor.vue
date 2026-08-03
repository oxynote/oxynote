<script lang="ts" setup>
import { useEditor, EditorContent } from "@tiptap/vue-3"
import { TrailingNode } from "@tiptap/extensions"
import Typography from "@tiptap/extension-typography"
import { cn } from "@/lib/utils"
import {
	SlashCommands,
	processSlashCommandTransaction,
} from "./slash/extension"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import Collaboration from "@tiptap/extension-collaboration"
import CollaborationCaret from "@tiptap/extension-collaboration-caret"
import type * as Y from "yjs"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import type { Editor } from "@tiptap/core"
import { TextSelection } from "@tiptap/pm/state"
import TextBubbleMenu from "./TextBubbleMenu.vue"
import LinkBubbleMenu from "./link/LinkBubbleMenu.vue"
import BlockHandle from "./drag-handle/BlockHandle.vue"
import {
	renderCollaborationCaret,
	setEditingNodeInAwareness,
	RemoteDeleteSelectionGuard,
	isEditingNodeInAwarenessOfType,
} from "./collaboration"
import { TextSelectClipboard, TextSelectShortcuts } from "./text-select"
import { ParagraphDeletionHandler, InputRules } from "./input-utils"
import { HookDecorator } from "./hooks/hook-decorator"
import CommentRenderer from "./comments/CommentRenderer.vue"
import { Drag } from "./drag-handle/drag"
import { GapDecorations } from "./drag-handle/gap-decorations"
import { showToastMessage } from "../toast"
import AutoJoiner from "./tiptap-utils/auto-joiner"
import {
	NodeComment,
	type NodeCommentOverlayState,
} from "./comments/node-comment-extension"
import type { TextCommentIndicatorState } from "./comments/comment-mark"
import CommentIndicatorContainer from "./comments/CommentIndicatorContainer.vue"
import { createImageFileHandler } from "./blocks/image"
import { createFigmaLinkHandler } from "./blocks/figma"
import { contentExtensionsWithIDs, editorProseClass } from "./schema-extensions"
import { METRIC_BLOCK_NAME } from "./blocks/node-names"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "./scroll-control"

const props = defineProps<{
	activeBranchProvider: HocuspocusProvider
	activeBranchYdoc: Y.Doc
	documentHooks: DocumentHook[]
	nameEditor: Editor | null | undefined
	userCaretDetails: { name: string; color: string }
}>()
const emit = defineEmits<{
	(e: "editor-ready", val: Editor): void
	(e: "open-settings", target: "github"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { browserType } = useDetectHost()

const containerElem = useTemplateRef("editor-container")
const commentRendererElem = useTemplateRef("comment-renderer")
const linkBubbleMenuElem = useTemplateRef("link-bubble-menu")
const editorStore = useEditorStore()
const suppressNextSelectionScroll = ref(false)
const { isEditable } = useEditorMeta()
const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

const textCommentState = ref<TextCommentIndicatorState | null>(null)
const editorExtensions = contentExtensionsWithIDs(t, isEditingDisabled, {
	onIndicatorStateChange: (v) => {
		textCommentState.value = v
	},
})
const contentEditor = useEditor({
	editorProps: {
		attributes: {
			class: editorProseClass,
			spellcheck: "false",
		},
		// Firefox has a bug where clicking on an empty trailing paragraph
		// when the editor is unfocused doesn't properly place the cursor.
		// This handler explicitly sets the selection based on click coordinates.
		handleClick: (view, pos, event) => {
			// Only apply fix for Firefox
			if (browserType.value !== HostBrowserType.Firefox) {
				return false
			}

			// Only handle left clicks
			if (event.button !== 0) {
				return false
			}

			const { doc, tr } = view.state
			const $pos = doc.resolve(pos)
			const node = $pos.parent

			// Check if we clicked on an empty textblock (paragraph, heading, etc.)
			if (node.isTextblock && node.content.size === 0) {
				// Explicitly set selection to this position
				const selection = TextSelection.create(doc, pos)
				view.dispatch(tr.setSelection(selection))
				return true
			}

			return false
		},
		handleScrollToSelection: () => {
			if (!suppressNextSelectionScroll.value) {
				return false
			}

			suppressNextSelectionScroll.value = false
			return true
		},
		handleDOMEvents: {
			mousedown: (view, event) => {
				// Firefox bug: clicking on empty space to the right of text in
				// the last paragraph while unfocused restores the previous selection
				// instead of placing the cursor at the clicked position.
				// This fix captures click coordinates and sets selection after focus.
				if (
					browserType.value !== HostBrowserType.Firefox ||
					view.hasFocus() ||
					event.button !== 0
				) {
					return false
				}

				const coords = view.posAtCoords({
					left: event.clientX,
					top: event.clientY,
				})

				if (!coords) {
					return false
				}

				suppressNextSelectionScroll.value = true

				// Set selection after focus restoration completes
				setTimeout(() => {
					const selection = TextSelection.create(view.state.doc, coords.pos)
					view.dispatch(view.state.tr.setSelection(selection))
				}, 0)

				return false
			},
		},
	},
	extensions: [
		Document,
		Text,
		...editorExtensions,
		NodeComment.configure({
			types: editorExtensions.map((v) => v.name),
			onOverlayStateChange: (v) => {
				nodeCommentState.value = v
			},
			onNodeCommentClick: (id: string) => {
				commentRendererElem.value?.selectComment({
					textComment: false,
					id: id,
					clickUpdate: true,
				})
			},
		}),
		HookDecorator.configure({
			attributeName: "uid",
			getHooks: () => props.documentHooks,
		}),
		Typography,
		Collaboration.configure({
			document: props.activeBranchYdoc,
			field: "content",
		}),
		CollaborationCaret.configure({
			provider: props.activeBranchProvider,
			user: props.userCaretDetails,
			render: renderCollaborationCaret,
		}),
		Drag.configure({
			onBeforeDrop: () => {
				editorStore.updateLastDragDropTimestamp()
			},
		}),
		GapDecorations,
		AutoJoiner,
		RemoteDeleteSelectionGuard,
		TrailingNode.configure({ notAfter: [Paragraph.name] }),
		SlashCommands.configure({
			decorationClass: cn(
				"font-normal relative z-1",
				"before:absolute before:content-[''] before:-top-[0.075em] before:-left-[0.25em] before:-right-[0.25em] before:-bottom-[0.075em] before:bg-[var(--tw-prose-pre-bg)] before:-z-1 before:rounded-sm before:border-1 before:border-border",
			),
		}),
		TextSelectClipboard,
		TextSelectShortcuts,
		InputRules,
		ParagraphDeletionHandler.configure({
			onDeleted: () => {
				props.nameEditor?.chain().focus("end").run()
			},
		}),
		createImageFileHandler({ documentId: editorStore.activeDocumentId }),
		createFigmaLinkHandler(),
	],
	onCreate: ({ editor }) => {
		emit("editor-ready", editor)
		editor.commands.refreshGapDecorations()
	},
	onTransaction: ({ editor, transaction }) => {
		if (transaction.getMeta(SUPPRESS_SCROLL_TO_SELECTION_META)) {
			suppressNextSelectionScroll.value = true
		}

		processSlashCommandTransaction(editor, transaction)
	},
	onContentError: ({ editor, disableCollaboration }) => {
		editor.setEditable(false)
		disableCollaboration()
		showToastMessage("error", t("editor.errors.content-load-failed"))
	},
	onSelectionUpdate: ({ editor }) => {
		// don't override awareness if metric config modal is open
		if (
			isEditingNodeInAwarenessOfType(
				props.activeBranchProvider,
				METRIC_BLOCK_NAME,
			) &&
			editorStore.activeMetricBlockConfig
		) {
			return
		}

		// broadcast which node the user is currently editing via awareness
		const { $anchor } = editor.state.selection
		for (let depth = $anchor.depth; depth > 0; depth--) {
			const node = $anchor.node(depth)
			const uid = node.attrs?.uid

			if (uid) {
				setEditingNodeInAwareness(props.activeBranchProvider, {
					uid,
					type: node.type.name,
				})
				return
			}
		}

		setEditingNodeInAwareness(props.activeBranchProvider, null)
	},
	onBlur: () => {
		// when switching browser tabs the editor blurs but the user still
		// has their cursor in the node. Only clear awareness if the
		// document still has focus (genuine in-page navigation).
		if (!document.hasFocus()) {
			return
		}

		if (
			isEditingNodeInAwarenessOfType(
				props.activeBranchProvider,
				METRIC_BLOCK_NAME,
			) &&
			editorStore.activeMetricBlockConfig
		) {
			// Don't clear awareness if metric config modal is open
			return
		}

		setEditingNodeInAwareness(props.activeBranchProvider, null)
	},
	onDestroy: () => {
		setEditingNodeInAwareness(props.activeBranchProvider, null)
	},
})
const nodeCommentState = ref<NodeCommentOverlayState | null>(null)

// when diff mode turns off the content editor becomes visible again.
// clear overlay state so indicators unmount, then force recalculation
// on next tick so they remount at correct positions without animating
// from stale (0,0) coordinates.
watch(
	() => editorStore.reviewableDiffActive,
	(active) => {
		if (!active && contentEditor.value) {
			nodeCommentState.value = null
			textCommentState.value = null

			nextTick(() => {
				contentEditor.value?.commands.refreshNodeCommentOverlays()
				contentEditor.value?.commands.refreshTextCommentIndicators()
			})
		}
	},
)

watchDeep(
	() => props.documentHooks,
	() => {
		contentEditor.value?.commands.refreshHookDecorations()
	},
)

watchImmediate(
	[
		() => editorStore.activeMetricBlockConfig,
		() => contentEditor.value?.isEditable,
	],
	(newV) => {
		if (!contentEditor.value || !newV[0] || !newV[1]) {
			setEditingNodeInAwareness(props.activeBranchProvider, null)
			return
		}

		setEditingNodeInAwareness(props.activeBranchProvider, {
			uid: newV[0],
			type: METRIC_BLOCK_NAME,
		})
	},
)

function editLink() {
	linkBubbleMenuElem.value?.editSelection()
}

function setNodeCommentIndicatorHoverChange(
	nodeCommentId: string,
	hovered: boolean,
) {
	if (
		!contentEditor.value ||
		commentRendererElem.value?.isCommentPopoverOpen(nodeCommentId)
	) {
		return
	}

	contentEditor.value.commands.setNodeCommentForcedHighlight(
		nodeCommentId,
		hovered,
	)
}

function setTextCommentIndicatorHoverChange(
	commentId: string,
	hovered: boolean,
) {
	if (
		!contentEditor.value ||
		commentRendererElem.value?.isCommentPopoverOpen(commentId)
	) {
		return
	}

	contentEditor.value.commands.setCommentMarkForcedHighlight(commentId, hovered)
}
</script>
<template>
	<div
		ref="editor-container"
		:class="[
			'group/content-editor content-editor relative z-editor-content w-full min-w-0 flex-1 bg-transparent',
			{ 'editor-not-editable': isEditingDisabled },
		]"
	>
		<template v-if="contentEditor">
			<TextBubbleMenu
				:editor="contentEditor"
				:container="containerElem"
				@add-thread="commentRendererElem?.addNewComment('text-selection')"
				@edit-link="editLink"
			/>
			<LinkBubbleMenu
				ref="link-bubble-menu"
				:editor="contentEditor"
				:container="containerElem"
			/>
			<BlockHandle
				:document-hooks="documentHooks"
				:editor="contentEditor"
				:data-sync-provider="activeBranchProvider"
				@add-node-comment="
					(pos: number) => {
						commentRendererElem?.addNewComment(pos)
					}
				"
				@open-node-comment="
					(pos: number) => {
						commentRendererElem?.selectComment({ textComment: false, pos: pos })
					}
				"
				@open-settings="(target: 'github') => emit('open-settings', target)"
			/>
			<CommentRenderer
				ref="comment-renderer"
				:container="containerElem"
				:content-editor="contentEditor"
			/>
			<CommentIndicatorContainer
				:node-comment-state="nodeCommentState"
				:text-comment-state="textCommentState"
				@open-comment="
					(type: 'node' | 'text', id: string) =>
						commentRendererElem?.selectComment({
							textComment: type === 'text',
							id: id,
						})
				"
				@comment-hover-change="
					(type: 'node' | 'text', id: string, hovered: boolean) => {
						if (type === 'node') {
							setNodeCommentIndicatorHoverChange(id, hovered)
						} else {
							setTextCommentIndicatorHoverChange(id, hovered)
						}
					}
				"
			/>
		</template>
		<EditorContent :editor="contentEditor" />
	</div>
</template>
