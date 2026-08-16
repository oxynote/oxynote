<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper, NodeViewContent } from "@tiptap/vue-3"
import type { JSONContent } from "@tiptap/core"
import IconPicker from "../../IconPicker.vue"

const props = defineProps(nodeViewProps)

const icon = computed(() => {
	return props.node.attrs.icon as string
})

const editorStore = useEditorStore()
const isDiffActive = computed(() => {
	return editorStore.reviewableDiffActive
})

const oldNode = computed<JSONContent | null>(() => {
	if (!isDiffActive.value) {
		return null
	}

	const raw = props.node.attrs.oldNode as string | JSONContent | null
	if (!raw) {
		return null
	}

	return typeof raw === "string" ? (JSON.parse(raw) as JSONContent) : raw
})

const isIconModified = computed(() => {
	if (!oldNode.value) {
		return false
	}

	return oldNode.value.attrs?.icon !== icon.value
})

function setIcon(newIcon: string) {
	props.updateAttributes({ icon: newIcon })
}
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		class="relative my-5 flex h-fit flex-1 flex-row rounded-lg bg-muted"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<div class="mt-4 ml-4 rounded-md transition-colors">
			<IconPicker
				:icon="icon"
				size="icon-xsm"
				:diff-mode="isDiffActive"
				:is-modified="isIconModified"
				@select="setIcon"
			/>
		</div>
		<NodeViewContent class="flex-1 pr-4 pl-1.5 text-sm text-foreground!" />
	</NodeViewWrapper>
</template>
