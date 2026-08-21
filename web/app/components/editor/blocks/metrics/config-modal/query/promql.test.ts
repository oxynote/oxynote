import { createRequire } from "node:module"
import { describe, it, vi } from "vitest"
import { toValue } from "vue"
import type { CompletionSource } from "@codemirror/autocomplete"
import { TimeRangePreset } from "../../utils"
import {
	installAutoImportGlobals,
	stubPrometheusAPI,
	type PrometheusStubData,
} from "./test-helpers"
import { usePromQLLegendExtension, usePromQLQueryExtension } from "./promql"

// @prometheus-io/codemirror-promql declares no package exports, so node
// loads its CJS build together with CJS copies of the codemirror
// packages. The editor state and completion context built here must come
// from those same copies, or instanceof checks inside codemirror reject
// the extensions under test.
const cjsRequire = createRequire(import.meta.url)
const { EditorState } = cjsRequire(
	"@codemirror/state",
) as typeof import("@codemirror/state")
const { CompletionContext } = cjsRequire(
	"@codemirror/autocomplete",
) as typeof import("@codemirror/autocomplete")

installAutoImportGlobals()

const prometheusAPIMock = vi.hoisted(() => vi.fn())

vi.mock("~/composables/api/usePrometheusDataSourceAPI", () => ({
	default: prometheusAPIMock,
}))

const t = (key: string) => key

const PRESET_LABELS = [
	"$__interval",
	"$__range",
	"$__rate_interval",
	"1m",
	"5m",
	"10m",
	"30m",
	"1h",
	"1d",
]

function makeComplete(data?: PrometheusStubData) {
	const stub = stubPrometheusAPI(prometheusAPIMock, data)
	const { extensions } = usePromQLQueryExtension(
		t,
		() => "ds-1",
		() => TimeRangePreset.Last1Hour,
		() => true,
	)

	async function complete(doc: string, pos: number) {
		const state = EditorState.create({ doc, extensions: extensions.value })
		const sources = state.languageDataAt<CompletionSource>("autocomplete", pos)
		const ctx = new CompletionContext(state, pos, false)

		for (const source of sources) {
			const res = await source(ctx)

			if (res) {
				return res
			}
		}

		return null
	}

	return { ...stub, complete }
}

function makeLegend(
	options: {
		query?: string | null
		timeRange?: TimeRangePreset | null
		data?: PrometheusStubData
	} = {},
) {
	const stub = stubPrometheusAPI(prometheusAPIMock, options.data)
	const legend = usePromQLLegendExtension(
		() => options.query ?? 'rate(http_requests_total{job="api"}[5m])',
		() => "ds-1",
		() =>
			"timeRange" in options ? options.timeRange : TimeRangePreset.Last1Hour,
		() => true,
	)

	return { ...stub, ...legend }
}

