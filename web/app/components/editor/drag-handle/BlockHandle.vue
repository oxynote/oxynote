<script lang="ts" setup>
import { highlightOverlayByNodeType } from "./config"
import { DragHandle } from "./MainElement"
import type { Editor } from "@tiptap/vue-3"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import { cn } from "~/lib/utils"
import HookMenuContent from "../hooks/HookMenuContent.vue"
import CoreMenu from "./menu-options/CoreMenu.vue"

// duration to wait before unlocking drag handle after menu closes,
// allowing the close animation to complete without visual flicker
const MENU_CLOSE_ANIMATION_DURATION = 150

const props = defineProps<{
	editor: Editor
	documentHooks?: DocumentHook[]
	dataSyncProvider?: HocuspocusProvider | null
}>()
const emit = defineEmits<{
	(
		e: "add-node-comment" | "open-node-comment" | "delete-node-comment",
		pos: number,
	): void
	(e: "open-settings", target: "github"): void
}>()

const { isLocked, updateLock, isEditable } = useEditorMeta()
const { isScrolling } = useWindowScroll()
const isMinWidth1024px = useMediaQuery("(min-width: 1024px)")
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const dragOverlayElement = ref<HTMLElement | null>(null)
const nodeActionMenuOpen = ref(false)
const hookMenuOpen = ref(false)
const hoveringDrag = ref(false)

const processedDocumentHooks = computed(() => {
	return props.documentHooks ?? []
})
const hoveredNodePos = ref<number | null>(null)
const hoveredMetadata = computed(() => {
	if (!props.editor || hoveredNodePos.value == null) {
		return null
	}

	const node = props.editor.state.doc.nodeAt(hoveredNodePos.value)
	if (!node) {
		return null
	}

	const nodeId = node.attrs["uid"] as string
	const hooks = processedDocumentHooks.value.filter((h) => {
		return h.blockId === nodeId
	})

	return {
		node: node,
		nodePos: hoveredNodePos.value,
		nodeId: nodeId,
		nodeHooks: hooks.length ? hooks : null,
		nodeHookStatus: hooks.length
			? ((hooks.some((h) => Number(h.score) === 0) ? "stale" : "fresh") as
					| "stale"
					| "fresh")
			: null,
	}
})
const compactHandles = computed(() => {
	return !isMinWidth1024px.value
})

watch([nodeActionMenuOpen, hookMenuOpen], ([nodeOpen, hookOpen]) => {
	const anyOpen = nodeOpen || hookOpen

	if (anyOpen) {
		updateLock(true)

		const nodeEl = getCurrentNodeEl()
		if (nodeEl) {
			createDragOverlay(nodeEl)
		}
	} else {
		// delay unlock to let close animation complete without visual flicker
		setTimeout(() => {
			// only unlock if no menu has reopened in the meantime
			if (!nodeActionMenuOpen.value && !hookMenuOpen.value) {
				updateLock(false)
			}
		}, MENU_CLOSE_ANIMATION_DURATION)
	}

	if (!hoveringDrag.value && !anyOpen) {
		removeDragOverlay()
	}
})
whenever(isScrolling, () => {
	removeDragOverlay()
})

function handleNodeHover(data: { pos: number; node: any | null }) {
	if (!data.node) {
		hoveredNodePos.value = null
		return
	}

	hoveredNodePos.value = data.pos
}

function getCurrentNodeEl(): HTMLElement | null {
	if (!props.editor || hoveredNodePos.value == null) {
		return null
	}

	return props.editor.view.nodeDOM(hoveredNodePos.value) as HTMLElement | null
}

function updateDragOverlayPosition(nodeEl: HTMLElement) {
	const rect = nodeEl.getBoundingClientRect()
	const nodeInfo = highlightOverlayByNodeType(
		props.editor,
		hoveredNodePos.value ?? 0,
		nodeEl,
	)

	const left = rect.left - nodeInfo.extraLeft
	const top = rect.top - nodeInfo.extraTop
	const width = rect.width + nodeInfo.extraLeft + nodeInfo.extraRight
	const height = rect.height + nodeInfo.extraTop + nodeInfo.extraBottom

	if (!dragOverlayElement.value) {
		return
	}

	const el = dragOverlayElement.value
	el.style.left = `${left}px`
	el.style.top = `${top}px`
	el.style.width = `${width}px`
	el.style.height = `${height}px`
}

function handleDragHoverEnter() {
	hoveringDrag.value = true

	const nodeEl = getCurrentNodeEl()
	if (nodeEl) {
		createDragOverlay(nodeEl)
	}
}

function handleDragHoverLeave() {
	hoveringDrag.value = false

	if (hookMenuOpen.value || nodeActionMenuOpen.value) {
		return
	}

	removeDragOverlay()
}

function createDragOverlay(nodeEl: HTMLElement) {
	removeDragOverlay()

	const el = document.createElement("div")
	el.setAttribute("aria-hidden", "true")

	el.className = cn(
		"fixed left-0 top-0 w-0 h-0 pointer-events-none z-editor-overlay transition-none rounded-md bg-drag-target/20",
	)

	document.body.appendChild(el)
	dragOverlayElement.value = el

	updateDragOverlayPosition(nodeEl)
}

