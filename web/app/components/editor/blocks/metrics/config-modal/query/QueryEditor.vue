<script lang="ts" setup>
import CodeMirror from "vue-codemirror6"
import { githubLight, githubDark } from "@uiw/codemirror-theme-github"
import { EditorState } from "@codemirror/state"
import { bracketMatching, indentUnit } from "@codemirror/language"
import {
	closeBrackets,
	autocompletion,
	closeCompletion,
	startCompletion,
} from "@codemirror/autocomplete"
import { type EditorView, keymap, tooltips } from "@codemirror/view"
import { minimalSetup } from "codemirror"
import { cn } from "~/lib/utils"
import {
	codemirrorTheme,
	trimEditorWhitespace,
	useQueryEditorExtension,
} from "./editor"
import type { TimeRangePreset } from "../../utils"
import { indentWithTab } from "@codemirror/commands"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps<{
	diffStatus?: DiffStatus | null
	dataSourceId: string | null | undefined // null disables the editor (it affects the autocompletions)
	timeRange: TimeRangePreset | null | undefined
}>()
const emit = defineEmits<{
	(event: "focus-change", focused: boolean): void
}>()

const query = defineModel<string>({ default: "" })
const queryEditorElem = useTemplateRef("query-editor")
const { isDark } = useAppearance()
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const { t } = useI18n({ useScope: "global" })

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
const queryExtension = useQueryEditorExtension(
	t,
	() => props.dataSourceId,
	() => props.timeRange,
	() => !isEditingDisabled.value,
)

const extensions = computed(() => {
	const res = [
		codemirrorTheme,
		minimalSetup,
		bracketMatching(),
		closeBrackets(),
		tooltips({ parent: document.body }),
		autocompletion({
			closeOnBlur: false,
		}),
		indentUnit.of("\t"),
		EditorState.tabSize.of(8),
		keymap.of([
			indentWithTab,
			{
				key: normalizeShortcut(
					SHORTCUT_ACTIONS.openEditorCompletionMenu.keyboardKey,
					HostOsType.Other,
					"codemirror",
				),
				mac: normalizeShortcut(
					SHORTCUT_ACTIONS.openEditorCompletionMenu.keyboardKey,
					HostOsType.MacOS,
					"codemirror",
				),
				run: startCompletion,
				preventDefault: true,
			},
		]),
	]
	if (queryExtension.extensions.value) {
		res.push(...queryExtension.extensions.value)
	}

	return res
})
let wasFocusedAtLeastOnce = false

function handleFocusChange(focused: boolean) {
	if (focused) {
		wasFocusedAtLeastOnce = true
		emit("focus-change", true)
	} else if (wasFocusedAtLeastOnce) {
		try {
			const view = queryEditorElem.value?.view as EditorView | undefined
			if (view) {
				closeCompletion(view) // close autocomplete tooltip
				trimEditorWhitespace(view)

				// clear selection by collapsing to cursor position
				view.dispatch({
					selection: { anchor: view.state.selection.main.head },
				})
			}
		} catch {
			// Ignore errors when accessing the view during blur
			// This can happen when the editor is unmounting or in a
			// transitional state
		}

		emit("focus-change", false)
	}
}
</script>
<template>
	<CodeMirror
		ref="query-editor"
		v-model="query"
		:class="
			cn(
				'query-editor pointer-events-auto rounded-md border border-border opacity-100 transition-opacity duration-150',
				(!props.dataSourceId || !props.timeRange) && 'opacity-50',
				isEditingDisabled && 'query-editor--readonly',
				props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
				props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
			)
		"
		wrap
		:readonly="isEditingDisabled"
		:disabled="!props.dataSourceId || !props.timeRange"
		:placeholder="queryExtension.placeholder.value"
		:extensions="[isDark ? githubDark : githubLight, ...extensions]"
		@focus="handleFocusChange"
	/>
</template>

<style>
.query-editor--readonly .cm-cursor {
	border-left-color: transparent !important;
}
</style>
