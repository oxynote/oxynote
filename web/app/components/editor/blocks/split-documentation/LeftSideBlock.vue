<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"

const props = defineProps(nodeViewProps)
const { isEditableAndUnlocked } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditableAndUnlocked.value || editorStore.reviewableDiffActive
})

function addElement() {
	const pos = props.getPos()
	if (!pos) {
		return
	}

	props.editor.chain().focus().appendParameterListOnLeftSide(pos).run()
}
</script>
<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		class="drag-handle-ignore-self relative h-fit min-w-0 flex-1"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<NodeViewContent
			class="drag-handle-ignore-self [&_li]:wrap-anywhere [&_p]:wrap-anywhere [&>*:has(+.pm-gap-wrapper:last-child)]:mb-0 [&>*:last-child]:mb-0"
		/>
		<div v-show="!isEditingDisabled">
			<EditorBlocksSplitDocumentationBottomAction
				:button-text="
					$t('editor.split-documentation.left-side-bottom-action-button')
				"
				button-icon="lucide:list-plus"
				:button-shortcut="SHORTCUT_ACTIONS.addParamsToSplitDocLeftSide"
				@button-click="addElement"
			/>
		</div>
	</NodeViewWrapper>
</template>
