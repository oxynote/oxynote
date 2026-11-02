<script lang="ts" setup>
import type { HookGroupStatus } from "../../hooks/hook-status"
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import { GridItemWidth } from "../../blocks/grid/utils"

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

const sizes = computed(() => [
	{
		value: GridItemWidth.Compact,
		label: t("editor.drag-handle.options.grid-item.width-options.compact"),
		icon: "lucide:signal-low",
	},
	{
		value: GridItemWidth.Standard,
		label: t("editor.drag-handle.options.grid-item.width-options.standard"),
		icon: "lucide:signal-medium",
	},
	{
		value: GridItemWidth.Wide,
		label: t("editor.drag-handle.options.grid-item.width-options.wide"),
		icon: "lucide:signal-high",
	},
])

const currentSize = computed<GridItemWidth>(() => {
	return (
		(props.hovered?.node.attrs.width as GridItemWidth | undefined) ||
		GridItemWidth.Standard
	)
})

function setSize(width: GridItemWidth) {
	if (props.hovered == null) {
		return
	}

	const { state, view } = props.editor
	const pos = props.hovered.nodePos
	const node = state.doc.nodeAt(pos)

	if (!node) {
		return
	}

	view.dispatch(state.tr.setNodeMarkup(pos, null, { ...node.attrs, width }))
}
</script>
<template>
	<ShadcnUiDropdownMenuSub>
		<ShadcnUiDropdownMenuSubTrigger>
			<div class="flex w-full items-center gap-1">
				<Icon name="lucide:move-horizontal" />
				<span>{{ $t("editor.drag-handle.options.grid-item.width") }}</span>
			</div>
		</ShadcnUiDropdownMenuSubTrigger>
		<ShadcnUiDropdownMenuSubContent>
			<ShadcnUiDropdownMenuItem
				v-for="size in sizes"
				:key="size.value"
				:active="size.value === currentSize"
				@click="setSize(size.value)"
			>
				<div class="flex w-full items-center gap-1">
					<Icon :name="size.icon" />
					<span>
						{{ size.label }}
					</span>
				</div>
			</ShadcnUiDropdownMenuItem>
		</ShadcnUiDropdownMenuSubContent>
	</ShadcnUiDropdownMenuSub>
</template>
