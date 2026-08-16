<script lang="ts" setup>
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"

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

function addNeighborBlock(side: "above" | "below") {
	if (props.hovered == null) {
		return
	}

	props.editor
		.chain()
		.focus()
		.insertParameterListOnLeftSide(props.hovered.nodePos, side)
		.run()
}
</script>
<template>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('above')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-start" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-parameter-list.add-above-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('below')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-end" class="scale-x-[-1] transform" />
			<span>
				{{
					$t(
						"editor.drag-handle.options.split-doc-parameter-list.add-below-block",
					)
				}}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
</template>
