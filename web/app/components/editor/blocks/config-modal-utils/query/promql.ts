import {
	newCompleteStrategy,
	PromQLExtension,
	type CompleteStrategy,
} from "@prometheus-io/codemirror-promql"
import type {
	Completion,
	CompletionResult,
	CompletionContext,
} from "@codemirror/autocomplete"
import { parser } from "@prometheus-io/lezer-promql"
import type { EditorState } from "@codemirror/state"
import { resolveTimeRange, TimeRangePreset } from "~/utils/time-range"
import type { VariableTreeResponse } from "~/utils"
import { referencedVariableValues } from "~/utils/variable"
import {
	TYPED_VARIABLE_PATTERN,
	variableCompletions,
} from "./variable-completion"

const MATCHER_PROCESSING_DEBOUNCE_MS = 700
const PROMQL_DURATION_UNITS = new Set(["ms", "s", "m", "h", "d", "w", "y"])

function promqlDurationPresets(t: (key: string) => string) {
	// NOTE: i18n here is static
	return [
		{
			label: "$__interval",
			type: "variable",
			info: t("editor.config-modal-utils.query-info.prometheus.interval"),
		},
		{
			label: "$__range",
			type: "variable",
			info: t("editor.config-modal-utils.query-info.prometheus.range"),
		},
		{
			label: "$__rate_interval",
			type: "variable",
			info: t("editor.config-modal-utils.query-info.prometheus.rate-interval"),
		},
		{ label: "1m", type: "constant" },
		{ label: "5m", type: "constant" },
		{ label: "10m", type: "constant" },
		{ label: "30m", type: "constant" },
		{ label: "1h", type: "constant" },
		{ label: "1d", type: "constant" },
	].map((c, i) => ({
		...c,
		boost: 99 - i, // preserves array order
	}))
}

function bracketContext(ctx: CompletionContext) {
	const { pos, state } = ctx
	const before = state.sliceDoc(0, pos)

	const open = before.lastIndexOf("[")
	const close = before.lastIndexOf("]")
	if (open < 0 || close > open) {
		return null
	}

	// token inside brackets (anchor for completion range)
	const m = ctx.matchBefore(/[\w$]+$/)
	const from = m ? Math.max(m.from, open + 1) : pos
	const to = pos
	const token = state.sliceDoc(from, to)

	return { open, from, to, token }
}

// acceptsVariable tells whether the grammar takes a variable in place of
// the text between from and to. It parses the query with a stand-in
// variable there: a slot that takes a value, a label name or a metric name
// parses it, while a keyword or an operator position turns it into an
// error. A string takes any text, so it takes a variable too.
function acceptsVariable(state: EditorState, from: number, to: number) {
	const probe = `${state.sliceDoc(0, from)}$x${state.sliceDoc(to)}`
	const node = parser.parse(probe).resolveInner(from + 1, 1)
	if (node.name === "StringLiteral") {
		return true
	}

	return node.name === "VariableReference" && !node.parent?.type.isError
}

// variableSlot is where a variable would go at the cursor: over the $key
// typed so far, or at the cursor before anything is typed. Null when the
// cursor is in the middle of another word or the grammar has no place for
// a variable there.
function variableSlot(ctx: CompletionContext) {
	const typed = ctx.matchBefore(TYPED_VARIABLE_PATTERN)
	if (!typed && ctx.matchBefore(/[\w:]+$/)) {
		return null
	}

	const from = typed?.from ?? ctx.pos
	if (!acceptsVariable(ctx.state, from, ctx.pos)) {
		return null
	}

	return { from: from, typed: !!typed }
}

function mergeDedupByLabel(...lists: (readonly Completion[])[]) {
	const seen = new Set<string>()
	const res: Completion[] = []
	for (const list of lists) {
		for (const c of list) {
			if (seen.has(c.label)) {
				continue
			}

			seen.add(c.label)
			res.push(c)
		}
	}

	return res
}

function makeWrappedCompleteStrategy(
	base: CompleteStrategy,
	t: (key: string) => string,
	variables: MaybeRefOrGetter<VariableTreeResponse>,
): CompleteStrategy {
	return {
		promQL: async (
			ctx: CompletionContext,
		): Promise<CompletionResult | null> => {
			const res = await base.promQL(ctx)
			const slot = variableSlot(ctx)
			const variableOptions = slot
				? variableCompletions(toValue(variables), t)
				: []
			const bc = bracketContext(ctx)

			// a typed $key only ever matches a variable, so the variables
			// replace whatever the base offers for the same text
			if (!bc && slot?.typed) {
				return {
					from: slot.from,
					to: ctx.pos,
					options: variableOptions,
					validFor: TYPED_VARIABLE_PATTERN,
				}
			}

			if (!res) {
				return res
			}

			// Outside [...] → return PromQL results with the variables added
			if (!bc) {
				return {
					...res,
					options: mergeDedupByLabel(res.options, variableOptions),
				}
			}

			// never allow unit-only duration suffix completions inside [...]
			const filtered = res.options.filter(
				(o) => !PROMQL_DURATION_UNITS.has(o.label),
			)

			//  show presets when empty, when typing "$", or when typing a number
			const showPresets =
				bc.token.length === 0 ||
				bc.token.startsWith("$") ||
				/^\d/.test(bc.token)

			const presets = showPresets ? promqlDurationPresets(t) : []

			return {
				...res,
				from: bc.from, // keep tooltip anchored correctly inside [...]
				to: bc.to,
				options: mergeDedupByLabel(presets, variableOptions, filtered),
				validFor: /^[\w$]*$/,
			}
		},
	}
}

