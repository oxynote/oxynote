<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { computePosition, flip, offset, shift } from "@floating-ui/dom"
import { cn } from "~/lib/utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { isFigmaUrl, convertToEmbedUrl } from "./index"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)

const { isDark } = useAppearance()

const src = computed(() => props.node.attrs.src as string | null)
// convertToEmbedUrl assumes a figma URL; anything else would produce a
// broken https://embed.figma.com/undefined/undefined embed
const embedUrl = computed(() =>
	src.value && isFigmaUrl(src.value)
		? convertToEmbedUrl(src.value, isDark.value)
		: null,
)

const diffStatus = computed(
	() => props.node.attrs.diffStatus as DiffStatus | null,
)
const diffClass = computed(() => {
	switch (diffStatus.value) {
		case DiffStatus.Added:
			return "diff-added"
		case DiffStatus.Removed:
			return "diff-removed"
		case DiffStatus.Modified:
			return "diff-modified"
		default:
			return null
	}
})

// Resize
const MIN_WIDTH = 128
const DEFAULT_HEIGHT = 450

const anchorRef = useTemplateRef<HTMLElement>("figma-anchor")
const dragWidth = ref<number | null>(null)
const dragHeight = ref<number | null>(null)
const isResizing = ref(false)
let resizeMoveHandler: ((event: MouseEvent) => void) | null = null
let resizeEndHandler: (() => void) | null = null

function toAttrNumber(value: unknown): number | null {
	if (typeof value === "number" && Number.isFinite(value)) return value
	if (typeof value === "string") {
		const parsed = Number.parseInt(value, 10)
		return Number.isFinite(parsed) ? parsed : null
	}
	return null
}

const widthAttr = computed(() => toAttrNumber(props.node.attrs.width))
const heightAttr = computed(() => toAttrNumber(props.node.attrs.height))

const displayWidth = computed(() => dragWidth.value ?? widthAttr.value)
const displayHeight = computed(
	() => dragHeight.value ?? heightAttr.value ?? DEFAULT_HEIGHT,
)

const containerStyle = computed(() => {
	const styles: Record<string, string> = {}
	if (displayWidth.value) {
		styles.width = `${displayWidth.value}px`
	}
	return styles
})

function cleanupResizeListeners() {
	if (resizeMoveHandler) {
		window.removeEventListener("mousemove", resizeMoveHandler)
		resizeMoveHandler = null
	}
	if (resizeEndHandler) {
		window.removeEventListener("mouseup", resizeEndHandler)
		resizeEndHandler = null
	}
}

function startResize(event: MouseEvent) {
	if (isEditingDisabled.value) return

	event.preventDefault()
	event.stopPropagation()

	const container = anchorRef.value
	if (!container) return

	const startWidth =
		displayWidth.value ?? container.getBoundingClientRect().width
	const startHeight = displayHeight.value
	const startX = event.clientX
	const startY = event.clientY

	cleanupResizeListeners()
	isResizing.value = true

	resizeMoveHandler = (moveEvent: MouseEvent) => {
		const nextWidth = Math.max(
			MIN_WIDTH,
			Math.round(startWidth + moveEvent.clientX - startX),
		)
		const nextHeight = Math.max(
			MIN_WIDTH,
			Math.min(Math.round(startHeight + moveEvent.clientY - startY), nextWidth),
		)
		dragWidth.value = nextWidth
		dragHeight.value = nextHeight
	}

	resizeEndHandler = () => {
		cleanupResizeListeners()
		if (dragWidth.value !== null || dragHeight.value !== null) {
			props.updateAttributes({
				width: dragWidth.value ?? widthAttr.value,
				height: dragHeight.value ?? heightAttr.value,
			})
		}
		dragWidth.value = null
		dragHeight.value = null
		isResizing.value = false
	}

	window.addEventListener("mousemove", resizeMoveHandler)
	window.addEventListener("mouseup", resizeEndHandler)
}

// Popover
const showPopover = ref(false)
const popoverRef = useTemplateRef<HTMLElement>("figma-url-popover")
const urlInput = ref("")
const popoverPos = ref({ top: 0, left: 0 })

async function updatePopoverPos() {
	const anchor = anchorRef.value
	const popover = popoverRef.value
	if (!anchor || !popover) return

	const { x, y } = await computePosition(anchor, popover, {
		placement: "bottom-start",
		strategy: "absolute",
		middleware: [offset(5), flip({ padding: 8 }), shift({ padding: 8 })],
	})
	popoverPos.value = { top: y, left: x }
}

function openPopover() {
	if (isEditingDisabled.value) return
	urlInput.value = src.value ?? ""
	showPopover.value = true
	void nextTick(updatePopoverPos)
}

