import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
	type RefreshIntervalSetting,
	referencedVariableValues,
	referencedWords,
	resolvedVariableValue,
	type TimeRangeSetting,
	variableByReference,
} from "~/utils/variable"

// the built-ins' seeded defaults, for when the variables cannot load
const FALLBACK_TIME_RANGE = TimeRangePreset.Last24Hours
const FALLBACK_REFRESH_INTERVAL = RefreshInterval.M5

// useMetricVariables resolves what a chart reads from the variables: the
// built-ins it follows and the values its queries reference. Each answer
// is null while the variables it needs are still loading, so a chart
// never queries with a value it is about to replace.
export function useMetricVariables() {
	const { fetchVariables } = useVariableAPI()
	const loading = computed(
		() =>
			!fetchVariables.data.value && fetchVariables.status.value === "pending",
	)

	function referenceValue(reference: string): string | null {
		const variable = variableByReference(
			fetchVariables.data.value ?? [],
			reference,
		)

		return variable ? resolvedVariableValue(variable) : null
	}

	function timeRange(value: TimeRangeSetting | null): TimeRangePreset | null {
		if (value !== METRIC_TIME_RANGE_REFERENCE) {
			return value
		}

		if (loading.value) {
			return null
		}

		const resolved = referenceValue(value)

		return (
			Object.values(TimeRangePreset).find((p) => p === resolved) ??
			FALLBACK_TIME_RANGE
		)
	}

	function refreshInterval(
		value: RefreshIntervalSetting | null,
	): RefreshInterval | null {
		if (value !== METRIC_REFRESH_INTERVAL_REFERENCE) {
			return value
		}

		if (loading.value) {
			return null
		}

		const resolved = referenceValue(value)

		return (
			Object.values(RefreshInterval).find((i) => i === resolved) ??
			FALLBACK_REFRESH_INTERVAL
		)
	}

	// queryValues maps each variable the queries reference to its value, or
	// is null while a reference may still turn out to name one
	function queryValues(queries: string[]): Record<string, string> | null {
		if (loading.value && queries.some((q) => referencedWords(q).length)) {
			return null
		}

		return referencedVariableValues(fetchVariables.data.value ?? [], queries)
	}

	return {
		variables: computed(() => fetchVariables.data.value ?? []),
		timeRange,
		refreshInterval,
		queryValues,
	}
}
