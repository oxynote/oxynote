import { LanguageType, promQLLanguage } from "@prometheus-io/codemirror-promql"
import type {
	Completion,
	CompletionContext,
	CompletionResult,
} from "@codemirror/autocomplete"
import type { Tree } from "@lezer/common"
import { metricsqlParser } from "~/utils/promql"

// the functions and aggregations MetricsQL has, in the groups its
// documentation uses: https://docs.victoriametrics.com/victoriametrics/metricsql/
const ROLLUP_FUNCTIONS = [
	"absent_over_time",
	"aggr_over_time",
	"ascent_over_time",
	"avg_over_time",
	"changes",
	"changes_prometheus",
	"count_eq_over_time",
	"count_gt_over_time",
	"count_le_over_time",
	"count_ne_over_time",
	"count_over_time",
	"count_values_over_time",
	"decreases_over_time",
	"default_rollup",
	"delta",
	"delta_prometheus",
	"deriv",
	"deriv_fast",
	"descent_over_time",
	"distinct_over_time",
	"duration_over_time",
	"first_over_time",
	"geomean_over_time",
	"histogram_over_time",
	"hoeffding_bound_lower",
	"hoeffding_bound_upper",
	"holt_winters",
	"idelta",
	"ideriv",
	"increase",
	"increase_prometheus",
	"increase_pure",
	"increases_over_time",
	"integrate",
	"irate",
	"lag",
	"last_over_time",
	"lifetime",
	"mad_over_time",
	"max_over_time",
	"median_over_time",
	"min_over_time",
	"mode_over_time",
	"outlier_iqr_over_time",
	"predict_linear",
	"present_over_time",
	"quantile_over_time",
	"quantiles_over_time",
	"range_over_time",
	"rate",
	"rate_prometheus",
	"rate_over_sum",
	"resets",
	"rollup",
	"rollup_candlestick",
	"rollup_delta",
	"rollup_deriv",
	"rollup_increase",
	"rollup_rate",
	"rollup_scrape_interval",
	"scrape_interval",
	"share_eq_over_time",
	"share_gt_over_time",
	"share_le_over_time",
	"stale_samples_over_time",
	"stddev_over_time",
	"stdvar_over_time",
	"sum_eq_over_time",
	"sum_gt_over_time",
	"sum_le_over_time",
	"sum_over_time",
	"sum2_over_time",
	"tfirst_over_time",
	"timestamp",
	"timestamp_with_name",
	"tlast_change_over_time",
	"tlast_over_time",
	"tmax_over_time",
	"tmin_over_time",
	"zscore_over_time",
] as const

const TRANSFORM_FUNCTIONS = [
	"abs",
	"absent",
	"acos",
	"acosh",
	"asin",
	"asinh",
	"atan",
	"atanh",
	"bitmap_and",
	"bitmap_or",
	"bitmap_xor",
	"buckets_limit",
	"ceil",
	"clamp",
	"clamp_max",
	"clamp_min",
	"cos",
	"cosh",
	"day_of_month",
	"day_of_week",
	"day_of_year",
	"days_in_month",
	"deg",
	"drop_empty_series",
	"end",
	"exp",
	"floor",
	"histogram_avg",
	"histogram_fraction",
	"histogram_quantile",
	"histogram_quantiles",
	"histogram_share",
	"histogram_stddev",
	"histogram_stdvar",
	"hour",
	"interpolate",
	"keep_last_value",
	"keep_next_value",
	"limit_offset",
	"ln",
	"log10",
	"log2",
	"minute",
	"month",
	"now",
	"pi",
	"prometheus_buckets",
	"rad",
	"rand",
	"rand_exponential",
	"rand_normal",
	"range_avg",
	"range_first",
	"range_last",
	"range_linear_regression",
	"range_mad",
	"range_max",
	"range_median",
	"range_min",
	"range_normalize",
	"range_quantile",
	"range_stddev",
	"range_stdvar",
	"range_sum",
	"range_trim_outliers",
	"range_trim_spikes",
	"range_trim_zscore",
	"range_zscore",
	"remove_resets",
	"round",
	"ru",
	"running_avg",
	"running_max",
	"running_min",
	"running_sum",
	"scalar",
	"sgn",
	"sin",
	"sinh",
	"smooth_exponential",
	"sort",
	"sort_desc",
	"sqrt",
	"start",
	"step",
	"tan",
	"tanh",
	"time",
	"timezone_offset",
	"ttf",
	"union",
	"vector",
	"year",
] as const

const LABEL_FUNCTIONS = [
	"alias",
	"drop_common_labels",
	"label_copy",
	"label_del",
	"label_graphite_group",
	"label_join",
	"label_keep",
	"label_lowercase",
	"label_map",
	"label_match",
	"label_mismatch",
	"label_move",
	"label_replace",
	"label_set",
	"label_transform",
	"label_uppercase",
	"label_value",
	"labels_equal",
	"sort_by_label",
	"sort_by_label_desc",
	"sort_by_label_numeric",
	"sort_by_label_numeric_desc",
] as const

