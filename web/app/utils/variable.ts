import type { VariableTreeElement, VariableTreeResponse } from "./api/variable"
import type { RefreshInterval, TimeRangePreset } from "./time-range"

// the shape every variable key has
export const KEY_PATTERN = /^[a-z][a-z0-9_]*$/

// a $ and the whole word after it, the same shape the server substitutes.
// The word names a variable only when it is one of the keys, so a macro
// such as $__from never does.
const REFERENCE_PATTERN = /\$([A-Za-z_][A-Za-z0-9_]*)/g

// the built-in variables a metric or log block can follow instead of a
// fixed time range or refresh interval. Built-in keys never change.
export const METRIC_TIME_RANGE_REFERENCE = "$metric_range"
export const METRIC_REFRESH_INTERVAL_REFERENCE = "$metric_refresh"

// a block either takes a fixed setting or follows the built-in variable
export type TimeRangeSetting =
	TimeRangePreset | typeof METRIC_TIME_RANGE_REFERENCE
export type RefreshIntervalSetting =
	RefreshInterval | typeof METRIC_REFRESH_INTERVAL_REFERENCE

// referencedWords lists the words the text writes as $word, each once
export function referencedWords(text: string): string[] {
	const words = new Set<string>()

	for (const match of text.matchAll(REFERENCE_PATTERN)) {
		if (match[1]) {
			words.add(match[1])
		}
	}

	return [...words]
}

export function variableByReference(
	tree: VariableTreeResponse,
	reference: string,
): VariableTreeElement | undefined {
	return tree
		.flatMap((group) => group.variables)
		.find((v) => `$${v.key}` === reference)
}

// resolvedVariableValue is what a query gets in place of the reference:
// the user's own value, else the default. A list gives its option's text.
export function resolvedVariableValue(
	variable: VariableTreeElement,
): string | null {
	const value = variable.value ?? variable.defaultValue
	if (variable.type !== "list") {
		return value
	}

	return variable.options.find((o) => o.id === value)?.value ?? null
}

// referencedVariableValues maps each key the texts reference to the value
// the server substitutes for it
export function referencedVariableValues(
	tree: VariableTreeResponse,
	texts: string[],
): Record<string, string> {
	const words = new Set(texts.flatMap(referencedWords))
	const values: Record<string, string> = {}

	tree
		.flatMap((group) => group.variables)
		.filter((v) => words.has(v.key))
		.forEach((v) => {
			values[v.key] = resolvedVariableValue(v) ?? ""
		})

	return values
}
