<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"

const props = defineProps(nodeViewProps)
const { isEditableAndUnlocked } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditableAndUnlocked.value || editorStore.reviewableDiffActive
})

function addElement(btn: "first" | "second") {
	const pos = props.getPos()
	if (!pos) {
		return
	}

	if (btn === "first") {
		props.editor.chain().focus().appendBlockOnRightSide(pos, "code").run()
	} else {
		props.editor
			.chain()
			.setMeta(SUPPRESS_SCROLL_TO_SELECTION_META, true)
			.focus()
			.appendBlockOnRightSide(pos, "metrics")
			.run()
	}
}
</script>
<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		:class="
			cn(
				'sticky top-[calc(var(--document-header-height)+theme(spacing.7))] h-fit min-w-0 flex-1',
				'drag-handle-ignore-self',
			)
		"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<NodeViewContent
			:class="
				cn('flex flex-col gap-4 [&>*]:last:mb-0', 'drag-handle-ignore-self')
			"
		/>
		<div v-show="!isEditingDisabled">
			<EditorBlocksSplitDocumentationBottomAction
				:button-text="
					$t(
						'editor.split-documentation.right-side-bottom-action-buttons.add-code',
					)
				"
				button-icon="lucide:code"
				:button-shortcut="SHORTCUT_ACTIONS.addCodeBlockToSplitDocRightSide"
				:second-button-text="
					$t(
						'editor.split-documentation.right-side-bottom-action-buttons.add-metrics',
					)
				"
				second-button-icon="lucide:chart-line"
				:second-button-shortcut="SHORTCUT_ACTIONS.addMetricsToSplitDocRightSide"
				@button-click="addElement"
			/>
		</div>
	</NodeViewWrapper>
</template>
