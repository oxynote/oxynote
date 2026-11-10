<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper, NodeViewContent } from "@tiptap/vue-3"
import { readingOrder } from "@oxynote/common/grid"
import { cn } from "#layers/ui/app/lib/utils"
import { gridLayoutOf, gridStackOf } from "./layout"
import {
	isWideGrid,
	moveTabFocus,
	redirectTabEntry,
	tabDirection,
	trackTabbing,
} from "./screen-order"

const props = defineProps(nodeViewProps)

const content = useTemplateRef("content")
const { width } = useElementSize(content)
const wide = computed(() => isWideGrid(width.value))

// the uids of the grid's blocks in the order the grid shows them at its
// width. Screen readers follow this order through aria-owns. Tab follows
// it through the handlers below. Chromium's reading-flow does the same on
// its own.
const screenOrder = computed(() => {
	const order = wide.value
		? readingOrder(gridLayoutOf(props.node))
		: gridStackOf(props.node)

	return order.flatMap((i) => {
		const uid: unknown = props.node.child(i).attrs.uid

		return typeof uid === "string" ? [uid] : []
	})
})

onMounted(trackTabbing)

function onKeydown(event: KeyboardEvent) {
	const grid = event.currentTarget as HTMLElement

	moveTabFocus(event, grid, () => screenBlocks(grid))
}

// focus that Tab brings into the grid goes to its first or last block
function onFocusin(event: FocusEvent) {
	const grid = event.currentTarget as HTMLElement
	const tabbing = tabDirection()

	if (tabbing) {
		redirectTabEntry(event, grid, screenBlocks(grid), tabbing === "backward")
	}
}

// the grid's block elements in screen order. A copied document can hold a
// uid twice, so each element is taken once.
function screenBlocks(grid: HTMLElement): HTMLElement[] {
	const children = Array.from(grid.children) as HTMLElement[]

	return screenOrder.value.flatMap((uid) => {
		const at = children.findIndex((child) => child.id === uid)

		return at < 0 ? [] : children.splice(at, 1)
	})
}
</script>
<template>
	<NodeViewWrapper
		as="div"
		data-type="block-grid"
		:data-diff-status="props.node.attrs.diffStatus"
		:class="
			cn('block-grid @container', 'not-has-[>.block-grid-content>*]:hidden')
		"
	>
		<!-- the grid layout plugin gives every block its cells and its place
		in the stacked layout as CSS variables. ProseMirror renders the blocks,
		so the grid styles them through its children (*:). A wide grid places
		each block in its cells. A narrower one stacks the blocks one per row
		in their stacked order. A row is as tall as a standard block (h-80),
		so the block stretches to its rows. -->
		<NodeViewContent
			ref="content"
			:class="
				cn(
					'block-grid-content grid [grid-auto-flow:row] auto-rows-[--spacing(80)] gap-3 [reading-flow:grid-rows]',
					'grid-cols-1',
					'*:data-grid-cell:order-(--grid-order) *:data-grid-cell:col-span-full',
					'*:data-grid-cell:row-span-(--grid-h) *:data-grid-cell:h-auto',
					'@5xl:grid-cols-[repeat(24,minmax(0,1fr))]',
					'@5xl:*:data-grid-cell:order-none',
					'@5xl:*:data-grid-cell:col-[var(--grid-col)/span_var(--grid-w)]',
					'@5xl:*:data-grid-cell:row-[var(--grid-row)/span_var(--grid-h)]',
				)
			"
			:aria-owns="screenOrder.join(' ') || undefined"
			@keydown="onKeydown"
			@focusin="onFocusin"
		/>
	</NodeViewWrapper>
</template>
