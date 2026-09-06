<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper } from "@tiptap/vue-3"
import { nanoid } from "nanoid"
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import {
	fileKind,
	fileKindStyle,
	formatFileSize,
	isViewable,
} from "./file-kind"

const props = defineProps(nodeViewProps)

const { t } = useI18n({ useScope: "global" })
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { uploadDocumentFile } = useDocumentFileAPI()
const { $coreAPIClient } = useNuxtApp()

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

	try {
		const { protocol } = new URL(value, "http://relative.invalid")

		return protocol === "http:" || protocol === "https:" ? value : ""
	} catch {
		return ""
	}
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
		case DiffStatus.Modified:
			return "diff-modified"
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

	const blockId = (props.node.attrs.uid as string) || nanoid()

	props.updateAttributes({ uploading: true, name: file.name, size: file.size })

	try {
		const uploaded = await uploadDocumentFile.mutateAsync({
			documentId: documentId.value,
			id: blockId,
			loc: DocumentFileLocation.Document,
			kind: DocumentFileKind.File,
			file,
		})
		props.updateAttributes({
			src: buildDocumentFileSrc(documentId.value, blockId, uploaded.name),
			uid: blockId,
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
	try {
		const blob = await $coreAPIClient<Blob, "blob">(src.value, {
			responseType: "blob",
		})
		const url = URL.createObjectURL(blob)
		const anchor = document.createElement("a")

		anchor.href = url
		anchor.download = name.value
		anchor.click()
		URL.revokeObjectURL(url)
	} catch (error) {
		console.error("Failed to download file:", error)
		showToastMessage("error", t("editor.file.errors.download-failed"))
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
						kindStyle.accentClass,
					)
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
		</component>
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
			<Icon name="lucide:paperclip" class="size-4.5 text-foreground" />
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
				class="hidden"
				@change="handleFileChange"
			/>
		</div>
	</NodeViewWrapper>
</template>
