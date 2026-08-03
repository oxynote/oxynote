<script lang="ts" setup>
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import { SplitDocumentationLeftSide } from "../../blocks/split-documentation"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"

const props = defineProps<{
	editor: Editor
	hovered: {
		node: Node
		nodePos: number
		nodeId: string
		nodeHooks: DocumentHook[] | null
		nodeHookStatus: "stale" | "fresh" | null
	} | null
}>()

function addParameterList() {
	if (!props.editor || props.hovered == null) {
		return
	}

	const splitDocNode = props.hovered.node
	const splitDocNodePos = props.hovered.nodePos

	// Find the SplitDocumentationLeftSide child by type
	let leftSideNodePos: number | null = null
	splitDocNode.forEach((child, childOffset) => {
		if (child.type.name === SplitDocumentationLeftSide.name) {
			leftSideNodePos = splitDocNodePos + 1 + childOffset
		}
	})

	if (leftSideNodePos == null) {
		return
	}

	props.editor
		.chain()
		.focus()
		.appendParameterListOnLeftSide(leftSideNodePos)
		.run()
}

function invertSplitDocumentation() {
	if (!props.editor || props.hovered == null) {
		return
	}

	props.editor
		.chain()
		.setMeta(SUPPRESS_SCROLL_TO_SELECTION_META, true)
		.focus()
		.invertSplitDocumentation(props.hovered.nodePos)
		.run()
}
</script>
<template>
	<ShadcnUiDropdownMenuItem @click="addParameterList">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-plus" class="scale-x-[-1] transform" />
			<span>
				{{ $t("editor.drag-handle.options.split-doc.add-parameter-list") }}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="invertSplitDocumentation">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:arrow-left-right" />
			<span>
				{{ $t("editor.drag-handle.options.split-doc.invert-sides") }}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
</template>
