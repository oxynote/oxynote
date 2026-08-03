<script lang="ts" setup>
import { EditorContent } from "@tiptap/vue-3"
import type { Editor } from "@tiptap/core"
import type * as Y from "yjs"
import { contentExtensionsWithIDs } from "../schema-extensions"
import TextBubbleMenu from "../TextBubbleMenu.vue"
import CommentRenderer from "../comments/CommentRenderer.vue"
import CommentIndicatorContainer from "../comments/CommentIndicatorContainer.vue"
import type { NodeCommentOverlayState } from "../comments/node-comment-extension"
import type { TextCommentIndicatorState } from "../comments/comment-mark"
import BlockHandle from "../drag-handle/BlockHandle.vue"

const props = defineProps<{
	targetBranchYdoc: Y.Doc
	activeBranchYdoc: Y.Doc
	contentEditor: Editor
}>()

const { t } = useI18n({ useScope: "global" })
const containerElem = useTemplateRef("diff-container")
const commentRendererElem = useTemplateRef("diff-comment-renderer")

const nodeCommentState = ref<NodeCommentOverlayState | null>(null)
const textCommentState = ref<TextCommentIndicatorState | null>(null)

const { editor, positionMap, destroy, suppressNextRecompute } = useDiffEditor(
	() => props.targetBranchYdoc,
	() => props.activeBranchYdoc,
	contentExtensionsWithIDs(t, true, {
		onIndicatorStateChange: (v) => {
			textCommentState.value = v
		},
	}),
	{
		onNodeCommentOverlayStateChange: (v) => {
			nodeCommentState.value = v
		},
		onNodeCommentClick: (id: string) => {
			commentRendererElem.value?.selectComment({
				textComment: false,
				id: id,
				clickUpdate: true,
			})
		},
	},
)

onBeforeUnmount(() => {
	destroy()
})
</script>
<template>
	<div
		ref="diff-container"
		class="diff-editor editor-not-editable relative z-editor-content w-full min-w-0 flex-1 bg-transparent"
	>
		<template v-if="editor">
			<TextBubbleMenu
				:editor="editor"
				:container="containerElem"
				:diff-context="{ positionMap }"
				@add-thread="commentRendererElem?.addNewComment('text-selection')"
			/>
			<BlockHandle
				:editor="editor"
				@add-node-comment="
					(pos: number) => commentRendererElem?.addNewComment(pos)
				"
				@open-node-comment="
					(pos: number) =>
						commentRendererElem?.selectComment({
							textComment: false,
							pos: pos,
						})
				"
			/>
			<CommentRenderer
				ref="diff-comment-renderer"
				:content-editor="props.contentEditor"
				:container="containerElem"
				:diff-context="{
					diffEditor: editor,
					positionMap,
					suppressNextRecompute,
				}"
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
							editor?.commands.setNodeCommentForcedHighlight(id, hovered)
						} else {
							editor?.commands.setCommentMarkForcedHighlight(id, hovered)
						}
					}
				"
			/>
			<EditorContent :editor="editor" />
		</template>
	</div>
</template>
