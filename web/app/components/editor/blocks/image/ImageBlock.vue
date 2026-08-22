<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { nanoid } from "nanoid"
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

const MIN_WIDTH = 128

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { uploadDocumentFile } = useDocumentFileAPI()

const imgRef = ref<HTMLImageElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const dragWidth = ref<number | null>(null)
const isResizing = ref(false)

let resizeMoveHandler: ((event: MouseEvent) => void) | null = null
let resizeEndHandler: (() => void) | null = null

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const src = computed(() => props.node.attrs.src as string)
const alt = computed(() => (props.node.attrs.alt as string) || "")
const title = computed(() => (props.node.attrs.title as string) || "")
const uploading = computed(() => props.node.attrs.uploading as boolean)
const hasImage = computed(() => Boolean(src.value))

const diffClass = computed(() => {
	switch (props.node.attrs.diffStatus) {
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

const widthAttr = computed(() => toNumber(props.node.attrs.width))

const displayWidth = computed(() => dragWidth.value ?? widthAttr.value)

const imageStyle = computed(() => {
	const styles: Record<string, string> = {}

	if (displayWidth.value) {
		styles.width = `${displayWidth.value}px`
	}

	return styles
})

const showResizeHandle = computed(
	() => !isEditingDisabled.value && hasImage.value,
)
const documentId = computed(() => {
	const route = useRoute()
	const slug = route.params.documentSlug
	if (typeof slug !== "string") {
		return null
	}

	return extractDocInfoFromSlug(slug)?.id ?? null
})
const canUpload = computed(() => {
	return (
		!isEditingDisabled.value && Boolean(documentId.value) && !uploading.value
	)
})

function toNumber(value: unknown): number | null {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value
	}

	if (typeof value === "string") {
		const parsed = Number.parseInt(value, 10)
		return Number.isFinite(parsed) ? parsed : null
	}

	return null
}

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
	if (isEditingDisabled.value) {
		return
	}

	const image = imgRef.value
	if (!image) {
		return
	}

	event.preventDefault()
	event.stopPropagation()

	const rect = image.getBoundingClientRect()
	const startWidth = rect.width || image.naturalWidth
	const startHeight = rect.height || image.naturalHeight

	if (!startWidth || !startHeight) {
		return
	}

	cleanupResizeListeners()
	isResizing.value = true

	const startX = event.clientX
	const startY = event.clientY

	resizeMoveHandler = (moveEvent: MouseEvent) => {
		const dx = moveEvent.clientX - startX
		const dy = moveEvent.clientY - startY
		const scaleX = (startWidth + dx) / startWidth
		const scaleY = (startHeight + dy) / startHeight
		const scale = Math.max(scaleX, scaleY)

		const nextWidth = Math.max(MIN_WIDTH, Math.round(startWidth * scale))
		dragWidth.value = nextWidth
	}

	resizeEndHandler = () => {
		cleanupResizeListeners()

		if (dragWidth.value) {
			props.updateAttributes({
				width: dragWidth.value,
			})
		}

		dragWidth.value = null
		isResizing.value = false
	}

	window.addEventListener("mousemove", resizeMoveHandler)
	window.addEventListener("mouseup", resizeEndHandler)
}

function openFilePicker() {
	if (!canUpload.value) {
		return
	}

	fileInputRef.value?.click()
}

async function handleFileChange(event: Event) {
	const target = event.target as HTMLInputElement | null
	const file = target?.files?.[0]
	if (!file) {
		return
	}

	if (!documentId.value) {
		showToastMessage("error", t("editor.image.upload-unavailable"))
		return
	}

	const blockId = (props.node.attrs.uid as string) || nanoid()

	props.updateAttributes({ uploading: true })

	try {
		await uploadDocumentFile.mutateAsync({
			documentId: documentId.value,
			id: blockId,
			loc: DocumentFileLocation.Document,
			file,
		})
		const nextSrc = buildDocumentFileSrc(documentId.value, blockId)
		props.updateAttributes({ src: nextSrc, uid: blockId, uploading: false })
	} catch (error) {
		props.updateAttributes({ uploading: false })
		if (!handleStorageError(error, t)) {
			console.error("Failed to upload image:", error)
			showToastMessage("error", t("editor.image.errors.upload-failed"))
		}
	} finally {
		target.value = ""
	}
}

onBeforeUnmount(() => {
	cleanupResizeListeners()
})
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="figure"
		class="not-prose flex caret-transparent"
		data-type="imageBlock"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<div
			v-if="hasImage"
			:class="
				cn(
					'group relative inline-block max-w-full',
					diffClass && 'diff-overlay-anchor',
				)
			"
		>
			<span
				v-if="diffClass"
				:class="[
					'diff-overlay z-editor-overlay!',
					/* overwrite the z-index to make the status more visible */ diffClass,
				]"
				class="-inset-[0.2em]"
			/>
			<img
				ref="imgRef"
				:src="src"
				:alt="alt"
				:title="title"
				:style="imageStyle"
				:class="[
					'mt-0 mb-0 block h-auto max-w-full rounded-lg',
					!displayWidth ? 'w-32' : '',
				]"
				draggable="false"
			/>
			<div
				v-if="showResizeHandle"
				aria-label="Resize image"
				class="absolute right-0.25 bottom-0.25 size-4 cursor-nwse-resize text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
				@mousedown="startResize"
			>
				<Icon name="lucide:move-diagonal-2" />
			</div>
		</div>
		<div
			v-else
			:class="
				cn(
					'relative flex w-full items-center gap-2 rounded-md bg-muted p-2 transition-colors duration-150 select-none',
					canUpload && 'active:bg-muted-90 cursor-pointer hover:bg-muted/70',
					diffClass,
				)
			"
			@click="openFilePicker"
		>
			<Icon name="lucide:image" class="size-4.5 text-foreground" />
			<div class="mt-0.25 text-2sm text-muted-foreground">
				{{
					uploading
						? t("editor.image.uploading")
						: !documentId
							? t("editor.image.upload-unavailable")
							: !isEditingDisabled
								? t("editor.image.description")
								: t("editor.image.empty")
				}}
			</div>
			<input
				ref="fileInputRef"
				type="file"
				accept="image/png,image/jpeg,image/webp"
				class="hidden"
				@change="handleFileChange"
			/>
		</div>
	</NodeViewWrapper>
</template>
