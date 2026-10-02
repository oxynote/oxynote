<script lang="ts" setup>
import type { Editor } from "@tiptap/core"
import HookDiffMarker from "./HookDiffMarker.vue"
import { blockChangeCount, diffHooks, type HookChangeCount } from "./hook-diff"
import { handlePositionByNodeType } from "../drag-handle/config"

// the height of the hook button on a wide screen. A block whose handle is
// centred on it gets its marker centred the same way
const HANDLE_HEIGHT_REM = 1.375

interface MarkerPlacement {
	blockId: string
	count: HookChangeCount
	hookStatus: "fresh" | "stale" | null
	top: number
	right: number
}

const props = defineProps<{
	editor: Editor
	activeBranchHooks: DocumentHook[]
	targetBranchHooks: DocumentHook[]
	// the block the handle shows on
	hoveredBlockId: string | null
}>()

const rootElem = useTemplateRef<HTMLElement>("root")
const placements = shallowRef<MarkerPlacement[]>([])

const entries = computed(() =>
	diffHooks(props.activeBranchHooks, props.targetBranchHooks),
)

// the page's own hooks are marked by the title, not here
const changedBlocks = computed(() => {
	const ids = new Set(
		entries.value
			.map((entry) => entry.hook.blockId)
			.filter((id): id is string => id !== null),
	)

	return [...ids]
		.map((id) => ({ id: id, count: blockChangeCount(entries.value, id) }))
		.filter((block) => block.count.removed || block.count.added)
})

onMounted(() => {
	props.editor.on("transaction", placeMarkers)
	placeMarkers()
})

onBeforeUnmount(() => {
	props.editor.off("transaction", placeMarkers)
})

// a block moves when anything above it changes height, which the editor's
// own size reports, images and charts loading included
useResizeObserver(() => props.editor.view.dom, placeMarkers)

watch(changedBlocks, placeMarkers)

// placeMarkers puts each marker where the block's handle shows, with its
// right edge on the handle's right edge. Whole pixels keep the marker from
// drawing between two of them
function placeMarkers() {
	const root = rootElem.value
	if (!root) {
		return
	}

	const rootRect = root.getBoundingClientRect()
	const remPx = Number.parseFloat(
		getComputedStyle(document.documentElement).fontSize,
	)
	const positions = new Map<string, number>()

	props.editor.state.doc.descendants((node, pos) => {
		const uid = node.attrs.uid as string | null | undefined
		if (uid && !positions.has(uid)) {
			positions.set(uid, pos)
		}
	})

	placements.value = changedBlocks.value.flatMap((block) => {
		const pos = positions.get(block.id)
		if (pos === undefined) {
			return []
		}

		const dom = props.editor.view.nodeDOM(pos)
		if (!(dom instanceof HTMLElement)) {
			return []
		}

		const rect = dom.getBoundingClientRect()
		const config = handlePositionByNodeType(props.editor, pos)
		const anchor = rect.left - rootRect.left + config.xOffset
		const top =
			config.placement === "left"
				? rect.top + rect.height / 2 - (HANDLE_HEIGHT_REM * remPx) / 2
				: rect.top + config.yOffset

		return [
			{
				blockId: block.id,
				count: block.count,
				hookStatus: hookStatus(block.id),
				top: Math.round(top - rootRect.top),
				right: Math.round(rootRect.width - anchor),
			},
		]
	})
}

// hookStatus colours the marker's hook icon as the hook button would be
function hookStatus(blockId: string): "fresh" | "stale" | null {
	const hooks = props.activeBranchHooks.filter((h) => h.blockId === blockId)
	if (!hooks.length) {
		return null
	}

	return hooks.some((h) => Number(h.score) === 0) ? "stale" : "fresh"
}
</script>

<template>
	<div ref="root" class="pointer-events-none absolute inset-0">
		<!--
			the gap to the block centres the marker under the handle's drag dots
		-->
		<HookDiffMarker
			v-for="placement in placements"
			:key="placement.blockId"
			class="absolute pr-0.5 lg:pr-1"
			:style="{ top: `${placement.top}px`, right: `${placement.right}px` }"
			:count="placement.count"
			:hook-status="placement.hookStatus"
			:lifted="placement.blockId === props.hoveredBlockId"
		/>
	</div>
</template>
