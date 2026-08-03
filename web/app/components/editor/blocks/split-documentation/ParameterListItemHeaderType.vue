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

const headerTypeContentClass = cn(
	placeholderEmptyNodeClass,
	"truncate font-normal text-xs! overflow-visible text-muted-foreground/80 caret-text-muted-foreground/80 placeholder:text-muted-foreground/60 min-w-20",
)

const contentProps = computed(() => {
	return {
		class: headerTypeContentClass,
		"data-placeholder": !props.node.textContent.length
			? explicitContentPlaceholder(t, props.node.type.name, isEditingDisabled)
			: undefined,
	}
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		class="before:hidden!"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<NodeViewContent as="p" v-bind="contentProps" />
	</NodeViewWrapper>
</template>
