<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import {
	explicitContentPlaceholder,
	placeholderEmptyNodeClass,
} from "../../placeholder"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

const headerContentClass = cn(
	placeholderEmptyNodeClass,
	"truncate min-h-6 font-semibold text-base! overflow-visible placeholder:text-muted-foreground/60",
)

const contentProps = computed(() => {
	return {
		class: headerContentClass,
		"data-placeholder": !props.node.textContent.length
			? explicitContentPlaceholder(t, props.node.type.name, isEditingDisabled)
			: undefined,
	}
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		class="mt-5.5"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<NodeViewContent class="mb-0" as="p" v-bind="contentProps" />
	</NodeViewWrapper>
</template>
