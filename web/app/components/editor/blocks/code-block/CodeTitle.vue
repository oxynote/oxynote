<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import {
	explicitContentPlaceholder,
	placeholderEmptyNodeClass,
} from "../../placeholder"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})

const titleContentClass = computed(() =>
	cn(
		placeholderEmptyNodeClass,
		"truncate min-h-4 font-medium overflow-visible placeholder:text-muted-foreground/60",
		editorStore.reviewableDiffActive ? "caret-transparent" : "caret-foreground",
		props.node.attrs.diffStatus !== DiffStatus.Added &&
			props.node.attrs.diffStatus !== DiffStatus.Removed &&
			"bg-muted-highlight",
	),
)

const contentProps = computed(() => {
	return {
		class: titleContentClass.value,
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
		:class="
			cn(
				'not-prose group/extended-code-block rounded-b-0 relative flex flex-col rounded-lg rounded-b-none border border-border',
				props.node.attrs.diffStatus !== DiffStatus.Added &&
					props.node.attrs.diffStatus !== DiffStatus.Removed &&
					'bg-muted-highlight',
				'px-4 py-2 before:hidden!',
			)
		"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<NodeViewContent v-bind="contentProps" />
	</NodeViewWrapper>
</template>
