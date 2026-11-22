import { EditorState } from "@codemirror/state"
import type {
	CompletionContext,
	CompletionResult,
} from "@codemirror/autocomplete"
import type { VariableTreeResponse } from "~/utils"
import {
	TYPED_VARIABLE_PATTERN,
	variableCompletions,
} from "./variable-completion"

// TODO: parse LogsQL once its grammar exists. Until then the query is
// plain text. It has no highlighting and no field or value completions,
// and only the variables complete.
export function useLogsQLQueryExtension(
	t: (key: string) => string,
	variables: MaybeRefOrGetter<VariableTreeResponse>,
) {
	// a typed $key completes as it is written. Anywhere else the variables
	// are offered only when asked for, and not in the middle of a word.
	function complete(ctx: CompletionContext): CompletionResult | null {
		const typed = ctx.matchBefore(TYPED_VARIABLE_PATTERN)
		if (!typed && (!ctx.explicit || ctx.matchBefore(/\w+$/))) {
			return null
		}

		return {
			from: typed?.from ?? ctx.pos,
			options: variableCompletions(toValue(variables), t),
			validFor: typed ? TYPED_VARIABLE_PATTERN : undefined,
		}
	}

	return {
		extensions: computed(() => [
			EditorState.languageData.of(() => [
				{ autocomplete: complete, commentTokens: { line: "#" } },
			]),
		]),
	}
}
