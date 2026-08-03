<script lang="ts" setup>
import type { NodeCommentOverlayState } from "./node-comment-extension"
import type { TextCommentIndicatorState } from "./comment-mark"
import CommentIndicator from "./CommentIndicator.vue"

const props = defineProps<{
	nodeCommentState: NodeCommentOverlayState | null
	textCommentState: TextCommentIndicatorState | null
}>()
const emit = defineEmits<{
	(e: "open-comment", type: "node" | "text", commentId: string): void
	(
		e: "comment-hover-change",
		type: "node" | "text",
		commentId: string,
		hovered: boolean,
	): void
}>()
</script>
<template>
	<Teleport
		v-if="props.nodeCommentState?.container"
		:to="props.nodeCommentState.container"
	>
		<CommentIndicator
			v-for="overlay in props.nodeCommentState.overlays"
			:key="'node-' + overlay.nodeCommentId"
			comment-type="node"
			:top="overlay.top"
			:left="overlay.left"
			:forced-highlight="overlay.forcedHighlight"
			:hovered="
				props.nodeCommentState.hoveredNodeCommentId === overlay.nodeCommentId
			"
			@click="emit('open-comment', 'node', overlay.nodeCommentId)"
			@mouseenter="
				emit('comment-hover-change', 'node', overlay.nodeCommentId, true)
			"
			@mouseleave="
				emit('comment-hover-change', 'node', overlay.nodeCommentId, false)
			"
		/>
	</Teleport>
	<Teleport
		v-if="props.textCommentState?.container"
		:to="props.textCommentState.container"
	>
		<CommentIndicator
			v-for="indicator in props.textCommentState.indicators"
			:key="'text-' + indicator.commentId"
			comment-type="text"
			:top="indicator.top"
			:left="indicator.left"
			:forced-highlight="indicator.forcedHighlight"
			:hovered="props.textCommentState.hoveredCommentId === indicator.commentId"
			@click="emit('open-comment', 'text', indicator.commentId)"
			@mouseenter="
				emit('comment-hover-change', 'text', indicator.commentId, true)
			"
			@mouseleave="
				emit('comment-hover-change', 'text', indicator.commentId, false)
			"
		/>
	</Teleport>
</template>
