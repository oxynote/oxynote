<script lang="ts" setup>
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
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

function addNeighborBlock(
	side: "above" | "below",
	blockType: "code" | "metrics",
) {
	if (props.hovered == null) {
		return
	}

	const chain = props.editor.chain()
	if (blockType === "metrics") {
		chain.setMeta(SUPPRESS_SCROLL_TO_SELECTION_META, true)
	}

	chain
		.focus()
		.insertBlockOnRightSide(props.hovered.nodePos, side, blockType)
		.run()
}
</script>
<template>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('above', 'code')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-start" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-right-side.add-code-block-above-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('below', 'code')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-end" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-right-side.add-code-block-below-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('above', 'metrics')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-start" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-right-side.add-metrics-above-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('below', 'metrics')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-end" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-right-side.add-metrics-below-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
</template>
