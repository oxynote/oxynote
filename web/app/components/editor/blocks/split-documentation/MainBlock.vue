<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { computed } from "vue"

const props = defineProps(nodeViewProps)
const isInversed = computed(() => Boolean(props.node.attrs.inversed))
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
		class="split-documentation @container border-y border-border"
	>
		<NodeViewContent
			:class="[
				'flex w-full gap-7 py-7 break-all whitespace-normal @2xl:gap-10',
				isInversed
					? 'flex-col-reverse @2xl:flex-row-reverse'
					: 'flex-col @2xl:flex-row',
			]"
		/>
	</NodeViewWrapper>
</template>

<style global lang="css">
@reference "@/assets/css/main.css";

.split-documentation:has(+ .split-documentation),
.split-documentation:has(+ .pm-gap-wrapper + .split-documentation) {
	@apply mb-0 border-b-0;

	+ .split-documentation,
	+ .pm-gap-wrapper + .split-documentation {
		@apply mt-0;
	}
}
</style>