function removeDragOverlay() {
	const el = dragOverlayElement.value
	if (el && (el as any)._cleanup) {
		;(el as any)._cleanup()
	}

	if (el && el.parentNode) {
		el.parentNode.removeChild(el)
	}

	dragOverlayElement.value = null
}

function dragStart(e: DragEvent) {
	if (isEditingDisabled.value) {
		e.preventDefault()
		return
	}

	document.documentElement.classList.add(
		cn("cursor-grabbing!"),
		cn("[&_*]:cursor-grabbing!"),
	)
}

function dragEnd() {
	document.documentElement.classList.remove(
		cn("cursor-grabbing!"),
		cn("[&_*]:cursor-grabbing!"),
	)
}
</script>

<template>
	<DragHandle
		:editor="props.editor"
		:provider="props.dataSyncProvider"
		:locked="isLocked"
		:on-drag-cancel="dragEnd"
		class="z-drag-handle pr-1.5"
		@node-change="handleNodeHover"
		@mouseenter="handleDragHoverEnter"
		@mouseleave="handleDragHoverLeave"
		@dragstart="dragStart"
		@dragend="dragEnd"
	>
		<div
			class="flex flex-col items-center gap-0.25 rounded-md bg-background-translucent lg:flex-row"
		>
			<ShadcnUiDropdownMenu
				v-if="!compactHandles && !isEditingDisabled"
				@update:open="(value: boolean) => (hookMenuOpen = value)"
			>
				<ShadcnUiDropdownMenuTrigger as-child>
					<div
						:data-menu-open="hookMenuOpen ? '' : undefined"
						:class="
							cn(
								'flex size-5.5 cursor-pointer items-center justify-center rounded-md text-foreground/50',
								'hover:bg-sidebar-accent/50 active:bg-sidebar-accent data-menu-open:bg-sidebar-accent/50 data-menu-open:active:bg-sidebar-accent/50',
							)
							// we cannot have a @mousedown.stop.prevent here
							// because the 'active' state would not be applied
						"
					>
						<Icon
							:data-hook-status="hoveredMetadata?.nodeHookStatus"
							name="mingcute:leaf-line"
							:class="
								cn(
									'mt-0.25 size-4.5',
									'data-[hook-status=fresh]:text-hook-status-fresh data-[hook-status=stale]:text-hook-status-stale',
								)
							"
						/>
					</div>
					<span class="sr-only">
						{{ $t("editor.hook-handle.screen-reader-hint") }}
					</span>
				</ShadcnUiDropdownMenuTrigger>
				<ShadcnUiDropdownMenuContent side="right" align="start" loop>
					<HookMenuContent
						:document-hooks="processedDocumentHooks"
						:node-id="hoveredMetadata?.nodeId || null"
						@open-settings="(target: 'github') => emit('open-settings', target)"
					/>
				</ShadcnUiDropdownMenuContent>
			</ShadcnUiDropdownMenu>
			<ShadcnUiDropdownMenu
				@update:open="(value: boolean) => (nodeActionMenuOpen = value)"
			>
				<ShadcnUiDropdownMenuTrigger as-child>
					<div
						:data-menu-open="nodeActionMenuOpen ? '' : undefined"
						:class="
							cn(
								'flex h-5 w-3 items-center justify-center rounded-sm text-foreground/50 lg:h-5.5 lg:w-4 lg:rounded-md',
								'hover:bg-sidebar-accent/50 active:bg-sidebar-accent data-menu-open:bg-sidebar-accent/50 data-menu-open:active:bg-sidebar-accent/50',
								isEditingDisabled ? 'cursor-pointer' : 'cursor-grab',
							)
						"
					>
						<div class="relative h-5 w-3 overflow-hidden lg:h-5 lg:w-4">
							<Icon
								name="mingcute:dots-line"
								class="absolute top-1/2 left-1/2 size-5 -translate-x-1/2 -translate-y-1/2"
							/>
						</div>
					</div>
					<span class="sr-only">
						{{ $t("editor.drag-handle.screen-reader-hint") }}
					</span>
				</ShadcnUiDropdownMenuTrigger>
				<ShadcnUiDropdownMenuContent side="left" align="start" loop>
					<template v-if="compactHandles && !isEditingDisabled">
						<HookMenuContent
							:document-hooks="processedDocumentHooks"
							:node-id="hoveredMetadata?.nodeId || null"
							@open-settings="
								(target: 'github') => emit('open-settings', target)
							"
						/>
						<ShadcnUiDropdownMenuSeparator />
					</template>
					<CoreMenu
						:editor="props.editor"
						:data-sync-provider="props.dataSyncProvider"
						:hovered="hoveredMetadata"
						@add-node-comment="(pos: number) => emit('add-node-comment', pos)"
						@open-node-comment="(pos: number) => emit('open-node-comment', pos)"
						@delete-node-comment="
							(pos: number) => emit('delete-node-comment', pos)
						"
					/>
				</ShadcnUiDropdownMenuContent>
			</ShadcnUiDropdownMenu>
		</div>
	</DragHandle>
</template>
