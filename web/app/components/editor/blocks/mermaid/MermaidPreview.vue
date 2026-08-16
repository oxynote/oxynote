<script lang="ts" setup>
import DOMPurify from "dompurify"
import { TAB_SIZE } from "./index"
import { useMermaid } from "./useMermaid"

const sourceDebounceMs = 400

const props = defineProps<{
	source: string
	uid: string
}>()

const { t } = useI18n({ useScope: "global" })
const { isDark } = useAppearance()
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

watchImmediate([debouncedSource, isDark], async ([source], oldValues) => {
	// when the theme changes, wait for the browser to apply the
	// new CSS variables before reading them.
	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- oldValues is undefined on the immediate first run despite the declared type
	if (oldValues && oldValues[1] !== isDark.value) {
		// nextTick isn't sufficient, the CSS update happens after the next
		// paint. Wait for the next animation frame to ensure styles are up
		// to date.
		await new Promise((r) => requestAnimationFrame(r))
	}

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
		renderedSvg.value = DOMPurify.sanitize(result.svg, {
			USE_PROFILES: { svg: true, svgFilters: true },
			ADD_TAGS: ["foreignObject"],
			ADD_ATTR: ["dominant-baseline"],
		})
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
		:class="['mermaid-preview', isInitialRender ? 'opacity-0' : 'opacity-100']"
	>
		<div v-if="isLoading" class="text-foreground">
			<ShadcnUiEmpty>
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia variant="icon" class="size-9">
						<Icon name="lucide:network" class="size-6" />
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
						<Icon name="lucide:network" class="size-6" />
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
						<Icon name="lucide:network" class="size-6" />
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
						<Icon name="lucide:network" class="size-6" />
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle>
						{{ t("editor.mermaid.preview.empty") }}
					</ShadcnUiEmptyTitle>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</div>
	</div>
</template>