describe("usePromQLQueryExtension", () => {
	it("passes base completions through outside duration brackets", async ({
		expect,
	}) => {
		const { api, captured, complete } = makeComplete({
			labelValues: { result: ["go_goroutines", "http_requests_total"] },
		})

		const res = await complete("go_", 3)
		const labels = res?.options.map((o) => o.label) ?? []

		expect(res?.from).toBe(0)
		expect(res?.to).toBe(3)
		expect(labels).toContain("go_goroutines")
		expect(labels).toContain("http_requests_total")
		expect(labels).not.toContain("$__interval")

		// metric names flow through the wired client as the values of the
		// reserved __name__ label, scoped to the configured time range
		expect(api.labelValues.refresh).toHaveBeenCalled()
		expect(toValue(captured.labelValuesParams)?.label).toBe("__name__")
		expect(toValue(captured.labelValuesParams)?.timeRangeKey).toBe(
			TimeRangePreset.Last1Hour,
		)
	})

	it.for([
		{
			name: "offers the duration presets inside empty range brackets",
			doc: "rate(http_requests_total[])",
			pos: 25,
			from: 25,
			to: 25,
		},
		{
			name: "offers the presets when typing a dollar variable",
			doc: "rate(http_requests_total[$",
			pos: 26,
			from: 25,
			to: 26,
		},
		{
			name: "offers the presets when typing a number",
			doc: "rate(up[5",
			pos: 9,
			from: 8,
			to: 9,
		},
		{
			name: "keeps offering the presets for a partial duration",
			doc: "rate(up[1m",
			pos: 10,
			from: 8,
			to: 10,
		},
	])("$name", async ({ doc, pos, from, to }, { expect }) => {
		const { complete } = makeComplete()

		const res = await complete(doc, pos)

		// the base strategy's bare unit suffixes (ms, s, m, …) are dropped,
		// leaving exactly the presets in their declared order
		expect(res?.options.map((o) => o.label)).toEqual(PRESET_LABELS)
		expect(res?.from).toBe(from)
		expect(res?.to).toBe(to)
		expect(res?.validFor).toEqual(/^[\w$]*$/)
	})

	it("hides the presets for a word token inside brackets", async ({
		expect,
	}) => {
		const { complete } = makeComplete()

		const res = await complete("rate(up[x", 9)
		const labels = res?.options.map((o) => o.label) ?? []

		expect(labels.length).toBeGreaterThan(0)
		expect(labels).not.toContain("$__interval")
		expect(labels).not.toContain("1m")
	})

	it("describes the variable presets through the translator", async ({
		expect,
	}) => {
		const { complete } = makeComplete()

		const res = await complete("rate(http_requests_total[])", 25)
		const byLabel = new Map(res?.options.map((o) => [o.label, o]))

		expect(byLabel.get("$__interval")).toMatchObject({
			type: "variable",
			info: "editor.metrics.config.query-info.prometheus.interval",
		})
		expect(byLabel.get("$__range")).toMatchObject({
			type: "variable",
			info: "editor.metrics.config.query-info.prometheus.range",
		})
		expect(byLabel.get("$__rate_interval")).toMatchObject({
			type: "variable",
			info: "editor.metrics.config.query-info.prometheus.rate-interval",
		})
		expect(byLabel.get("1m")?.type).toBe("constant")
	})
})

describe("usePromQLLegendExtension", () => {
	it("requests labels for the selectors extracted from the query", ({
		expect,
	}) => {
		const { captured } = makeLegend({
			query: 'rate(http_requests_total{job="api"}[5m]) + up',
		})

		const params = toValue(captured.labelsParams)

		expect(params?.matchers).toEqual(['http_requests_total{job="api"}', "up"])
		expect(params?.timeRangeKey).toBe(TimeRangePreset.Last1Hour)
		expect(params?.from).toBeInstanceOf(Date)
		expect(params?.to).toBeInstanceOf(Date)
	})

	it.for([
		{ name: "requests nothing while the query is empty", query: "" },
		{
			name: "requests nothing without a time range",
			query: "up",
			timeRange: null,
		},
	])("$name", ({ query, timeRange }, { expect }) => {
		const { captured } = makeLegend({ query, timeRange })

		expect(toValue(captured.labelsParams)).toBeNull()
	})

	describe("fetchAllLabelNames", () => {
		it("drops prometheus-internal labels from the fetched names", async ({
			expect,
		}) => {
			const { api, fetchAllLabelNames } = makeLegend({
				data: { labels: { result: ["job", "__name__", "instance"] } },
			})

			await expect(fetchAllLabelNames()).resolves.toEqual(["job", "instance"])
			expect(api.labels.refresh).toHaveBeenCalledTimes(1)
			expect(api.series.refresh).toHaveBeenCalledTimes(0)
		})

		it("returns no names when the labels query has no data", async ({
			expect,
		}) => {
			const { fetchAllLabelNames } = makeLegend({ data: { labels: null } })

			await expect(fetchAllLabelNames()).resolves.toEqual([])
		})
	})

	describe("fetchExampleLabelValues", () => {
		it("returns the first matching series", async ({ expect }) => {
			const { api, fetchExampleLabelValues } = makeLegend({
				data: {
					series: {
						result: [
							{ job: "api", instance: "a:9090" },
							{ job: "web", instance: "b:9090" },
						],
					},
				},
			})

			await expect(fetchExampleLabelValues()).resolves.toEqual({
				job: "api",
				instance: "a:9090",
			})
			expect(api.series.refresh).toHaveBeenCalledTimes(1)
			expect(api.labels.refresh).toHaveBeenCalledTimes(0)
		})

		it("returns no values when no series match", async ({ expect }) => {
			const { fetchExampleLabelValues } = makeLegend({
				data: { series: { result: [] } },
			})

			await expect(fetchExampleLabelValues()).resolves.toEqual({})
		})
	})
})