export function usePromQLQueryExtension(
	t: (key: string) => string,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	enable: MaybeRefOrGetter<boolean>,
	variables: MaybeRefOrGetter<VariableTreeResponse>,
) {
	const {
		usePrometheusMetadata,
		usePrometheusLabels,
		usePrometheusLabelValues,
		usePrometheusSeries,
	} = usePrometheusDataSourceAPI()
	const fetchPrometheusMetadata = usePrometheusMetadata(dataSourceId, enable)

	const lastFetchPrometheusLabelsParams = ref<
		PrometheusLabelParams | null | undefined
	>(null)
	const fetchPrometheusLabels = usePrometheusLabels(
		dataSourceId,
		lastFetchPrometheusLabelsParams,
		enable,
	)

	const lastFetchPrometheusLabelValuesParams = ref<
		PrometheusLabelValuesParams | null | undefined
	>(null)
	const fetchPrometheusLabelValues = usePrometheusLabelValues(
		dataSourceId,
		lastFetchPrometheusLabelValuesParams,
		enable,
	)

	const lastFetchPrometheusSeriesParams = ref<
		PrometheusSeriesParams | null | undefined
	>(null)
	const fetchPrometheusSeries = usePrometheusSeries(
		dataSourceId,
		lastFetchPrometheusSeriesParams,
		enable,
	)

	// a lookup sends the values of the variables its matchers reference,
	// for the server to substitute
	function withVariableValues<T extends { matchers?: string[] }>(params: T): T {
		return {
			...params,
			variableValues: referencedVariableValues(
				toValue(variables),
				params.matchers ?? [],
			),
		}
	}

	const basePromQL = newCompleteStrategy({
		remote: new PrometheusDataSourceClient({
			timeRangeFn: () => {
				return Promise.resolve(
					toValue(timeRange) || TimeRangePreset.Last5Minutes,
				)
			},
			metricMetadataFn: async () => {
				return (
					(await fetchPrometheusMetadata.refresh()).data ?? {
						result: {},
					}
				)
			},
			labelNamesFn: async (params: PrometheusLabelParams) => {
				lastFetchPrometheusLabelsParams.value = withVariableValues(params)
				return (
					(await fetchPrometheusLabels.refresh()).data ?? {
						result: [],
					}
				)
			},
			labelValuesFn: async (params: PrometheusLabelValuesParams) => {
				lastFetchPrometheusLabelValuesParams.value = withVariableValues(params)
				return (
					(await fetchPrometheusLabelValues.refresh()).data ?? {
						result: [],
					}
				)
			},
			seriesFn: async (params: PrometheusSeriesParams) => {
				lastFetchPrometheusSeriesParams.value = withVariableValues(params)
				return (
					(await fetchPrometheusSeries.refresh()).data ?? {
						result: [],
					}
				)
			},
		}),
	})

	return {
		extensions: computed(() => [
			new PromQLExtension()
				.activateLinter(false)
				.setComplete({
					completeStrategy: makeWrappedCompleteStrategy(
						basePromQL,
						t,
						variables,
					),
				})
				.asExtension(),
		]),
	}
}

export function usePromQLLegendExtension(
	query: MaybeRefOrGetter<string | null | undefined>,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	enable: MaybeRefOrGetter<boolean>,
	variables: MaybeRefOrGetter<VariableTreeResponse>,
) {
	const matchers = computed(() => {
		const q = toValue(query)?.trim()
		if (!q) {
			return []
		}

		return extractPromQLSelectors(q)
	})
	const debouncedMatchers = refDebounced(
		matchers,
		MATCHER_PROCESSING_DEBOUNCE_MS,
	)

	const { usePrometheusLabels, usePrometheusSeries } =
		usePrometheusDataSourceAPI()
	const fetchPrometheusLabels = usePrometheusLabels(
		dataSourceId,
		() => {
			const timeRangeVal = toValue(timeRange)

			if (!timeRangeVal || debouncedMatchers.value.length === 0) {
				return null
			}

			const resolvedTimeRange = resolveTimeRange(timeRangeVal)

			return {
				from: resolvedTimeRange.from,
				to: resolvedTimeRange.to,
				timeRangeKey: timeRangeVal,
				matchers: debouncedMatchers.value,
				variableValues: referencedVariableValues(
					toValue(variables),
					debouncedMatchers.value,
				),
			}
		},
		enable,
		30 * 1000, // 30 secs
	)
	const fetchPrometheusSeries = usePrometheusSeries(
		dataSourceId,
		() => {
			const timeRangeVal = toValue(timeRange)

			if (!timeRangeVal || debouncedMatchers.value.length === 0) {
				return null
			}

			const resolvedTimeRange = resolveTimeRange(timeRangeVal)

			return {
				from: resolvedTimeRange.from,
				to: resolvedTimeRange.to,
				timeRangeKey: timeRangeVal,
				matchers: debouncedMatchers.value,
				variableValues: referencedVariableValues(
					toValue(variables),
					debouncedMatchers.value,
				),
			}
		},
		enable,
		30 * 1000, // 30 secs
	)

	async function fetchAllLabelNames(): Promise<string[]> {
		const res = await fetchPrometheusLabels.refresh()

		// drop internal labels (Prometheus reserves __* labels for itself)
		return res.data?.result.filter((n) => !n.startsWith("__")) ?? []
	}

	async function fetchExampleLabelValues(): Promise<Record<string, string>> {
		const res = await fetchPrometheusSeries.refresh()
		if (!res.data?.result.length) {
			return {}
		}

		const firstSeries = res.data.result[0]

		return firstSeries ?? {}
	}

	return {
		fetchAllLabelNames,
		fetchExampleLabelValues,
	}
}
