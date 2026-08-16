<script lang="ts" setup>
import { nodeViewProps } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import {
	defaultExtendedCodeBlockLanguage,
	extendedCodeBlockLanguageOptions,
} from "./languages"
import { lowlight, type CodeBlockOptions } from "./index"

const props = defineProps({
	...nodeViewProps,
})
const { isEditable } = useEditorMeta()
const isCopied = ref(false)
const typeClass = computed(() => {
	return (props.extension.options as CodeBlockOptions).type === "comment"
		? "text-2sm"
		: "text-sm"
})

const detectedLanguage = ref(defaultExtendedCodeBlockLanguage)

function detectLanguage() {
	if (props.node.attrs.language) {
		return
	}

	const content = props.node.textContent
	if (!content.trim()) {
		detectedLanguage.value = defaultExtendedCodeBlockLanguage
		return
	}

	try {
		const result = lowlight.highlightAuto(content)

		if (
			result.data?.language &&
			result.data.relevance &&
			result.data.relevance >= 2
		) {
			detectedLanguage.value = result.data.language
			return
		}
	} catch {
		// Ignore errors
	}

	detectedLanguage.value = defaultExtendedCodeBlockLanguage
}

onBeforeMount(() => {
	detectLanguage()
})
onMounted(() => {
	props.editor.on("update", detectLanguage)
})
onUnmounted(() => {
	props.editor.off("update", detectLanguage)
})

const currentLang = computed({
	get: () => {
		// If language is null (auto mode), show detected language
		if (!props.node.attrs.language) {
			return detectedLanguage.value
		}

		return props.node.attrs.language as string
	},
	set: (lang: string) => {
		// User selected a language - set it (no longer auto)
		if (props.node.attrs.language !== lang) {
			props.updateAttributes({ language: lang })
		}
	},
})

async function copyCodeBlock() {
	try {
		await navigator.clipboard.writeText(props.node.textContent || "")
		isCopied.value = true
	} finally {
		setTimeout(() => (isCopied.value = false), 700)
	}
}
</script>
<template>
	<div
		v-show="!props.node.attrs.diffStatus"
		:class="
			cn(
				'flex items-center gap-2 caret-transparent transition-opacity',
				'absolute top-2 right-3 z-10 justify-end bg-muted p-1',
			)
		"
	>
		<ShadcnUiSelect v-model="currentLang">
			<ShadcnUiSelectTrigger
				ghost
				size="custom"
				:disable="!isEditable || !props.editor.isEditable"
				:class="typeClass"
			>
				<ShadcnUiSelectValue />
			</ShadcnUiSelectTrigger>
			<ShadcnUiSelectContent side="bottom" align="center" class="max-h-[40dvh]">
				<ShadcnUiSelectItem
					v-for="[langKey, langName] in Object.entries(
						extendedCodeBlockLanguageOptions,
					)"
					:key="langKey"
					:value="langKey"
					:class="typeClass"
				>
					{{ langName }}
				</ShadcnUiSelectItem>
			</ShadcnUiSelectContent>
		</ShadcnUiSelect>
		<ShadcnUiButton
			variant="ghost-plain"
			size="custom"
			class="w-fit"
			@click="copyCodeBlock"
		>
			<Icon
				name="lucide:copy"
				class="transition-colors duration-300"
				:class="{ 'text-chart-2': isCopied }"
			/>
		</ShadcnUiButton>
	</div>
</template>
