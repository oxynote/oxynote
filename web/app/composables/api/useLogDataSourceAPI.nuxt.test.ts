import { flushPromises } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useLogDataSourceAPI from "./useLogDataSourceAPI"

const LINES_URL = "/api/data-sources/ds1/logs"
const VOLUME_URL = "/api/data-sources/ds1/logs/volume"
const FIELDS_URL = "/api/data-sources/ds1/logs/fields"
const VALUES_URL = "/api/data-sources/ds1/logs/fields/env/values"

const FROM = "2026-01-01T00:00:00.000Z"
const TO = "2026-01-02T00:00:00.000Z"

const TIME_RANGE = {
	from: FROM,
	to: TO,
	timeRangeKey: "last-24h",
}

const NO_LINES = {
	status: GenericQueryResultStatus.NoData,
	lines: [],
	cursor: null,
	partial: false,
}

// creating a query with a truthy enabled condition eagerly loads it once;
// refresh() joins that in-flight load (or reuses its fresh result) instead
// of forcing a second request, which keeps the call accounting deterministic
describe("useLogDataSourceAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useLogLines", () => {
		it.for([
			{ name: "returns null without a data source id", dataSourceId: null },
			{ name: "returns null without a query", dataSourceId: "ds1" },
		])("$name", async ({ dataSourceId }, { expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines(dataSourceId, { q: " ", ...TIME_RANGE }, true),
			)

			const result = await lines.refresh()

			expect(result.data?.pages).toEqual([null])
			expect(lines.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines("ds1", { q: "{env}", ...TIME_RANGE }, false),
			)

			await flushPromises()

			expect(lines.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})

		it("fetches the newest page with every parameter", async ({ expect }) => {
			const page = makeLinesPage("l1", "ns:1")
			const calls = mockEndpoint("GET", LINES_URL, () => page)
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines(
					"ds1",
					{
						q: '{env="$env"}',
						from: new Date(FROM),
						to: "2026-01-02T00:00:00.123456789Z",
						timeRangeKey: "last-24h",
						limit: 50,
						variableValues: { env: "prod" },
					},
					true,
				),
			)

			const result = await lines.refresh()

			expect(result.data).toEqual({
				pages: [
					{
						...page,
						window: { from: FROM, to: "2026-01-02T00:00:00.123456789Z" },
					},
				],
				pageParams: [null],
			})
			expect(lines.hasNextPage.value).toBe(true)
			expect(calls).toHaveLength(1)
			// a Date becomes RFC 3339; a string passes through with its
			// fractions
			expect(calls[0]?.query).toEqual({
				q: '{env="$env"}',
				from: FROM,
				to: "2026-01-02T00:00:00.123456789Z",
				"var-env": "prod",
				limit: "50",
			})
		})

		it("reports no further page when nothing matches", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines("ds1", { q: "{env}", ...TIME_RANGE }, true),
			)

			const result = await lines.refresh()

			expect(result.data).toEqual({
				pages: [{ ...NO_LINES, window: { from: FROM, to: TO } }],
				pageParams: [null],
			})
			expect(lines.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({ q: "{env}", from: FROM, to: TO })
		})

		it("loads older pages with the cursor of the previous one", async ({
			expect,
		}) => {
			const first = makeLinesPage("l1", "ns:2")
			const second = makeLinesPage("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines("ds1", { q: "{env}", ...TIME_RANGE }, true),
			)
			await lines.refresh()

			await lines.loadNextPage()

			expect(lines.data.value).toEqual({
				pages: [
					{ ...first, window: { from: FROM, to: TO } },
					{ ...second, window: { from: FROM, to: TO } },
				],
				pageParams: [null, { cursor: "ns:2", window: { from: FROM, to: TO } }],
			})
			expect(lines.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: "{env}",
				from: FROM,
				to: TO,
				cursor: "ns:2",
			})
		})

		it("reads older pages from the first page's window after the window moves", async ({
			expect,
		}) => {
			const first = makeLinesPage("l1", "ns:2")
			const second = makeLinesPage("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const params = ref({ q: "{env}", ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() => api.useLogLines("ds1", params, true))
			await lines.refresh()
			params.value = {
				...params.value,
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}

			await lines.loadNextPage()

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: "{env}",
				from: FROM,
				to: TO,
				cursor: "ns:2",
			})
		})

		it("reloads every page from the new window on a refresh", async ({
			expect,
		}) => {
			const first = makeLinesPage("l1", "ns:2")
			const second = makeLinesPage("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const params = ref({ q: "{env}", ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() => api.useLogLines("ds1", params, true))
			await lines.refresh()
			await lines.loadNextPage()
			const moved = {
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}
			params.value = { ...params.value, ...moved }

			await lines.refetch()

			expect(calls).toHaveLength(4)
			expect(calls[2]?.query).toEqual({ q: "{env}", ...moved })
			expect(calls[3]?.query).toEqual({ q: "{env}", ...moved, cursor: "ns:2" })
		})

		it("asks again for other variable values", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const params = ref({
				q: '{env="$env"}',
				...TIME_RANGE,
				variableValues: { env: "prod" },
			})
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() => api.useLogLines("ds1", params, true))
			await lines.refresh()

			params.value = { ...params.value, variableValues: { env: "dev" } }
			await lines.refresh()

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: '{env="$env"}',
				from: FROM,
				to: TO,
				"var-env": "dev",
			})
		})

		it("reports an api error through the error state", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "cursor.invalid", message: "bad cursor" }
			})
			const api = makeLogDataSourceAPI()
			const lines = runInApp(() =>
				api.useLogLines("ds1", { q: "{env}", ...TIME_RANGE }, true),
			)

			const result = await lines.refresh()

			expect(result.error).toMatchObject({
				statusCode: 400,
				data: { code: "cursor.invalid" },
			})
			expect(calls).toHaveLength(1)
		})
	})

	describe("useLogVolume", () => {
		it.for([
			{ name: "returns null without a data source id", dataSourceId: null },
			{ name: "returns null without a query", dataSourceId: "ds1" },
		])("$name", async ({ dataSourceId }, { expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, () => ({}))
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume(dataSourceId, { q: "", ...TIME_RANGE }, true),
			)

			const result = await volume.refresh()

			expect(result.data).toBeNull()
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, () => ({}))
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume("ds1", { q: "{env}", ...TIME_RANGE }, false),
			)

			await flushPromises()

			expect(volume.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})

		it.for([
			{
				name: "fetches the line counts per level",
				expected: {
					status: GenericQueryResultStatus.Ok,
					data: [{ labels: { level: "error" }, metrics: [[1793030835, 2]] }],
				},
			},
			{
				name: "fetches a no-data result when no line matches",
				expected: { status: GenericQueryResultStatus.NoData },
			},
		])("$name", async ({ expected }, { expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, () => expected)
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume(
					"ds1",
					{
						q: '{env="$env"}',
						...TIME_RANGE,
						variableValues: { env: "prod" },
					},
					true,
				),
			)

			const result = await volume.refresh()

			expect(result.data).toEqual(expected)
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({
				q: '{env="$env"}',
				from: FROM,
				to: TO,
				"var-env": "prod",
			})
		})

		it("reports an api error through the error state", async ({ expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "query.error", message: "parse error" }
			})
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume("ds1", { q: "{env", ...TIME_RANGE }, true),
			)

			const result = await volume.refresh()

			expect(result.error).toMatchObject({
				statusCode: 400,
				data: { code: "query.error" },
			})
			expect(calls).toHaveLength(1)
		})
	})

	describe("useLogFields", () => {
		it.for([
			{
				name: "returns null without a data source id",
				dataSourceId: null,
				params: TIME_RANGE,
			},
			{
				name: "returns null without parameters",
				dataSourceId: "ds1",
				params: null,
			},
		])("$name", async ({ dataSourceId, params }, { expect }) => {
			const calls = mockEndpoint("GET", FIELDS_URL, () => ({ fields: [] }))
			const api = makeLogDataSourceAPI()
			const fields = runInApp(() =>
				api.useLogFields(dataSourceId, params, true),
			)

			const result = await fields.refresh()

			expect(result.data).toBeNull()
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", FIELDS_URL, () => ({ fields: [] }))
			const api = makeLogDataSourceAPI()
			const fields = runInApp(() =>
				api.useLogFields("ds1", { ...TIME_RANGE }, false),
			)

			await flushPromises()

			expect(fields.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})

		it.for([
			{
				name: "fetches the fields of the time range without a query",
				params: { ...TIME_RANGE },
				expected: { from: FROM, to: TO },
			},
			{
				name: "fetches the fields of the lines the query matches",
				params: {
					...TIME_RANGE,
					q: '{env="$env"}',
					variableValues: { env: "prod" },
				},
				expected: {
					q: '{env="$env"}',
					from: FROM,
					to: TO,
					"var-env": "prod",
				},
			},
		])("$name", async ({ params, expected }, { expect }) => {
			const response = {
				fields: [
					{ name: "env", kind: LogFieldKind.Label },
					{ name: "severity", kind: LogFieldKind.Field },
				],
			}
			const calls = mockEndpoint("GET", FIELDS_URL, () => response)
			const api = makeLogDataSourceAPI()
			const fields = runInApp(() => api.useLogFields("ds1", params, true))

			const result = await fields.refresh()

			expect(result.data).toEqual(response)
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual(expected)
		})

		it("reports an api error through the error state", async ({ expect }) => {
			const calls = mockEndpoint("GET", FIELDS_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "from.invalid", message: "bad from" }
			})
			const api = makeLogDataSourceAPI()
			const fields = runInApp(() =>
				api.useLogFields("ds1", { ...TIME_RANGE }, true),
			)

			const result = await fields.refresh()

			expect(result.error).toMatchObject({
				statusCode: 400,
				data: { code: "from.invalid" },
			})
			expect(calls).toHaveLength(1)
		})
	})

	describe("useLogFieldValues", () => {
		it.for([
			{
				name: "returns null without a data source id",
				dataSourceId: null,
				field: "env",
			},
			{
				name: "returns null without a field",
				dataSourceId: "ds1",
				field: "",
			},
		])("$name", async ({ dataSourceId, field }, { expect }) => {
			const calls = mockEndpoint("GET", VALUES_URL, () => ({ values: [] }))
			const api = makeLogDataSourceAPI()
			const values = runInApp(() =>
				api.useLogFieldValues(dataSourceId, { field, ...TIME_RANGE }, true),
			)

			const result = await values.refresh()

			expect(result.data).toBeNull()
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", VALUES_URL, () => ({ values: [] }))
			const api = makeLogDataSourceAPI()
			const values = runInApp(() =>
				api.useLogFieldValues("ds1", { field: "env", ...TIME_RANGE }, false),
			)

			await flushPromises()

			expect(values.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})

		it("fetches the values of the field", async ({ expect }) => {
			const response = {
				values: [
					{ value: "production", count: 12 },
					{ value: "staging", count: null },
				],
			}
			const calls = mockEndpoint("GET", VALUES_URL, () => response)
			const api = makeLogDataSourceAPI()
			const values = runInApp(() =>
				api.useLogFieldValues(
					"ds1",
					{
						field: "env",
						q: '{job="$job"}',
						...TIME_RANGE,
						variableValues: { job: "api" },
					},
					true,
				),
			)

			const result = await values.refresh()

			expect(result.data).toEqual(response)
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({
				q: '{job="$job"}',
				from: FROM,
				to: TO,
				"var-job": "api",
			})
		})

		it("encodes the field name in the path", async ({ expect }) => {
			const calls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/logs/fields/a%2Fb/values",
				() => ({ values: [] }),
			)
			const api = makeLogDataSourceAPI()
			const values = runInApp(() =>
				api.useLogFieldValues("ds1", { field: "a/b", ...TIME_RANGE }, true),
			)

			const result = await values.refresh()

			expect(result.data).toEqual({ values: [] })
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({ from: FROM, to: TO })
		})

		it("asks again for another field", async ({ expect }) => {
			const envCalls = mockEndpoint("GET", VALUES_URL, () => ({ values: [] }))
			const jobCalls = mockEndpoint(
				"GET",
				"/api/data-sources/ds1/logs/fields/job/values",
				() => ({ values: [] }),
			)
			const params = ref({ field: "env", ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const values = runInApp(() => api.useLogFieldValues("ds1", params, true))
			await values.refresh()

			params.value = { ...params.value, field: "job" }
			await values.refresh()

			expect(envCalls).toHaveLength(1)
			expect(jobCalls).toHaveLength(1)
		})

		it("reports an api error through the error state", async ({ expect }) => {
			const calls = mockEndpoint("GET", VALUES_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "data_source.type_not_supported", message: "not loki" }
			})
			const api = makeLogDataSourceAPI()
			const values = runInApp(() =>
				api.useLogFieldValues("ds1", { field: "env", ...TIME_RANGE }, true),
			)

			const result = await values.refresh()

			expect(result.error).toMatchObject({
				statusCode: 400,
				data: { code: "data_source.type_not_supported" },
			})
			expect(calls).toHaveLength(1)
		})
	})
})

function makeLogDataSourceAPI() {
	return runInApp(() => useLogDataSourceAPI())
}

function makeLinesPage(id: string, cursor: string | null): LogLinesResult {
	return {
		status: GenericQueryResultStatus.Ok,
		lines: [
			{
				id,
				time: "2026-01-01T12:00:00.825596924Z",
				level: LogLevel.Info,
				line: "deploy finished",
				labels: { env: "production" },
				fields: { outcome: "successful" },
			},
		],
		cursor,
		partial: false,
	}
}
