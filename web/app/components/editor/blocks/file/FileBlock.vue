<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { nanoid } from "nanoid"
import { showProgressToast, showToastMessage } from "~/components/toast"
import type { ProgressToast } from "~/components/toast"
import { cn } from "~/lib/utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import DiffChangeMarker from "~/components/editor/diff/DiffChangeMarker.vue"
import {
	fileKind,
	fileKindStyle,
	formatFileSize,
	isViewable,
} from "./file-kind"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const { coreAPIBaseHttpURL } = useRuntimeConfig().public
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { uploadDocumentFile, downloadDocumentFile } = useDocumentFileAPI()

const fileInputRef = ref<HTMLInputElement | null>(null)

// the desktop app routes window.open and off-origin navigation to the
// system browser, which holds no session, so a link cannot reach the
// file there: the card fetches it through the api client instead and
// hands the bytes over as a download
const isDesktop = __DESKTOP_BUILD__

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
// the attribute is shared editor state that any collaborator can write,
// so only an http(s) address reaches the anchor: a javascript: URL would
// otherwise run for whoever clicks the card
const src = computed(() => {
	const value = (props.node.attrs.src as string) || ""

	return isDocumentFileSrc(value, coreAPIBaseHttpURL) ? value : ""
})
const name = computed(() => (props.node.attrs.name as string) || "")
const contentType = computed(
	() => (props.node.attrs.contentType as string) || "",
)
const size = computed(() => formatFileSize(props.node.attrs.size as number))
const uploading = computed(() => props.node.attrs.uploading as boolean)
const hasFile = computed(() => Boolean(src.value))
const viewable = computed(() => isViewable(contentType.value))
const kindStyle = computed(() =>
	fileKindStyle(fileKind(name.value, contentType.value)),
)

const diffClass = computed(() => {
	switch (props.node.attrs.diffStatus) {
		case DiffStatus.Added:
			return "diff-added"
		case DiffStatus.Removed:
			return "diff-removed"
		default:
			return null
	}
})

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
		showToastMessage("error", t("editor.file.upload-unavailable"))
		return
	}

	const fileId = nanoid()

	props.updateAttributes({ uploading: true, name: file.name, size: file.size })

	try {
		const uploaded = await uploadDocumentFile.mutateAsync({
			documentId: documentId.value,
			id: fileId,
			loc: DocumentFileLocation.Document,
			kind: DocumentFileKind.File,
			file,
		})
		props.updateAttributes({
			src: buildDocumentFileSrc(documentId.value, fileId, uploaded.name),
			name: uploaded.name,
			size: uploaded.size,
			contentType: uploaded.contentType,
			uploading: false,
		})
	} catch (error) {
		props.updateAttributes({
			uploading: false,
			name: null,
			size: null,
			contentType: null,
		})
		if (!handleStorageError(error, t)) {
			console.error("Failed to upload file:", error)
			showToastMessage("error", t("editor.file.errors.upload-failed"))
		}
	} finally {
		target.value = ""
	}
}

async function downloadOnDesktop() {
	// NOCOV: the desktop branch is compiled out of the web bundle.
	if (downloadDocumentFile.asyncStatus.value === "loading") {
		return
	}

	const fileName = name.value

	// the toast opens with the first progress report rather than on
	// click, so it never sits behind the save dialog
	let progressToast: ProgressToast | undefined
	const openToast = () => {
		progressToast ??= showProgressToast(
			t("editor.file.downloading", { name: fileName }),
		)

		return progressToast
	}

	try {
		const result = await downloadDocumentFile.mutateAsync({
			src: src.value,
			name: fileName,
			onProgress: (received, total) => {
				openToast().update(total > 0 ? received / total : null)
			},
		})

		if (result === "completed") {
			openToast().succeed(t("editor.file.downloaded", { name: fileName }))
		}
	} catch (error) {
		console.error("Failed to download file:", error)
		openToast().fail(t("editor.file.errors.download-failed"))
	}
}
</script>

<template>
	<NodeViewWrapper
		:id="props.node.attrs.uid"
		as="figure"
		class="not-prose flex caret-transparent"
		data-type="fileBlock"
		:data-uid="props.node.attrs.uid"
		:data-node-comment-id="props.node.attrs.nodeCommentId"
		:data-diff-status="props.node.attrs.diffStatus"
	>
		<component
			:is="isDesktop ? 'button' : 'a'"
			v-if="hasFile"
			:href="isDesktop ? undefined : src"
			:target="!isDesktop && viewable ? '_blank' : undefined"
			:rel="isDesktop ? undefined : 'noopener'"
			:type="isDesktop ? 'button' : undefined"
			:class="
				cn(
					'group relative flex w-full items-center gap-3 rounded-md border border-border bg-background p-2 text-left no-underline transition-colors duration-150 select-none hover:bg-muted/70',
					diffClass && 'diff-overlay-anchor',
				)
			"
			draggable="false"
			@click="isDesktop ? downloadOnDesktop() : undefined"
		>
			<!-- the overlay's z-index is raised so the status stays visible -->
			<span
				v-if="diffClass"
				:class="['diff-overlay z-editor-overlay!', diffClass]"
				class="-inset-[0.2em]"
			/>
			<span
				:class="
					cn(
						'flex size-9 shrink-0 items-center justify-center rounded-md',
						kindStyle.tint
							? 'bg-(--kind-bg) text-(--kind-fg) dark:bg-(--kind-dark-bg) dark:text-(--kind-dark-fg)'
							: 'bg-muted text-muted-foreground',
					)
				"
				:style="
					kindStyle.tint
						? {
								'--kind-bg': kindStyle.tint.lightBg,
								'--kind-fg': kindStyle.tint.lightFg,
								'--kind-dark-bg': kindStyle.tint.darkBg,
								'--kind-dark-fg': kindStyle.tint.darkFg,
							}
						: undefined
				"
			>
				<Icon :name="kindStyle.icon" class="size-5" />
			</span>
			<span class="flex min-w-0 flex-col">
				<span class="truncate text-2sm font-medium text-foreground">
					{{ name }}
				</span>
				<span v-if="size" class="text-xs text-muted-foreground">
					{{ size }}
				</span>
			</span>
			<DiffChangeMarker
				:node="props.node"
				class="absolute top-1/2 right-2 -translate-y-1/2"
			/>
		</component>
		<button
			v-else
			type="button"
			:aria-disabled="!canUpload"
			:class="
				cn(
					'relative flex w-full items-center gap-2 rounded-md bg-muted p-2 text-left transition-colors duration-150 select-none',
					canUpload && 'active:bg-muted-90 cursor-pointer hover:bg-muted/70',
					diffClass,
				)
			"
			@click="openFilePicker"
		>
			<Icon name="mingcute:attachment-line" class="size-4 text-foreground" />
			<DiffChangeMarker
				:node="props.node"
				class="absolute top-1/2 right-2 -translate-y-1/2"
			/>
			<div class="mt-0.25 text-2sm text-muted-foreground">
				{{
					uploading
						? t("editor.file.uploading")
						: !documentId
							? t("editor.file.upload-unavailable")
							: !isEditingDisabled
								? t("editor.file.description")
								: t("editor.file.empty")
				}}
			</div>
			<input
				ref="fileInputRef"
				type="file"
				aria-hidden="true"
				class="hidden"
				@change="handleFileChange"
			/>
		</button>
	</NodeViewWrapper>
</template>
