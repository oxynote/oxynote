import { flushPromises } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useLogDataSourceAPI, { canRunLogQueries } from "./useLogDataSourceAPI"

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
	nextCursor: null,
	previousCursor: null,
	partial: false,
}

describe("canRunLogQueries", () => {
	it.for([
		{ name: "rejects a missing list", input: undefined, expected: false },
		{ name: "rejects a null list", input: null, expected: false },
		{ name: "rejects an empty list", input: [], expected: false },
		{
			name: "rejects a list with a blank query",
			input: ["{env}", " \t"],
			expected: false,
		},
		{ name: "accepts one query", input: ["{env}"], expected: true },
		{
			name: "accepts several queries",
			input: ["{job=api}", "{job=db}"],
			expected: true,
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(canRunLogQueries(input)).toBe(expected)
	})
})

// creating a query with a truthy enabled condition eagerly loads it once;
// refresh() joins that in-flight load (or reuses its fresh result) instead
// of forcing a second request, which keeps the call accounting deterministic
describe("useLogDataSourceAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useLogs", () => {
		it.for([
			{
				name: "returns null without a data source id",
				dataSourceId: null,
				queries: ["{env}"],
			},
			{
				name: "returns null without a query",
				dataSourceId: "ds1",
				queries: [],
			},
			{
				name: "returns null when one query is blank",
				dataSourceId: "ds1",
				queries: ["{env}", " "],
			},
		])("$name", async ({ dataSourceId, queries }, { expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(dataSourceId, { queries, ...TIME_RANGE }, true),
			)

			const result = await logs.refresh()

			expect(result.data?.pages).toEqual([null])
			expect(logs.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs("ds1", { queries: ["{env}"], ...TIME_RANGE }, false),
			)

			await flushPromises()

			expect(logs.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})

		it("fetches the newest page with every parameter", async ({ expect }) => {
			const page = makeLogsResult("l1", "ns:1")
			const calls = mockEndpoint("GET", LINES_URL, () => page)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{
						queries: ['{env="$env"}'],
						from: new Date(FROM),
						to: "2026-01-02T00:00:00.123456789Z",
						timeRangeKey: "last-24h",
						limit: 50,
						order: LogOrder.Asc,
						variableValues: { env: "prod" },
					},
					true,
				),
			)

			const result = await logs.refresh()

			expect(result.data).toEqual({
				pages: [
					{
						results: [page],
						window: { from: FROM, to: "2026-01-02T00:00:00.123456789Z" },
					},
				],
				pageParams: [null],
			})
			expect(logs.hasNextPage.value).toBe(true)
			expect(calls).toHaveLength(1)
			// a Date becomes RFC 3339; a string passes through with its
			// fractions
			expect(calls[0]?.query).toEqual({
				q: '{env="$env"}',
				from: FROM,
				to: "2026-01-02T00:00:00.123456789Z",
				"var-env": "prod",
				limit: "50",
				order: "asc",
			})
		})

		it("keeps the pages of each order apart", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const params = { queries: ["{env}"], ...TIME_RANGE }

			await runInApp(() =>
				api.useLogs("ds1", { ...params, order: LogOrder.Desc }, true),
			).refresh()
			await runInApp(() =>
				api.useLogs("ds1", { ...params, order: LogOrder.Asc }, true),
			).refresh()

			expect(calls.map((call) => call.query.order)).toEqual(["desc", "asc"])
		})

		it("fetches the newest page of every query into one page", async ({
			expect,
		}) => {
			const apiLogs = makeLogsResult("api-1", "api:1")
			const dbLogs = makeLogsResult("db-1", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.q === "{job=api}" ? apiLogs : dbLogs,
			)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{ queries: ["{job=api}", "{job=db}"], ...TIME_RANGE },
					true,
				),
			)

			const result = await logs.refresh()

			expect(result.data).toEqual({
				pages: [{ results: [apiLogs, dbLogs], window: { from: FROM, to: TO } }],
				pageParams: [null],
			})
			expect(logs.hasNextPage.value).toBe(true)
			// the queries run in parallel, so their order of arrival is not
			// part of the contract
			expect(calls).toHaveLength(2)
			expect(calls.map((call) => call.query)).toContainEqual({
				q: "{job=api}",
				from: FROM,
				to: TO,
			})
			expect(calls.map((call) => call.query)).toContainEqual({
				q: "{job=db}",
				from: FROM,
				to: TO,
			})
		})

		it("reports no further page when nothing matches", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{ queries: ["{job=api}", "{job=db}"], ...TIME_RANGE },
					true,
				),
			)

			const result = await logs.refresh()

			expect(result.data).toEqual({
				pages: [
					{ results: [NO_LINES, NO_LINES], window: { from: FROM, to: TO } },
				],
				pageParams: [null],
			})
			expect(logs.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(2)
		})

		it("loads older pages with the cursor of the previous one", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs("ds1", { queries: ["{env}"], ...TIME_RANGE }, true),
			)
			await logs.refresh()

			await logs.loadNextPage()

			expect(logs.data.value).toEqual({
				pages: [
					{ results: [first], window: { from: FROM, to: TO } },
					{ results: [second], window: { from: FROM, to: TO } },
				],
				pageParams: [
					null,
					{ cursors: ["ns:2"], window: { from: FROM, to: TO } },
				],
			})
			expect(logs.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: "{env}",
				from: FROM,
				to: TO,
				cursor: "ns:2",
			})
		})

		it("asks only the queries with older lines for the next page", async ({
			expect,
		}) => {
			const apiFirst = makeLogsResult("api-1", "api:1")
			const apiSecond = makeLogsResult("api-2", "api:2")
			const dbFirst = makeLogsResult("db-1", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.q === "{job=db}") {
					return dbFirst
				}

				return call.query.cursor ? apiSecond : apiFirst
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{ queries: ["{job=api}", "{job=db}"], ...TIME_RANGE },
					true,
				),
			)
			await logs.refresh()

			await logs.loadNextPage()

			expect(logs.data.value).toEqual({
				pages: [
					{ results: [apiFirst, dbFirst], window: { from: FROM, to: TO } },
					{ results: [apiSecond, null], window: { from: FROM, to: TO } },
				],
				pageParams: [
					null,
					{ cursors: ["api:1", null], window: { from: FROM, to: TO } },
				],
			})
			expect(logs.hasNextPage.value).toBe(true)
			expect(calls).toHaveLength(3)
			expect(calls[2]?.query).toEqual({
				q: "{job=api}",
				from: FROM,
				to: TO,
				cursor: "api:1",
			})
		})

		it("reports no further page once every query ran out of older lines", async ({
			expect,
		}) => {
			const apiFirst = makeLogsResult("api-1", "api:1")
			const apiSecond = makeLogsResult("api-2", null)
			const dbFirst = makeLogsResult("db-1", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.q === "{job=db}") {
					return dbFirst
				}

				return call.query.cursor ? apiSecond : apiFirst
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{ queries: ["{job=api}", "{job=db}"], ...TIME_RANGE },
					true,
				),
			)
			await logs.refresh()

			await logs.loadNextPage()

			expect(logs.data.value?.pages[1]).toEqual({
				results: [apiSecond, null],
				window: { from: FROM, to: TO },
			})
			expect(logs.hasNextPage.value).toBe(false)
			expect(calls).toHaveLength(3)
		})

		it("reads older pages from the first page's window after the window moves", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			params.value = {
				...params.value,
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}

			await logs.loadNextPage()

			expect(logs.data.value?.pages[1]).toEqual({
				results: [second],
				window: { from: FROM, to: TO },
			})
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
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			await logs.loadNextPage()
			const moved = {
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}
			params.value = { ...params.value, ...moved }

			await logs.refetch()

			expect(logs.data.value).toEqual({
				pages: [
					{ results: [first], window: moved },
					{ results: [second], window: moved },
				],
				pageParams: [null, { cursors: ["ns:2"], window: moved }],
			})
			expect(calls).toHaveLength(4)
			expect(calls[2]?.query).toEqual({ q: "{env}", ...moved })
			expect(calls[3]?.query).toEqual({ q: "{env}", ...moved, cursor: "ns:2" })
		})

		it("reads only the first page from the new window on a reload", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			await logs.loadNextPage()
			const moved = {
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}
			params.value = { ...params.value, ...moved }

			await logs.reload()

			expect(logs.data.value).toEqual({
				pages: [{ results: [first], window: moved }],
				pageParams: [null],
			})
			expect(logs.hasNextPage.value).toBe(true)
			expect(calls).toHaveLength(3)
			expect(calls[2]?.query).toEqual({ q: "{env}", ...moved })
		})

		it("puts the lines that arrived since before the first page and moves the window, newest first", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null)
			const added = makeLogsResult("n1", null)
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.from === TO) {
					return added
				}

				return call.query.cursor ? second : first
			})
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			await logs.loadNextPage()
			const later = "2026-01-02T00:05:00.000Z"
			params.value = {
				...params.value,
				from: "2026-01-01T00:05:00.000Z",
				to: later,
			}

			await logs.loadNewer()

			const moved = { from: "2026-01-01T00:05:00.000Z", to: later }
			expect(logs.data.value).toEqual({
				pages: [
					{
						results: [{ ...first, lines: [...added.lines, ...first.lines] }],
						window: moved,
					},
					{ results: [second], window: moved },
				],
				pageParams: [null, { cursors: ["ns:2"], window: moved }],
			})
			expect(calls).toHaveLength(3)
			expect(calls[2]?.query).toEqual({ q: "{env}", from: TO, to: later })
		})

		it("reloads from the first page when more lines arrived than a few pages hold", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			let newer = 0
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.from !== TO) {
					return first
				}

				newer++

				return makeLogsResult(`n${String(newer)}`, "ns:9")
			})
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			await logs.loadNextPage()
			params.value = {
				...params.value,
				from: "2026-01-01T00:05:00.000Z",
				to: "2026-01-02T00:05:00.000Z",
			}

			await logs.loadNewer()

			expect(logs.data.value?.pages).toHaveLength(1)
			expect(calls.filter((call) => call.query.from === TO)).toHaveLength(5)
		})

		it.for([
			{ name: "newest first", input: LogOrder.Desc },
			{ name: "oldest first", input: LogOrder.Asc },
		])(
			"drops the lines that fell out of the moved window, $name",
			async ({ input: order }, { expect }) => {
				const first = makeLogsResult("l1", null)
				first.lines = [
					makeLog("old", "2026-01-01T00:01:00Z"),
					makeLog("kept", "2026-01-01T12:00:00Z"),
				]
				mockEndpoint("GET", LINES_URL, (call) =>
					call.query.from === TO ? NO_LINES : first,
				)
				const params = ref<LogsParams>({
					queries: ["{env}"],
					...TIME_RANGE,
					order: order,
				})
				const api = makeLogDataSourceAPI()
				const logs = runInApp(() => api.useLogs("ds1", params, true))
				await logs.refresh()
				params.value = {
					...params.value,
					from: "2026-01-01T00:05:00.000Z",
					to: "2026-01-02T00:05:00.000Z",
				}

				await logs.loadNewer()

				expect(
					logs.data.value?.pages[0]?.results[0]?.lines.map((l) => l.id),
				).toEqual(["kept"])
			},
		)

		it("loads the lines a trim dropped from the top back, read the other way", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", null, "pv:2")
			const back = {
				...makeLogsResult("b1", "up:1", "dn:1"),
				lines: [
					makeLog("b1", "2026-01-01T12:00:01Z"),
					makeLog("b2", "2026-01-01T12:00:02Z"),
				],
			}
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.cursor === "pv:2") {
					return back
				}

				return call.query.cursor ? second : first
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs("ds1", { queries: ["{env}"], ...TIME_RANGE }, true),
			)
			await logs.refresh()
			await logs.loadNextPage()
			logs.trimStart(1)

			await logs.loadPreviousPage()

			expect(logs.data.value).toEqual({
				pages: [
					{
						results: [
							{
								...back,
								lines: [...back.lines].reverse(),
								nextCursor: "dn:1",
								previousCursor: "up:1",
							},
						],
						window: { from: FROM, to: TO },
					},
					{ results: [second], window: { from: FROM, to: TO } },
				],
				pageParams: [
					{ cursors: ["pv:2"], window: { from: FROM, to: TO }, previous: true },
					{ cursors: ["ns:2"], window: { from: FROM, to: TO } },
				],
			})
			expect(logs.hasPreviousPage.value).toBe(true)
			expect(calls[2]?.query).toEqual({
				q: "{env}",
				from: FROM,
				to: TO,
				cursor: "pv:2",
				order: "asc",
			})
		})

		it("trims the result whose last line sits highest first, keeping one result per query", async ({
			expect,
		}) => {
			const pagesOf = {
				"{job=a}": [
					{
						...makeLogsResult("a", "a:1"),
						lines: [
							makeLog("a1", "2026-01-01T12:00:09Z"),
							makeLog("a2", "2026-01-01T12:00:08Z"),
						],
					},
					{
						...makeLogsResult("a", null, "a:prev"),
						lines: [makeLog("a3", "2026-01-01T12:00:01Z")],
					},
				],
				"{job=b}": [
					{
						...makeLogsResult("b", "b:1"),
						lines: [
							makeLog("b1", "2026-01-01T12:00:07Z"),
							makeLog("b2", "2026-01-01T12:00:02Z"),
						],
					},
					{
						...makeLogsResult("b", null, "b:prev"),
						lines: [makeLog("b3", "2026-01-01T12:00:00Z")],
					},
				],
			}
			mockEndpoint("GET", LINES_URL, (call) => {
				const results = pagesOf[call.query.q as keyof typeof pagesOf]

				return call.query.cursor ? results[1] : results[0]
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs(
					"ds1",
					{ queries: ["{job=a}", "{job=b}"], ...TIME_RANGE },
					true,
				),
			)
			await logs.refresh()
			await logs.loadNextPage()

			logs.trimStart(4)

			expect(logs.data.value?.pages.map((page) => page?.results)).toEqual([
				[null, pagesOf["{job=b}"][0]],
				[pagesOf["{job=a}"][1], pagesOf["{job=b}"][1]],
			])
			expect(logs.hasPreviousPage.value).toBe(true)

			logs.trimStart(1)

			expect(logs.data.value?.pages.map((page) => page?.results)).toEqual([
				[pagesOf["{job=a}"][1], pagesOf["{job=b}"][1]],
			])
		})

		it("trims whole results from the end, keeping the first", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const second = makeLogsResult("l2", "ns:3")
			mockEndpoint("GET", LINES_URL, (call) =>
				call.query.cursor ? second : first,
			)
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() =>
				api.useLogs("ds1", { queries: ["{env}"], ...TIME_RANGE }, true),
			)
			await logs.refresh()
			await logs.loadNextPage()

			logs.trimEnd(1)

			expect(logs.data.value).toEqual({
				pages: [{ results: [first], window: { from: FROM, to: TO } }],
				pageParams: [null],
			})
			expect(logs.hasNextPage.value).toBe(true)
		})

		it("adds the lines that arrived since after a query read to its end, oldest first", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", null)
			const added = makeLogsResult("n1", "ns:7")
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.from === TO ? added : first,
			)
			const params = ref<LogsParams>({
				queries: ["{env}"],
				...TIME_RANGE,
				order: LogOrder.Asc,
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			const later = "2026-01-02T00:05:00.000Z"
			params.value = { ...params.value, to: later }

			await logs.loadNewer()

			expect(logs.data.value?.pages).toEqual([
				{
					results: [
						{
							...first,
							lines: [...first.lines, ...added.lines],
							nextCursor: "ns:7",
						},
					],
					window: { from: FROM, to: later },
				},
			])
			expect(logs.hasNextPage.value).toBe(true)
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: "{env}",
				from: TO,
				to: later,
				order: "asc",
			})
		})

		it("only widens the window of an oldest-first query with pages left", async ({
			expect,
		}) => {
			const first = makeLogsResult("l1", "ns:2")
			const calls = mockEndpoint("GET", LINES_URL, () => first)
			const params = ref<LogsParams>({
				queries: ["{env}"],
				...TIME_RANGE,
				order: LogOrder.Asc,
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			const later = "2026-01-02T00:05:00.000Z"
			params.value = { ...params.value, to: later }

			await logs.loadNewer()

			expect(logs.data.value?.pages).toEqual([
				{ results: [first], window: { from: FROM, to: later } },
			])
			expect(calls).toHaveLength(1)
		})

		it("keeps the previous lines while a changed query loads", async ({
			expect,
			onTestFinished,
		}) => {
			const first = makeLogsResult("l1", null)
			const held = holdResponse()
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.q === "{env}" ? first : held.respond(),
			)
			const params = ref<LogsParams>({ queries: ["{env}"], ...TIME_RANGE })
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()
			onTestFinished(() => {
				held.release(NO_LINES)
			})

			params.value = { ...params.value, queries: ['{env} |= "timeout"'] }
			await held.reached

			expect(logs.data.value).toEqual({
				pages: [{ results: [first], window: { from: FROM, to: TO } }],
				pageParams: [null],
			})
			expect(logs.isPlaceholderData.value).toBe(true)
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: '{env} |= "timeout"',
				from: FROM,
				to: TO,
			})
		})

		it("asks again for other variable values", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => NO_LINES)
			const params = ref<LogsParams>({
				queries: ['{env="$env"}'],
				...TIME_RANGE,
				variableValues: { env: "prod" },
			})
			const api = makeLogDataSourceAPI()
			const logs = runInApp(() => api.useLogs("ds1", params, true))
			await logs.refresh()

			params.value = { ...params.value, variableValues: { env: "dev" } }
			await logs.refresh()

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
			const logs = runInApp(() =>
				api.useLogs("ds1", { queries: ["{env}"], ...TIME_RANGE }, true),
			)

			const result = await logs.refresh()

			expect(result.error).toMatchObject({
				statusCode: 400,
				data: { code: "cursor.invalid" },
			})
			expect(calls).toHaveLength(1)
		})
	})

	describe("useLogVolume", () => {
		it.for([
			{
				name: "returns null without a data source id",
				dataSourceId: null,
				queries: ["{env}"],
			},
			{
				name: "returns null without a query",
				dataSourceId: "ds1",
				queries: [],
			},
			{
				name: "returns null when one query is blank",
				dataSourceId: "ds1",
				queries: ["{env}", ""],
			},
		])("$name", async ({ dataSourceId, queries }, { expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, () => ({}))
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume(dataSourceId, { queries, ...TIME_RANGE }, true),
			)

			const result = await volume.refresh()

			expect(result.data).toBeNull()
			expect(calls).toHaveLength(0)
		})

		it("sends no request while fetching is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, () => ({}))
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume("ds1", { queries: ["{env}"], ...TIME_RANGE }, false),
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
						queries: ['{env="$env"}'],
						...TIME_RANGE,
						variableValues: { env: "prod" },
					},
					true,
				),
			)

			const result = await volume.refresh()

			expect(result.data).toEqual([expected])
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({
				q: '{env="$env"}',
				from: FROM,
				to: TO,
				"var-env": "prod",
			})
		})

		it("fetches the line counts of every query in query order", async ({
			expect,
		}) => {
			const apiVolume = {
				status: GenericQueryResultStatus.Ok,
				data: [{ labels: { level: "info" }, metrics: [[1793030835, 4]] }],
			}
			const dbVolume = { status: GenericQueryResultStatus.NoData }
			const calls = mockEndpoint("GET", VOLUME_URL, (call) =>
				call.query.q === "{job=api}" ? apiVolume : dbVolume,
			)
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume(
					"ds1",
					{ queries: ["{job=api}", "{job=db}"], ...TIME_RANGE },
					true,
				),
			)

			const result = await volume.refresh()

			expect(result.data).toEqual([apiVolume, dbVolume])
			// the queries run in parallel, so their order of arrival is not
			// part of the contract
			expect(calls).toHaveLength(2)
			expect(calls.map((call) => call.query)).toContainEqual({
				q: "{job=api}",
				from: FROM,
				to: TO,
			})
			expect(calls.map((call) => call.query)).toContainEqual({
				q: "{job=db}",
				from: FROM,
				to: TO,
			})
		})

		it("keeps the previous line counts while a changed query loads", async ({
			expect,
			onTestFinished,
		}) => {
			const first = { status: GenericQueryResultStatus.NoData }
			const held = holdResponse()
			const calls = mockEndpoint("GET", VOLUME_URL, (call) =>
				call.query.q === "{env}" ? first : held.respond(),
			)
			const params = ref<LogQueriesParams>({
				queries: ["{env}"],
				...TIME_RANGE,
			})
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() => api.useLogVolume("ds1", params, true))
			await volume.refresh()
			onTestFinished(() => {
				held.release(first)
			})

			params.value = { ...params.value, queries: ['{env} |= "timeout"'] }
			await held.reached

			expect(volume.data.value).toEqual([first])
			expect(volume.isPlaceholderData.value).toBe(true)
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: '{env} |= "timeout"',
				from: FROM,
				to: TO,
			})
		})

		it("reports an api error through the error state", async ({ expect }) => {
			const calls = mockEndpoint("GET", VOLUME_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "query.error", message: "parse error" }
			})
			const api = makeLogDataSourceAPI()
			const volume = runInApp(() =>
				api.useLogVolume("ds1", { queries: ["{env"], ...TIME_RANGE }, true),
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

function makeLogsResult(
	id: string,
	nextCursor: string | null,
	previousCursor: string | null = null,
): LogsResult {
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
		nextCursor,
		previousCursor,
		partial: false,
	}
}

function makeLog(id: string, time: string): Log {
	return {
		id,
		time,
		level: LogLevel.Info,
		line: "deploy finished",
		labels: { env: "production" },
		fields: { outcome: "successful" },
	}
}

// holds a response until release, so a test can look at the query while
// its request is in flight
function holdResponse() {
	let release: (value: unknown) => void = () => undefined
	let reach: () => void = () => undefined
	const reached = new Promise<void>((resolve) => {
		reach = resolve
	})

	return {
		reached,
		respond: () => {
			reach()

			return new Promise((resolve) => {
				release = resolve
			})
		},
		release: (value: unknown) => {
			release(value)
		},
	}
}
