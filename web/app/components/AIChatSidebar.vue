<script lang="ts" setup>
import type { Editor } from "@tiptap/core"

defineProps<{
	contentEditor: Editor | null
	nameEditor: Editor | null
}>()

const { t } = useI18n({ useScope: "global" })
const editorStore = useEditorStore()
const isMobile = useMediaQuery("(max-width: 1250px)")
</script>
<template>
	<ShadcnUiSheet
		v-if="isMobile"
		:open="editorStore.aiAssistantOpen"
		@update:open="
			(value) => (value ? null : editorStore.toggleAiAssistantOpen())
		"
	>
		<ShadcnUiSheetContent side="right" class="w-fit p-0">
			<ShadcnUiSheetHeader class="sr-only">
				<ShadcnUiSheetTitle>{{ t("editor.ai-chat.title") }}</ShadcnUiSheetTitle>
				<ShadcnUiSheetDescription>
					{{ t("editor.ai-chat.sheet-description") }}
				</ShadcnUiSheetDescription>
			</ShadcnUiSheetHeader>
			<EditorAiChatBox
				class="w-80"
				mobile
				:content-editor="contentEditor"
				:name-editor="nameEditor"
				@close-chat-box="editorStore.toggleAiAssistantOpen()"
			/>
		</ShadcnUiSheetContent>
	</ShadcnUiSheet>
	<div
		v-else
		class="sticky top-0 h-svh shrink-0 overflow-hidden border-l border-border bg-sidebar transition-[width] duration-200 ease-out"
		:style="{ width: editorStore.aiAssistantOpen ? '30rem' : '0rem' }"
	>
		<EditorAiChatBox
			class="w-120"
			:content-editor="contentEditor"
			:name-editor="nameEditor"
		/>
	</div>
</template>
