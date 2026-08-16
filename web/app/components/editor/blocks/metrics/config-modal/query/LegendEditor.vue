<script lang="ts" setup>
import {
	codemirrorTheme,
	singleLineExtension,
	trimEditorWhitespace,
	useLegendEditorExtension,
} from "./editor"
import CodeMirror from "vue-codemirror6"
import { githubLight, githubDark } from "@uiw/codemirror-theme-github"
import { minimalSetup } from "codemirror"
import { type EditorView, keymap, tooltips } from "@codemirror/view"
import {
	bracketMatching,
	LanguageSupport,
	StreamLanguage,
} from "@codemirror/language"
import {
	autocompletion,
	closeBrackets,
	startCompletion,
	type CompletionContext,
	type Completion,
	closeCompletion,
} from "@codemirror/autocomplete"
import { tags } from "@lezer/highlight"
import { cn } from "~/lib/utils"
import type { TimeRangePreset } from "../../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps<{
	diffStatus?: DiffStatus | null
	dataSourceId: string | null | undefined // null disables the editor (it affects the autocompletions)
	timeRange: TimeRangePreset | null | undefined
	query: string | null | undefined
}>()
const legend = defineModel<string>({ default: "" })

const { isDark } = useAppearance()
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()
const legendEditorElem = useTemplateRef("legend-editor")
const { fetchAllLabelNames, fetchExampleLabelValues } =
	useLegendEditorExtension(
		() => props.query,
		() => props.dataSourceId,
		() => props.timeRange,
	)

const legendFormatLanguage = StreamLanguage.define<{ inBraces: boolean }>({
	name: "legend-formatting-syntax",
	startState: () => ({ inBraces: false }),
	token(stream, state) {
		if (!state.inBraces && stream.match("{{")) {
			state.inBraces = true
			return "brace"
		}

		if (state.inBraces && stream.match("}}")) {
			state.inBraces = false
			return "brace"
		}

		if (state.inBraces) {
			if (stream.eatWhile(/\s/)) {
				return null
			}

			if (stream.match(/[a-zA-Z_][\w:]*/)) {
				return "label"
			}

			stream.next()

			return null
		}

		stream.next()

		return null
	},

	tokenTable: {
		brace: tags.brace,
		label: tags.variableName,
	},
})

const isEditingDisabled = computed(
	() => !isEditable.value || editorStore.reviewableDiffActive,
)
const extensions = computed(() => [
	codemirrorTheme,
	minimalSetup,
	new LanguageSupport(legendFormatLanguage),
	bracketMatching(),
	closeBrackets(),
	tooltips({ parent: document.body }),
	autocompletion({
		override: [legendLabelCompletionSource()],
		closeOnBlur: false,
	}),
	keymap.of([
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
	singleLineExtension(),
])
let wasFocusedAtLeastOnce = false

function legendLabelCompletionSource() {
	return async (ctx: CompletionContext) => {
		const pos = ctx.pos
		const before = ctx.state.doc.sliceString(0, pos)

		const open = before.lastIndexOf("{{")
		const close = before.lastIndexOf("}}")

		if (open < 0 || close > open) {
			return null // not inside
		}

		// figure out current partial word
		let posIndex = pos
		while (posIndex > open + 2) {
			const ch = before.charAt(posIndex - 1)
			if (!/[\w:]/.test(ch)) {
				break
			}

			posIndex--
		}

		const from = posIndex
		const to = pos

		const isBeforeClosingBraces = ctx.state.sliceDoc(pos, pos + 2) === "}}"
		if (!ctx.explicit && from === to && !isBeforeClosingBraces) {
			return null
		}

		const [labels, exampleValues] = await Promise.all([
			fetchAllLabelNames(),
			fetchExampleLabelValues(),
		])

		const options: Completion[] = labels.map((name) => ({
			label: name,
			type: "variable",
			// NOTE: this doesn't use i18n because this isn't a reactive context
			detail: exampleValues[name] ? `ex. ${exampleValues[name]}` : undefined,
			apply: (view, completion, f, tPos) => {
				const hasClosingBraces = view.state.sliceDoc(tPos, tPos + 2) === "}}"
				const insertText = completion.label + (hasClosingBraces ? "" : "}}")
				const cursorPos = f + completion.label.length

				view.dispatch({
					changes: { from: f, to: tPos, insert: insertText },
					selection: { anchor: cursorPos },
					scrollIntoView: true,
				})
			},
		}))

		return {
			from,
			to,
			options,
			validFor: /^[\w:]*$/,
		}
	}
}

function handleFocusChange(focused: boolean) {
	if (focused) {
		wasFocusedAtLeastOnce = true
	} else if (wasFocusedAtLeastOnce) {
		try {
			const view = legendEditorElem.value?.view as EditorView | undefined
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
	}
}
</script>
<template>
	<CodeMirror
		ref="legend-editor"
		v-model="legend"
		:class="
			cn(
				'legend-editor pointer-events-auto rounded-md border border-border opacity-100 transition-opacity duration-150',
				(!props.dataSourceId || !props.timeRange) && 'opacity-50',
				isEditingDisabled && 'legend-editor--readonly',
				props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
				props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
			)
		"
		wrap
		:readonly="isEditingDisabled"
		:disabled="!props.dataSourceId || !props.timeRange"
		:placeholder="
			isEditingDisabled
				? $t('editor.metrics.config.legend-format-empty-value-placeholder')
				: $t('editor.metrics.config.legend-format-placeholder')
		"
		:extensions="[isDark ? githubDark : githubLight, ...extensions]"
		@focus="handleFocusChange"
	/>
</template>

<style>
.legend-editor--readonly .cm-cursor {
	border-left-color: transparent !important;
}
</style>