function closePopover() {
	showPopover.value = false
	urlInput.value = ""
}

function submitUrl() {
	const trimmed = urlInput.value.trim()
	if (!trimmed || !isFigmaUrl(trimmed)) {
		return
	}

	props.updateAttributes({ src: trimmed })
	closePopover()
}

function handleKeydown(event: KeyboardEvent) {
	if (event.key === "Enter") {
		event.preventDefault()
		submitUrl()
	} else if (event.key === "Escape") {
		event.preventDefault()
		closePopover()
	}
}

function handleClickOutside(event: MouseEvent) {
	const popover = popoverRef.value
	const anchor = anchorRef.value
	const target = event.target as Node | null
	if (!target) return
	if (popover?.contains(target) || anchor?.contains(target)) return
	closePopover()
}

watch(showPopover, (val) => {
	if (val) {
		void nextTick(() => {
			document.addEventListener("mousedown", handleClickOutside)
		})
	} else {
		document.removeEventListener("mousedown", handleClickOutside)
	}
})

onBeforeUnmount(() => {
	cleanupResizeListeners()
	document.removeEventListener("mousedown", handleClickOutside)
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="div"
		class="not-prose"
		data-type="figmaBlock"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<!-- Filled state -->
		<div
			v-if="embedUrl"
			ref="figma-anchor"
			:style="containerStyle"
			:class="
				cn(
					'group relative max-w-full overflow-hidden rounded-lg border border-border',
					diffClass && 'diff-overlay-anchor',
				)
			"
		>
			<span
				v-if="diffClass"
				:class="['diff-overlay z-editor-overlay!', diffClass]"
				class="-inset-[0.2em]"
			/>
			<div
				v-if="!isEditingDisabled"
				class="absolute bottom-13.5 left-1.5 z-1 opacity-0 transition-opacity group-hover:opacity-100 hover:opacity-100"
				contenteditable="false"
			>
				<ShadcnUiButton
					variant="ghost-plain"
					size="icon-sm"
					class="size-6 !bg-background/80"
					:aria-label="t('editor.figma.edit')"
					type="button"
					@click="openPopover"
				>
					<Icon name="lucide:pencil" class="size-3.5" />
				</ShadcnUiButton>
			</div>
			<iframe
				:src="embedUrl"
				class="block w-full border-0"
				:style="{ height: `${displayHeight}px` }"
				allowfullscreen
			/>
			<div
				v-if="!isEditingDisabled"
				aria-label="Resize"
				class="absolute right-0.25 bottom-0.25 size-4 cursor-nwse-resize text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
				contenteditable="false"
				@mousedown="startResize"
			>
				<Icon name="lucide:move-diagonal-2" />
			</div>
		</div>

		<!-- Empty state -->
		<div
			v-else
			ref="figma-anchor"
			contenteditable="false"
			:class="
				cn(
					'relative flex w-full items-center gap-2 rounded-md bg-muted p-2 transition-colors duration-150 select-none',
					!isEditingDisabled &&
						'cursor-pointer hover:bg-muted/70 active:bg-muted/90',
					diffClass,
				)
			"
			@click="openPopover"
		>
			<Icon name="simple-icons:figma" class="size-4.5 text-foreground" />
			<span class="mt-0.25 text-2sm text-muted-foreground">
				{{
					isEditingDisabled
						? t("editor.figma.empty")
						: t("editor.figma.description")
				}}
			</span>
		</div>
	</NodeViewWrapper>

	<!-- URL popover -->
	<Teleport to="body">
		<Transition
			enter-from-class="fade-in-0"
			enter-active-class="animate-in"
			enter-to-class="fade-in-100"
			leave-from-class="fade-in-100"
			leave-active-class="animate-out"
			leave-to-class="fade-out-0"
		>
			<div
				v-if="showPopover"
				ref="figma-url-popover"
				:style="{ top: `${popoverPos.top}px`, left: `${popoverPos.left}px` }"
				class="absolute z-popover rounded-lg border border-border bg-background px-1 py-1 shadow-md"
				contenteditable="false"
			>
				<div class="flex items-center gap-0.75">
					<ShadcnUiInput
						v-model="urlInput"
						:placeholder="t('editor.figma.placeholder')"
						class="w-64 border-none px-2 text-2sm shadow-none md:text-2sm"
						autofocus
						disable-focus-effect
						@keydown="handleKeydown"
					/>
					<div class="w-[0.0625rem] shrink-0 self-stretch bg-border" />
					<ShadcnUiButton size="icon-sm" variant="ghost" @click="closePopover">
						<Icon name="lucide:x" />
					</ShadcnUiButton>
				</div>
			</div>
		</Transition>
	</Teleport>
</template>
