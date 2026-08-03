<script lang="ts" setup>
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import { MetricBlockWidth } from "../../blocks/metrics/utils"

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

const { t } = useI18n({ useScope: "global" })

const sizes = computed(() => [
	{
		value: MetricBlockWidth.Compact,
		label: t("editor.drag-handle.options.metric-block.width-options.compact"),
		icon: "lucide:signal-low",
	},
	{
		value: MetricBlockWidth.Standard,
		label: t("editor.drag-handle.options.metric-block.width-options.standard"),
		icon: "lucide:signal-medium",
	},
	{
		value: MetricBlockWidth.Wide,
		label: t("editor.drag-handle.options.metric-block.width-options.wide"),
		icon: "lucide:signal-high",
	},
])

const currentSize = computed<MetricBlockWidth>(() => {
	return (
		(props.hovered?.node.attrs.width as MetricBlockWidth | undefined) ||
		MetricBlockWidth.Standard
	)
})

function setSize(width: MetricBlockWidth) {
	if (!props.editor || props.hovered == null) {
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
				<span>{{ $t("editor.drag-handle.options.metric-block.width") }}</span>
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
