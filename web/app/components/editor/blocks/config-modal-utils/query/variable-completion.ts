import type { Completion } from "@codemirror/autocomplete"
import type { VariableTreeResponse } from "~/utils"
import { resolvedVariableValue } from "~/utils/variable"

// a variable's key as typed so far, from its $ on
export const TYPED_VARIABLE_PATTERN = /\$[a-z0-9_]*$/

// variableCompletions offers each variable as its $key, with its type
// beside it. The info holds the description, when there is one, and the
// variable's current value.
export function variableCompletions(
	tree: VariableTreeResponse,
	t: (key: string, named?: Record<string, unknown>) => string,
): Completion[] {
	return tree
		.flatMap((group) => group.variables)
		.map((variable) => {
			const current = t("editor.config-modal-utils.query-info.variable", {
				value: resolvedVariableValue(variable) ?? "",
			})

			return {
				label: `$${variable.key}`,
				detail: t(`general.variable-types.${variable.type}`),
				type: "variable",
				info: variable.description
					? `${variable.description}\n\n${current}`
					: current,
			}
		})
}
