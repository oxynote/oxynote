<script lang="ts" setup>
import type { HookGroupStatus } from "../../hooks/hook-status"
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import type { RightSideBlockType } from "../../blocks/split-documentation"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"

const props = defineProps<{
	editor: Editor
	hovered: {
		node: Node
		nodePos: number
		nodeId: string
		nodeHooks: DocumentHook[] | null
		nodeHookStatus: HookGroupStatus | null
	} | null
}>()

const { t } = useI18n({ useScope: "global" })

const sides = computed(
	(): { value: "above" | "below"; label: string; icon: string }[] => [
		{
			value: "above",
			label: t(
				"editor.drag-handle.options.split-doc-right-side.add-above-block",
			),
			icon: "lucide:list-start",
		},
		{
			value: "below",
			label: t(
				"editor.drag-handle.options.split-doc-right-side.add-below-block",
			),
			icon: "lucide:list-end",
		},
	],
)

const blockTypes = computed(
	(): { value: RightSideBlockType; label: string; icon: string }[] => [
		{
			value: "code",
			label: t(
				"editor.drag-handle.options.split-doc-right-side.block-types.code",
			),
			icon: BLOCK_ICONS.codeBlock,
		},
		{
			value: "metrics",
			label: t(
				"editor.drag-handle.options.split-doc-right-side.block-types.metrics",
			),
			icon: BLOCK_ICONS.metricBlock,
		},
		{
			value: "diagram",
			label: t(
				"editor.drag-handle.options.split-doc-right-side.block-types.diagram",
			),
			icon: BLOCK_ICONS.mermaidBlock,
		},
	],
)

function addNeighborBlock(
	side: "above" | "below",
	blockType: RightSideBlockType,
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
	<ShadcnUiDropdownMenuSub v-for="side in sides" :key="side.value">
		<ShadcnUiDropdownMenuSubTrigger>
			<div class="flex w-full items-center gap-1">
				<Icon :name="side.icon" class="scale-x-[-1] transform" />
				<span>{{ side.label }}</span>
			</div>
		</ShadcnUiDropdownMenuSubTrigger>
		<ShadcnUiDropdownMenuSubContent>
			<ShadcnUiDropdownMenuItem
				v-for="blockType in blockTypes"
				:key="blockType.value"
				@click="addNeighborBlock(side.value, blockType.value)"
			>
				<div class="flex w-full items-center gap-1">
					<Icon :name="blockType.icon" />
					<span>{{ blockType.label }}</span>
				</div>
			</ShadcnUiDropdownMenuItem>
		</ShadcnUiDropdownMenuSubContent>
	</ShadcnUiDropdownMenuSub>
</template>
