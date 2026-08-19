import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import usePrometheusDataSourceAPI from "./usePrometheusDataSourceAPI"

function makePrometheusDataSourceAPI() {
	return runInApp(() => usePrometheusDataSourceAPI())
}

const FROM = "2026-01-01T00:00:00.000Z"
const TO = "2026-01-02T00:00:00.000Z"

const TIME_RANGE = {
	from: FROM,
	to: TO,
	timeRangeKey: "last-24h",
}

const QUERY_RESULT = {
	type: "vector",
	result: [{ metric: { job: "web" }, value: [1735689600, "1"] }],
}

// creating a query with a truthy enabled condition eagerly loads it once;
// refresh() joins that in-flight load (or reuses its fresh result) instead
// of forcing a second request, which keeps the call accounting deterministic
describe("usePrometheusDataSourceAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("usePrometheusQuery", () => {
		it("returns null without a query string", async ({ expect }) => {
			const queryCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/query",
				() => QUERY_RESULT,
			)
			const api = makePrometheusDataSourceAPI()
			const query = runInApp(() =>
				api.usePrometheusQuery("ds1", { q: "", ...TIME_RANGE }, true),
			)

			const result = await query.refresh()

			expect(result.data).toBeNull()
			expect(queryCalls).toHaveLength(0)
		})

		it("fetches the query result with the time range", async ({ expect }) => {
			const queryCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/query",
				() => QUERY_RESULT,
			)
			const api = makePrometheusDataSourceAPI()
			const query = runInApp(() =>
				api.usePrometheusQuery("ds1", { q: "up", ...TIME_RANGE }, true),
			)

			const result = await query.refresh()

			expect(result.data).toEqual(QUERY_RESULT)
			expect(queryCalls).toHaveLength(1)
			expect(queryCalls[0]?.query).toEqual({ q: "up", from: FROM, to: TO })
		})
	})

	describe("usePrometheusMultipleQueries", () => {
		it("returns no results without query strings", async ({ expect }) => {
			const queryCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/query",
				() => QUERY_RESULT,
			)
			const api = makePrometheusDataSourceAPI()
			const queries = runInApp(() =>
				api.usePrometheusMultipleQueries(
					"ds1",
					{ queries: [], ...TIME_RANGE },
					true,
				),
			)

			const result = await queries.refresh()

			expect(result.data).toEqual([])
			expect(queryCalls).toHaveLength(0)
		})

		it("fetches one result per query string in order", async ({ expect }) => {
			// the response embeds the incoming query string so the resolved
			// data proves which request produced which result
			const queryCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/query",
				(call) => ({
					type: "vector",
					result: [{ metric: { q: call.query.q } }],
				}),
			)
			const api = makePrometheusDataSourceAPI()
			const queries = runInApp(() =>
				api.usePrometheusMultipleQueries(
					"ds1",
					{ queries: ["up", "process_cpu"], ...TIME_RANGE },
					true,
				),
			)

			const result = await queries.refresh()

			expect(result.data).toEqual([
				{ type: "vector", result: [{ metric: { q: "up" } }] },
				{ type: "vector", result: [{ metric: { q: "process_cpu" } }] },
			])
			expect(queryCalls).toHaveLength(2)
			expect(queryCalls[0]?.query).toEqual({ q: "up", from: FROM, to: TO })
			expect(queryCalls[1]?.query).toEqual({
				q: "process_cpu",
				from: FROM,
				to: TO,
			})
		})
	})

	describe("usePrometheusMetadata", () => {
		it("returns null without a data source id", async ({ expect }) => {
			const metadataCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/metadata",
				() => ({ result: {} }),
			)
			const api = makePrometheusDataSourceAPI()
			const metadata = runInApp(() => api.usePrometheusMetadata(null, true))

			const result = await metadata.refresh()

			expect(result.data).toBeNull()
			expect(metadataCalls).toHaveLength(0)
		})

		it("fetches the metric metadata of the data source", async ({ expect }) => {
			const metadataResult = {
				result: { up: [{ type: "gauge", help: "up", unit: "" }] },
			}
			const metadataCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/metadata",
				() => metadataResult,
			)
			const api = makePrometheusDataSourceAPI()
			const metadata = runInApp(() => api.usePrometheusMetadata("ds1", true))

			const result = await metadata.refresh()

			expect(result.data).toEqual(metadataResult)
			expect(metadataCalls).toHaveLength(1)
			expect(metadataCalls[0]?.query).toEqual({})
		})
	})

	describe("usePrometheusLabels", () => {
		it("returns null without matchers", async ({ expect }) => {
			const labelCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/labels",
				() => ({ result: [] }),
			)
			const api = makePrometheusDataSourceAPI()
			const labels = runInApp(() =>
				api.usePrometheusLabels("ds1", { ...TIME_RANGE }, true),
			)

			const result = await labels.refresh()

			expect(result.data).toBeNull()
			expect(labelCalls).toHaveLength(0)
		})

		it("fetches the label names matching the matchers", async ({ expect }) => {
			const labelCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/labels",
				() => ({ result: ["job", "instance"] }),
			)
			const api = makePrometheusDataSourceAPI()
			const labels = runInApp(() =>
				api.usePrometheusLabels(
					"ds1",
					{ matchers: ["up", "node_load1"], ...TIME_RANGE },
					true,
				),
			)

			const result = await labels.refresh()

			expect(result.data).toEqual({ result: ["job", "instance"] })
			expect(labelCalls).toHaveLength(1)
			// the matchers parameter is repeated, so getQuery yields an array
			expect(labelCalls[0]?.query).toEqual({
				from: FROM,
				to: TO,
				matchers: ["up", "node_load1"],
			})
		})
	})

	describe("usePrometheusLabelValues", () => {
		it("returns null without a label", async ({ expect }) => {
			const valueCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/labels/job/values",
				() => ({ result: [] }),
			)
			const api = makePrometheusDataSourceAPI()
			const values = runInApp(() =>
				api.usePrometheusLabelValues("ds1", { label: "", ...TIME_RANGE }, true),
			)

			const result = await values.refresh()

			expect(result.data).toBeNull()
			expect(valueCalls).toHaveLength(0)
		})

		it("fetches the values of the label", async ({ expect }) => {
			const valueCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/labels/job/values",
				() => ({ result: ["web", "worker"] }),
			)
			const api = makePrometheusDataSourceAPI()
			const values = runInApp(() =>
				api.usePrometheusLabelValues(
					"ds1",
					{ label: "job", ...TIME_RANGE },
					true,
				),
			)

			const result = await values.refresh()

			expect(result.data).toEqual({ result: ["web", "worker"] })
			expect(valueCalls).toHaveLength(1)
			expect(valueCalls[0]?.query).toEqual({ from: FROM, to: TO })
		})

		it("fetches the values of the label matching the matchers", async ({
			expect,
		}) => {
			const valueCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/labels/job/values",
				() => ({ result: ["web"] }),
			)
			const api = makePrometheusDataSourceAPI()
			const values = runInApp(() =>
				api.usePrometheusLabelValues(
					"ds1",
					{ label: "job", matchers: ["up", "node_load1"], ...TIME_RANGE },
					true,
				),
			)

			const result = await values.refresh()

			expect(result.data).toEqual({ result: ["web"] })
			expect(valueCalls).toHaveLength(1)
			// the matchers parameter is repeated, so getQuery yields an array
			expect(valueCalls[0]?.query).toEqual({
				from: FROM,
				to: TO,
				matchers: ["up", "node_load1"],
			})
		})
	})

	describe("usePrometheusSeries", () => {
		it("returns null without matchers", async ({ expect }) => {
			const seriesCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/series",
				() => ({ result: [] }),
			)
			const api = makePrometheusDataSourceAPI()
			const series = runInApp(() =>
				api.usePrometheusSeries("ds1", { matchers: [], ...TIME_RANGE }, true),
			)

			const result = await series.refresh()

			expect(result.data).toBeNull()
			expect(seriesCalls).toHaveLength(0)
		})

		it("fetches the series matching the matchers", async ({ expect }) => {
			const seriesResult = {
				result: [{ __name__: "up", job: "web" }],
			}
			const seriesCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/prometheus/series",
				() => seriesResult,
			)
			const api = makePrometheusDataSourceAPI()
			const series = runInApp(() =>
				api.usePrometheusSeries(
					"ds1",
					{ matchers: ["up", "node_load1"], ...TIME_RANGE },
					true,
				),
			)

			const result = await series.refresh()

			expect(result.data).toEqual(seriesResult)
			expect(seriesCalls).toHaveLength(1)
			// the matchers parameter is repeated, so getQuery yields an array
			expect(seriesCalls[0]?.query).toEqual({
				from: FROM,
				to: TO,
				matchers: ["up", "node_load1"],
			})
		})
	})
})