const AGGREGATIONS = [
	"any",
	"avg",
	"bottomk",
	"bottomk_avg",
	"bottomk_last",
	"bottomk_max",
	"bottomk_median",
	"bottomk_min",
	"count",
	"count_values",
	"distinct",
	"geomean",
	"group",
	"histogram",
	"limitk",
	"mad",
	"max",
	"median",
	"min",
	"mode",
	"outliers_iqr",
	"outliers_mad",
	"outliersk",
	"quantile",
	"quantiles",
	"share",
	"stddev",
	"stdvar",
	"sum",
	"sum2",
	"topk",
	"topk_avg",
	"topk_last",
	"topk_max",
	"topk_median",
	"topk_min",
	"zscore",
] as const

const INFO_KEY = "query-languages.victoriametrics"

const KNOWN_NAMES = new Set<string>([
	...ROLLUP_FUNCTIONS,
	...TRANSFORM_FUNCTIONS,
	...LABEL_FUNCTIONS,
	...AGGREGATIONS,
])

// the keywords that may follow a whole expression, next to the binary
// operators the PromQL completion offers there
const AFTER_EXPRESSION_KEYWORDS = [
	"if",
	"ifnot",
	"default",
	"keep_metric_names",
]

// the language comes from codemirror-promql itself, so its completion reads
// the same syntax tree the editor holds
export const metricsqlLanguage = promQLLanguage(LanguageType.PromQL).configure(
	{ dialect: "metricsql" },
	"metricsql",
)

function descriptionKey(group: string, name: string) {
	return `${INFO_KEY}.${group}.${name.replaceAll("_", "-")}`
}

function namedCompletions(
	t: (key: string) => string,
	names: readonly string[],
	group: string,
	detail: string,
	type: string,
): Completion[] {
	return names.map((name) => ({
		label: name,
		detail: detail,
		info: t(descriptionKey(group, name)),
		type: type,
	}))
}

function functionCompletions(t: (key: string) => string): Completion[] {
	return [
		...namedCompletions(t, ROLLUP_FUNCTIONS, "rollup", "function", "function"),
		...namedCompletions(
			t,
			TRANSFORM_FUNCTIONS,
			"transform",
			"function",
			"function",
		),
		...namedCompletions(t, LABEL_FUNCTIONS, "label", "function", "function"),
		...namedCompletions(t, AGGREGATIONS, "aggregate", "aggregation", "keyword"),
	]
}

function keywordCompletion(t: (key: string) => string, name: string) {
	return {
		label: name,
		detail: "keyword",
		info: t(descriptionKey("keywords", name)),
		type: "keyword",
	}
}

// templateCompletions offers the names that the query's WITH bindings
// define, before the cursor.
function templateCompletions(
	t: (key: string) => string,
	tree: Tree,
	doc: string,
	pos: number,
): Completion[] {
	const names = new Set<string>()

	tree.iterate({
		enter: (node) => {
			if (node.name !== "WithBinding" || node.from >= pos) {
				return
			}

			const name = node.node.firstChild
			if (name) {
				names.add(doc.slice(name.from, name.to))
			}
		},
	})

	return [...names].map((name) => ({
		label: name,
		detail: "template",
		info: t(`${INFO_KEY}.template`),
		type: "variable",
	}))
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

// withMetricsQLCompletions turns the PromQL completion result into a
// MetricsQL one. Where functions go, it offers MetricsQL's functions and
// aggregations with their descriptions, drops the PromQL ones MetricsQL
// lacks, and adds WITH and the templates the query defines. After an
// expression it adds MetricsQL's operators, and after an aggregation's
// modifier it adds limit. On a name that is already called, which the
// PromQL completion does not handle, it offers the functions alone.
export function withMetricsQLCompletions(
	ctx: CompletionContext,
	res: CompletionResult | null,
	t: (key: string) => string,
): CompletionResult | null {
	const doc = ctx.state.doc.toString()
	const tree = metricsqlParser.parse(doc)
	const node = tree.resolveInner(ctx.pos, -1)

	if (node.name === "CallIdentifier") {
		return {
			from: node.from,
			to: node.to,
			options: functionCompletions(t),
			validFor: /^\w*$/,
		}
	}

	if (!res) {
		return res
	}

	const offersFunctions = res.options.some((o) => o.type === "function")
	const offersOperators = res.options.some((o) => o.label === "unless")
	const offersModifiers = res.options.some((o) => o.label === "without")
	if (!offersFunctions && !offersOperators && !offersModifiers) {
		return res
	}

	const kept = res.options.filter(
		(o) =>
			(o.detail !== "function" && o.detail !== "aggregation") ||
			KNOWN_NAMES.has(o.label),
	)
	const added: Completion[] = []
	if (offersFunctions) {
		added.push(
			...functionCompletions(t),
			keywordCompletion(t, "with"),
			...templateCompletions(t, tree, doc, ctx.pos),
		)
	}

	if (offersOperators) {
		added.push(
			...AFTER_EXPRESSION_KEYWORDS.map((name) => keywordCompletion(t, name)),
		)
	}

	if (offersModifiers) {
		added.push(keywordCompletion(t, "limit"))
	}

	return { ...res, options: mergeDedupByLabel(added, kept) }
}
