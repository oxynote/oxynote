<script setup lang="ts">
import { EditorContent, useEditor } from "@tiptap/vue-3"
import { CommentClass, CommentExtensions } from "./utils"
import { cn } from "~/lib/utils"

const props = defineProps<{
	content: Record<string, any>
}>()

const editor = useEditor({
	editorProps: {
		attributes: {
			class: cn(CommentClass),
			spellcheck: "false",
		},
	},
	content: props.content,
	extensions: CommentExtensions,
	editable: false,
})

// Update editor content when prop changes (needed for virtual scroller item recycling)
watchDeep(
	() => props.content,
	(newContent) => {
		if (editor.value && newContent) {
			editor.value.commands.setContent(newContent)
		}
	},
)
</script>

<template>
	<EditorContent v-if="editor" :editor="editor" />
</template>
