<script lang="ts" setup>
import { Editor, EditorContent } from "@tiptap/vue-3"
import { cn } from "@/lib/utils"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type * as Y from "yjs"
import { yXmlFragmentToProseMirrorRootNode } from "y-prosemirror"
import {
	DiffTextAddedMark,
	DiffTextRemovedMark,
} from "~/components/editor/diff/diff-text-marks"

const props = defineProps<{
	targetBranchYdoc: Y.Doc
	activeBranchYdoc: Y.Doc
}>()

const editor = shallowRef<Editor | null>(null)

const editorInstance = new Editor({
	editable: false,
	extensions: [
		Document,
		Text,
		Paragraph.configure({
			HTMLAttributes: {
				class: cn("break-all whitespace-normal"),
			},
		}),
		DiffTextAddedMark,
		DiffTextRemovedMark,
	],
	editorProps: {
		attributes: {
			class: cn(
				"focus:outline-none w-full max-w-none",
				"text-2xl font-semibold",
			),
			spellcheck: "false",
		},
	},
})
editor.value = editorInstance

const targetBranchTitle = ref(extractText(props.targetBranchYdoc))
const activeBranchTitle = ref(extractText(props.activeBranchYdoc))
const { diffContent } = useDiffTitle(targetBranchTitle, activeBranchTitle)

const onTargetBranchUpdate = () => recompute()
const onActiveBranchUpdate = () => recompute()

props.targetBranchYdoc.on("update", onTargetBranchUpdate)
props.activeBranchYdoc.on("update", onActiveBranchUpdate)

onBeforeUnmount(() => {
	props.targetBranchYdoc.off("update", onTargetBranchUpdate)
	props.activeBranchYdoc.off("update", onActiveBranchUpdate)
	editor.value?.destroy()
	editor.value = null
})

watch(editor, (e) => {
	if (e) {
		recompute()
	}
})

watchImmediate(diffContent, (content) => {
	editor.value?.commands.setContent(content)
})

function extractText(ydoc: Y.Doc): string {
	if (!editor.value) {
		return ""
	}

	try {
		const fragment = ydoc.getXmlFragment("name")
		if (fragment.length === 0) {
			return ""
		}

		return yXmlFragmentToProseMirrorRootNode(fragment, editor.value.schema)
			.textContent
	} catch {
		return ""
	}
}

function recompute() {
	targetBranchTitle.value = extractText(props.targetBranchYdoc)
	activeBranchTitle.value = extractText(props.activeBranchYdoc)
}
</script>
<template>
	<EditorContent v-if="editor" :editor="editor" class="diff-title" />
</template>
