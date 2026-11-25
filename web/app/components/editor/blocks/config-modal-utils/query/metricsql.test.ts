import { describe, it } from "vitest"
import { EditorState } from "@codemirror/state"
import {
	CompletionContext,
	type Completion,
	type CompletionResult,
} from "@codemirror/autocomplete"
import { withMetricsQLCompletions } from "./metricsql"
import { interpolatingT } from "./test-helpers"

const t = interpolatingT

const METRIC: Completion = { label: "up", type: "constant" }
const PROMQL_RATE: Completion = {
	label: "rate",
	detail: "function",
	info: "Calculate the per-second rate",
	type: "function",
}
const PROMQL_ONLY_FUNCTION: Completion = {
	label: "info",
	detail: "function",
	info: "Adds data labels",
	type: "function",
}
const PROMQL_SUM: Completion = {
	label: "sum",
	detail: "aggregation",
	info: "Calculate sum over dimensions",
	type: "keyword",
}
const PROMQL_ONLY_AGGREGATION: Completion = {
	label: "limit_ratio",
	detail: "aggregation",
	info: "Sample a ratio of series",
	type: "keyword",
}

describe("withMetricsQLCompletions", () => {
	it("offers MetricsQL's functions and aggregations where a function goes", ({
		expect,
	}) => {
		const res = complete("ra", 2, [
			METRIC,
			PROMQL_RATE,
			PROMQL_ONLY_FUNCTION,
			PROMQL_SUM,
			PROMQL_ONLY_AGGREGATION,
		])

		const options = byLabel(res)

		expect(res?.from).toBe(0)
		expect.soft(options.get("median_over_time")).toEqual({
			label: "median_over_time",
			detail: "function",
			info: "query-languages.victoriametrics.rollup.median-over-time",
			type: "function",
		})
		expect
			.soft(options.get("rate")?.info)
			.toBe("query-languages.victoriametrics.rollup.rate")
		expect
			.soft(options.get("label_set")?.info)
			.toBe("query-languages.victoriametrics.label.label-set")
		expect
			.soft(options.get("range_median")?.info)
			.toBe("query-languages.victoriametrics.transform.range-median")
		expect.soft(options.get("topk_max")).toEqual({
			label: "topk_max",
			detail: "aggregation",
			info: "query-languages.victoriametrics.aggregate.topk-max",
			type: "keyword",
		})
		expect
			.soft(options.get("sum")?.info)
			.toBe("query-languages.victoriametrics.aggregate.sum")
		expect.soft(options.get("with")).toEqual({
			label: "with",
			detail: "keyword",
			info: "query-languages.victoriametrics.keywords.with",
			type: "keyword",
		})
		expect.soft(options.get("up")).toBe(METRIC)
		expect.soft(options.has("info")).toBe(false)
		expect.soft(options.has("limit_ratio")).toBe(false)
	})

	it("offers the WITH templates defined before the cursor", ({ expect }) => {
		const doc = 'WITH (f = {a="b"}, g = 1) f + '
		const res = complete(doc, doc.length, [PROMQL_RATE])

		const options = byLabel(res)

		expect.soft(options.get("f")).toEqual({
			label: "f",
			detail: "template",
			info: "query-languages.victoriametrics.template",
			type: "variable",
		})
		expect.soft(options.get("g")?.type).toBe("variable")
	})

	it("leaves out a WITH template defined after the cursor", ({ expect }) => {
		const res = complete(" WITH (late = 1) late", 0, [PROMQL_RATE])

		expect(byLabel(res).has("late")).toBe(false)
	})

	it("adds MetricsQL's operators where a binary operator goes", ({
		expect,
	}) => {
		const res = complete("up ", 3, [{ label: "and" }, { label: "unless" }])

		const options = byLabel(res)

		for (const name of ["if", "ifnot", "default", "keep_metric_names"]) {
			expect
				.soft(options.get(name)?.info)
				.toBe(
					`query-languages.victoriametrics.keywords.${name.replaceAll("_", "-")}`,
				)
		}

		expect.soft(options.has("unless")).toBe(true)
		expect.soft(options.has("median_over_time")).toBe(false)
	})

	it("adds limit where an aggregation modifier goes", ({ expect }) => {
		const res = complete("sum(up) ", 8, [{ label: "by" }, { label: "without" }])

		const options = byLabel(res)

		expect
			.soft(options.get("limit")?.info)
			.toBe("query-languages.victoriametrics.keywords.limit")
		expect.soft(options.has("without")).toBe(true)
		expect.soft(options.has("if")).toBe(false)
	})

	it("offers the functions on a name that is already called", ({ expect }) => {
		const res = complete("rat(up)", 2, null)

		const options = byLabel(res)

		expect(res?.from).toBe(0)
		expect(res?.to).toBe(3)
		expect(res?.validFor).toEqual(/^\w*$/)
		// every function and aggregation, which the editor narrows to the
		// typed text
		expect.soft(options.size).toBe(232)
		expect.soft(options.has("rate")).toBe(true)
		expect.soft(options.has("with")).toBe(false)
	})

	it.for([
		{ name: "passes no result through", options: null },
		{
			name: "passes a result through when no function or operator goes there",
			options: [{ label: "job", type: "text" }],
		},
	])("$name", ({ options }, { expect }) => {
		const state = EditorState.create({ doc: "up{" })
		const res: CompletionResult | null = options
			? { from: 3, options: options }
			: null

		expect(
			withMetricsQLCompletions(new CompletionContext(state, 3, false), res, t),
		).toBe(res)
	})
})

function complete(doc: string, pos: number, options: Completion[] | null) {
	const state = EditorState.create({ doc })
	const res = options ? { from: 0, options: options } : null

	return withMetricsQLCompletions(
		new CompletionContext(state, pos, false),
		res,
		t,
	)
}

function byLabel(res: CompletionResult | null) {
	return new Map(res?.options.map((o) => [o.label, o]))
}
