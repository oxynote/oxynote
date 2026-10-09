<script lang="ts" setup>
import { nodeViewProps } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"
import {
	defaultExtendedCodeBlockLanguage,
	extendedCodeBlockLanguageOptions,
} from "./languages"
import { isLanguageDetected, type CodeBlockOptions } from "./index"
import { detectLanguage } from "./detection"
import { isChangeOrigin } from "@tiptap/extension-collaboration"
import type { Transaction } from "@tiptap/pm/state"

// detection runs every grammar over the code, so it waits until the code
// stops changing
const STORE_DELAY_MS = 500

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

const currentLang = computed({
	get: () => {
		// a block the detector has not stored yet shows its own guess
		if (!props.node.attrs.language) {
			return detectedLanguage.value
		}

		return props.node.attrs.language as string
	},
	set: (lang: string) => {
		// picking a language turns detection off, even when it is the
		// detected one
		if (props.node.attrs.language !== lang || props.node.attrs.auto) {
			props.updateAttributes({ language: lang, auto: false })
		}
	},
})

let storeTimeout: ReturnType<typeof setTimeout> | undefined

onBeforeMount(() => {
	updateDetectedLanguage()
})
onMounted(() => {
	props.editor.on("update", onEditorUpdate)
})
onUnmounted(() => {
	props.editor.off("update", onEditorUpdate)
	clearTimeout(storeTimeout)
})

function updateDetectedLanguage() {
	if (props.node.attrs.language) {
		return
	}

	detectedLanguage.value = detectLanguage(props.node.textContent)
}

function onEditorUpdate({ transaction }: { transaction: Transaction }) {
	updateDetectedLanguage()

	// only the user typing in the block stores its language, because the
	// server counts every write as an edit by its user
	if (isChangeOrigin(transaction) || !isCaretInside()) {
		return
	}

	clearTimeout(storeTimeout)
	storeTimeout = setTimeout(storeDetectedLanguage, STORE_DELAY_MS)
}

function isCaretInside(): boolean {
	const pos = props.getPos()
	if (typeof pos !== "number") {
		return false
	}

	const { from, to } = props.editor.state.selection

	return from >= pos && to <= pos + props.node.nodeSize
}

function storeDetectedLanguage() {
	const pos = props.getPos()
	if (typeof pos !== "number" || !isLanguageDetected(props.node)) {
		return
	}

	const language = detectLanguage(props.node.textContent)
	if (props.node.attrs.language === language && props.node.attrs.auto) {
		return
	}

	// the write stays out of the undo history. A tracked write after an
	// undo would clear what can be redone.
	props.editor
		.chain()
		.command(({ tr }) => {
			tr.setNodeAttribute(pos, "language", language)
			tr.setNodeAttribute(pos, "auto", true)
			tr.setMeta("addToHistory", false)

			return true
		})
		.run()
}

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
