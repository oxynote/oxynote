import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useSQLDataSourceAPI from "./useSQLDataSourceAPI"

function makeSQLDataSourceAPI() {
	return runInApp(() => useSQLDataSourceAPI())
}

const METADATA_RESULT = {
	tables: { "public.users": { columns: [{ name: "id" }] } },
	defaultSchema: "public",
}

// creating a query with a truthy enabled condition eagerly loads it once;
// refresh() joins that in-flight load (or reuses its fresh result) instead
// of forcing a second request, which keeps the call accounting deterministic
describe("useSQLDataSourceAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useSQLMetadata", () => {
		it("returns null without a data source id", async ({ expect }) => {
			const metadataCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/sql/metadata",
				() => METADATA_RESULT,
			)
			const api = makeSQLDataSourceAPI()
			const metadata = runInApp(() => api.useSQLMetadata(null, true))

			const result = await metadata.refresh()

			expect(result.data).toBeNull()
			expect(metadataCalls).toHaveLength(0)
		})

		it("fetches the sql metadata of the data source", async ({ expect }) => {
			const metadataCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/sql/metadata",
				() => METADATA_RESULT,
			)
			const api = makeSQLDataSourceAPI()
			const metadata = runInApp(() => api.useSQLMetadata("ds1", true))

			const result = await metadata.refresh()

			expect(result.data).toEqual(METADATA_RESULT)
			expect(metadataCalls).toHaveLength(1)
			expect(metadataCalls[0]?.query).toEqual({})
		})
	})

	describe("useSQLLabels", () => {
		it("returns null without a query string", async ({ expect }) => {
			const labelCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/sql/query-labels",
				() => ({ labels: {} }),
			)
			const api = makeSQLDataSourceAPI()
			const labels = runInApp(() =>
				api.useSQLLabels(
					"ds1",
					{
						q: "",
						from: "2026-01-01T00:00:00.000Z",
						to: "2026-01-02T00:00:00.000Z",
						timeRangeKey: "last-24h",
					},
					true,
				),
			)

			const result = await labels.refresh()

			expect(result.data).toBeNull()
			expect(labelCalls).toHaveLength(0)
		})

		it("fetches the query labels with the time range", async ({ expect }) => {
			const labelCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/sql/query-labels",
				() => ({ labels: { hostname: "web-1" } }),
			)
			const api = makeSQLDataSourceAPI()
			const labels = runInApp(() =>
				api.useSQLLabels(
					"ds1",
					{
						q: "SELECT * FROM metrics",
						from: "2026-01-01T00:00:00.000Z",
						to: "2026-01-02T00:00:00.000Z",
						timeRangeKey: "last-24h",
					},
					true,
				),
			)

			const result = await labels.refresh()

			expect(result.data).toEqual({ labels: { hostname: "web-1" } })
			expect(labelCalls).toHaveLength(1)
			// timeRangeKey is a local cache key only — it is never sent
			expect(labelCalls[0]?.query).toEqual({
				q: "SELECT * FROM metrics",
				from: "2026-01-01T00:00:00.000Z",
				to: "2026-01-02T00:00:00.000Z",
			})
		})
	})
})
