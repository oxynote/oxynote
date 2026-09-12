<script lang="ts" setup>
import { NodeViewContent, nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"
import type { RightSideBlockType } from "."
import type { ShortcutAction } from "~/utils/shortcuts"

const props = defineProps(nodeViewProps)
const { t } = useI18n({ useScope: "global" })
const { isEditableAndUnlocked } = useEditorMeta()
const editorStore = useEditorStore()

const buttons = computed(
	(): {
		id: RightSideBlockType
		text: string
		icon: string
		shortcut: ShortcutAction
	}[] => [
		{
			id: "code",
			text: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
			icon: "lucide:code",
			shortcut: SHORTCUT_ACTIONS.addCodeBlockToSplitDocRightSide,
		},
		{
			id: "metrics",
			text: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
			icon: "lucide:chart-line",
			shortcut: SHORTCUT_ACTIONS.addMetricsToSplitDocRightSide,
		},
		{
			id: "diagram",
			text: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-diagram",
			),
			icon: "lucide:network",
			shortcut: SHORTCUT_ACTIONS.addDiagramToSplitDocRightSide,
		},
	],
)

const isEditingDisabled = computed(() => {
	return !isEditableAndUnlocked.value || editorStore.reviewableDiffActive
})

function addElement(blockType: RightSideBlockType) {
	// a falsiness check would wrongly reject position 0 (document start)
	const pos = props.getPos()
	if (typeof pos !== "number" || pos < 0) {
		return
	}

	const chain = props.editor.chain()
	// metric blocks don’t have text content to select, so nothing to
	// scroll to
	if (blockType === "metrics") {
		chain.setMeta(SUPPRESS_SCROLL_TO_SELECTION_META, true)
	}

	chain.focus().appendBlockOnRightSide(pos, blockType).run()
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
				:buttons="buttons"
				@button-click="addElement"
			/>
		</div>
	</NodeViewWrapper>
</template>
