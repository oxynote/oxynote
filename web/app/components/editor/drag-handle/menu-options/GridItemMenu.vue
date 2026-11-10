<script lang="ts" setup>
import type { HookGroupStatus } from "../../hooks/hook-status"
import type { Node } from "@tiptap/pm/model"
import type { Editor } from "@tiptap/vue-3"
import { BLOCK_GRID_NAMES, gridSizeLabel } from "../../blocks/grid/utils"
import {
	GRID_HEIGHT_ROWS,
	GRID_WIDTH_COLUMNS,
	gridItemOf,
	GridItemHeight,
	GridItemWidth,
	heightName,
	resizeInGrid,
	widthName,
} from "@oxynote/common/grid"
import { gridLayoutOf, writeGridPlaces } from "../../blocks/grid/layout"
import { isWideGrid } from "../../blocks/grid/screen-order"

const WIDTH_ICONS: Record<GridItemWidth, string> = {
	[GridItemWidth.Compact]: "lucide:signal-low",
	[GridItemWidth.Standard]: "lucide:signal-medium",
	[GridItemWidth.Wide]: "lucide:signal-high",
}

const HEIGHT_ICONS: Record<GridItemHeight, string> = {
	[GridItemHeight.Standard]: "mingcute:rectangle-line",
	[GridItemHeight.Tall]: "mingcute:rectangle-vertical-line",
}

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

// a grid that stacks its blocks shows each one at full width. A size set
// there would also move blocks in the wide layout, which is out of view.
// So the menu offers no size there.
const resizable = computed(() => {
	if (props.hovered == null) {
		return false
	}

	const { state, view } = props.editor
	const pos = props.hovered.nodePos

	// the block can be gone by the time the menu opens
	if (pos > state.doc.content.size || !state.doc.nodeAt(pos)) {
		return false
	}

	if (!BLOCK_GRID_NAMES.has(state.doc.resolve(pos).parent.type.name)) {
		return true
	}

	const content = view.nodeDOM(pos)?.parentElement

	return (
		!(content instanceof HTMLElement) ||
		isWideGrid(content.getBoundingClientRect().width)
	)
})
const size = computed(() => gridItemOf(props.hovered?.node.attrs ?? {}))
const widths = computed(() =>
	[...GRID_WIDTH_COLUMNS].reverse().map(([name, columns]) => ({
		value: columns,
		label: t(`editor.grid-item.width-options.${name}`),
		icon: WIDTH_ICONS[name],
	})),
)
const heights = computed(() =>
	GRID_HEIGHT_ROWS.map(([name, rows]) => ({
		value: rows,
		label: t(`editor.grid-item.height-options.${name}`),
		icon: HEIGHT_ICONS[name],
	})),
)
// a size without a name in the menu. It shows only while the block has it.
const otherWidth = computed(() =>
	widthName(size.value.w) ? null : gridSizeLabel(t, "width", size.value.w),
)
const otherHeight = computed(() =>
	heightName(size.value.h) ? null : gridSizeLabel(t, "height", size.value.h),
)

// resize gives the block new columns or rows. The size it keeps is read
// from the block now, since another user may have changed it since the
// menu opened. In a grid, the grid makes room. The blocks beside it slide
// and the ones below it move. Then every block gets its new cells.
function resize(change: { w: number } | { h: number }) {
	if (props.hovered == null) {
		return
	}

	const { state, view } = props.editor
	const pos = props.hovered.nodePos
	const node = state.doc.nodeAt(pos)

	if (!node) {
		return
	}

	const { w, h } = { ...gridItemOf(node.attrs), ...change }
	const $pos = state.doc.resolve(pos)
	const grid = $pos.parent
	const tr = state.tr

	// outside a grid a block has no place, only its size
	if (!BLOCK_GRID_NAMES.has(grid.type.name)) {
		view.dispatch(
			tr.setNodeMarkup(pos, null, { ...node.attrs, width: w, height: h }),
		)

		return
	}

	writeGridPlaces(
		tr,
		$pos.before(),
		resizeInGrid(gridLayoutOf(grid), $pos.index(), w, h),
		null,
	)
	view.dispatch(tr)
}
</script>
<template>
	<template v-if="resizable">
		<ShadcnUiDropdownMenuSub>
			<ShadcnUiDropdownMenuSubTrigger>
				<div class="flex w-full items-center gap-1">
					<Icon name="lucide:move-horizontal" />
					<span>{{ $t("editor.drag-handle.options.grid-item.width") }}</span>
				</div>
			</ShadcnUiDropdownMenuSubTrigger>
			<ShadcnUiDropdownMenuSubContent>
				<ShadcnUiDropdownMenuItem v-if="otherWidth" active disabled>
					<div class="flex w-full items-center gap-1">
						<Icon name="mingcute:ruler-line" />
						<span>{{ otherWidth }}</span>
					</div>
				</ShadcnUiDropdownMenuItem>
				<ShadcnUiDropdownMenuItem
					v-for="width in widths"
					:key="width.value"
					:active="width.value === size.w"
					@click="resize({ w: width.value })"
				>
					<div class="flex w-full items-center gap-1">
						<Icon :name="width.icon" />
						<span>
							{{ width.label }}
						</span>
					</div>
				</ShadcnUiDropdownMenuItem>
			</ShadcnUiDropdownMenuSubContent>
		</ShadcnUiDropdownMenuSub>
		<ShadcnUiDropdownMenuSub>
			<ShadcnUiDropdownMenuSubTrigger>
				<div class="flex w-full items-center gap-1">
					<Icon name="lucide:move-vertical" />
					<span>{{ $t("editor.drag-handle.options.grid-item.height") }}</span>
				</div>
			</ShadcnUiDropdownMenuSubTrigger>
			<ShadcnUiDropdownMenuSubContent>
				<ShadcnUiDropdownMenuItem v-if="otherHeight" active disabled>
					<div class="flex w-full items-center gap-1">
						<Icon name="mingcute:ruler-line" />
						<span>{{ otherHeight }}</span>
					</div>
				</ShadcnUiDropdownMenuItem>
				<ShadcnUiDropdownMenuItem
					v-for="height in heights"
					:key="height.value"
					:active="height.value === size.h"
					@click="resize({ h: height.value })"
				>
					<div class="flex w-full items-center gap-1">
						<Icon :name="height.icon" />
						<span>
							{{ height.label }}
						</span>
					</div>
				</ShadcnUiDropdownMenuItem>
			</ShadcnUiDropdownMenuSubContent>
		</ShadcnUiDropdownMenuSub>
	</template>
</template>
