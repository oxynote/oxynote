<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"

const props = defineProps(nodeViewProps)
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		class="flex flex-col"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<div class="mb-2 flex flex-col gap-1">
			<NodeViewContent class="flex w-full items-center gap-1.5" />
		</div>
	</NodeViewWrapper>
</template>
