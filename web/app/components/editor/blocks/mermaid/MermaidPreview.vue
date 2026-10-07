<script lang="ts" setup>
import { TAB_SIZE } from "./index"
import { sanitizeMermaidSvg } from "./sanitize-svg"
import { useMermaid } from "./useMermaid"

const sourceDebounceMs = 400

const props = defineProps<{
	source: string
	uid: string
}>()

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()

// the palette lives in CSS variables that switch with the html "dark"
// class, which unhead applies in a setTimeout after vue's flush. Following
// the class rather than the colour mode means the variables are already
// in place when the theme is read.
const root = document.documentElement
const isDark = ref(root.classList.contains("dark"))

useMutationObserver(
	root,
	() => {
		isDark.value = root.classList.contains("dark")
	},
	{ attributes: true, attributeFilter: ["class"] },
)

const { render, isLoading, loadError } = useMermaid(isDark)

const renderedSvg = ref("")
const renderError = ref("")
const isInitialRender = ref(!!props.source.trim())
const instanceId = Math.random().toString(36).slice(2, 8)
let renderCounter = 0

const debouncedSource = refDebounced(
	toRef(() => props.source),
	sourceDebounceMs,
)

// the page's fade-in waits for every block's first result; a block taken
// down before it has one must not hold the page up. On a document switch
// the previous document's blocks unmount after the store has moved on,
// and must not vouch for same-uid blocks of the next document.
const documentId = editorStore.activeDocumentId

onBeforeUnmount(() => {
	if (editorStore.activeDocumentId === documentId) {
		editorStore.markBlockRenderSettled(props.uid)
	}
})

watchImmediate(isInitialRender, (initialComplete) => {
	if (!initialComplete) {
		editorStore.markBlockRenderSettled(props.uid)
	}
})

watchImmediate([debouncedSource, isDark], async ([source]) => {
	if (!source.trim()) {
		renderedSvg.value = ""
		renderError.value = ""
		isInitialRender.value = false
		return
	}

	renderCounter++
	const currentRender = renderCounter
	const id = `mermaid-${instanceId}-${currentRender}`
	const sanitizedSource = source.replaceAll("\t", " ".repeat(TAB_SIZE))

	const result = await render(id, sanitizedSource)

	// Discard stale renders
	if (currentRender !== renderCounter) {
		return
	}

	if ("svg" in result) {
		renderedSvg.value = sanitizeMermaidSvg(result.svg)
		renderError.value = ""
	} else {
		renderedSvg.value = ""
		renderError.value = result.error
	}

	isInitialRender.value = false
})
</script>

<template>
	<div
		:class="[
			'mermaid-preview break-normal',
			isInitialRender ? 'opacity-0' : 'opacity-100',
		]"
	>
		<div v-if="isLoading" class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon :name="BLOCK_ICONS.mermaidBlock" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ t("editor.mermaid.preview.loading") }}
					</ShadcnUiEmptyTitle>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
		<div v-else-if="loadError" class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon :name="BLOCK_ICONS.mermaidBlock" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ t("editor.mermaid.preview.load-error") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						<div class="font-medium">
							{{ cleanSentenceCase(loadError) }}
						</div>
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
		<div v-else-if="renderError" class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon :name="BLOCK_ICONS.mermaidBlock" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ t("editor.mermaid.preview.render-error") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						<div class="font-medium">
							{{ cleanSentenceCase(renderError) }}
						</div>
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
		<!-- the block form is required: disable-next-line cannot reach the
			v-html attribute inside a multi-line element -->
		<!-- eslint-disable vue/no-v-html -- the svg is sanitized with DOMPurify right after rendering -->
		<div
			v-else-if="renderedSvg"
			class="flex items-center justify-center overflow-x-auto [&>svg]:h-auto [&>svg]:max-w-full"
			v-html="renderedSvg"
		/>
		<!-- eslint-enable vue/no-v-html -->
		<div v-else class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon :name="BLOCK_ICONS.mermaidBlock" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ t("editor.mermaid.preview.empty") }}
					</ShadcnUiEmptyTitle>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
	</div>
</template>
