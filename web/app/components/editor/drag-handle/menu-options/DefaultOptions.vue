<script lang="ts" setup>
import type { JSONContent } from "@tiptap/core"
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import { ListItem, TaskItem } from "@tiptap/extension-list"
import { METRIC_BLOCK_NAME } from "../../blocks/node-names"

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

	const pos = props.hovered.nodePos
	const node = props.editor.state.doc.nodeAt(pos)
	if (!node) {
		return
	}

	const { startPos, endPos } = resolveInsertTargetBounds(
		props.editor,
		pos,
		node,
	)
	const insertPos = side === "above" ? startPos : endPos
	const { content, selectionOffset } = buildNeighborInsertContent(node)

	// Use editor chain to properly handle the insert with all extensions
	props.editor
		.chain()
		.focus()
		.insertContentAt(insertPos, content)
		.setTextSelection(insertPos + selectionOffset)
		.run()
}

function buildNeighborInsertContent(node: Node): {
	content: JSONContent
	selectionOffset: number
} {
	if (node.type.name === ListItem.name) {
		return {
			content: {
				type: ListItem.name,
				content: [
					{
						type: "paragraph",
						content: [],
					},
				],
			},
			selectionOffset: 1,
		}
	}

	if (node.type.name === TaskItem.name) {
		return {
			content: {
				type: TaskItem.name,
				attrs: { checked: false },
				content: [
					{
						type: "paragraph",
						content: [],
					},
				],
			},
			selectionOffset: 1,
		}
	}

	return {
		content: {
			type: "paragraph",
			content: [{ type: "text", text: "/" }],
		},
		selectionOffset: 2,
	}
}

function resolveInsertTargetBounds(
	editor: Editor,
	nodePos: number,
	node: Node,
): { startPos: number; endPos: number } {
	// Metric blocks are wrapped by metricGrid at root level; when inserting a
	// neighbor, target the wrapper bounds to add above/below the whole grid.
	if (node.type.name === METRIC_BLOCK_NAME) {
		const $pos = editor.state.doc.resolve(nodePos + 1)

		for (let d = $pos.depth; d > 0; d--) {
			const ancestor = $pos.node(d)
			if (ancestor.type.name === "metricGrid") {
				return {
					startPos: $pos.before(d),
					endPos: $pos.after(d),
				}
			}
		}
	}

	return {
		startPos: nodePos,
		endPos: nodePos + node.nodeSize,
	}
}
</script>
<template>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('above')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-start" class="scale-x-[-1] transform" />
			<span>
				{{ $t("editor.drag-handle.options.default.add-above-block") }}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
	<ShadcnUiDropdownMenuItem @click="addNeighborBlock('below')">
		<div class="flex w-full items-center gap-1">
			<Icon name="lucide:list-end" class="scale-x-[-1] transform" />
			<span>
				{{ $t("editor.drag-handle.options.default.add-below-block") }}
			</span>
		</div>
	</ShadcnUiDropdownMenuItem>
</template>
