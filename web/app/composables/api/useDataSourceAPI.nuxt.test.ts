import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	ANY_DATE,
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useDataSourceAPI from "./useDataSourceAPI"

const DS_ID = makeXid("ds1")
const OTHER_DS_ID = makeXid("ds2")
const OPTIMISTIC_ID = "local-1"

const LIST_KEY = ["data-sources"] as const
const LIST_URL = "/api/data-sources"
const DETAIL_URL = `${LIST_URL}/${DS_ID}`
const CONNECTION_URL = `${DETAIL_URL}/connection`
const QUERY_URL = `${DETAIL_URL}/query`

function makeDataSourceAPI() {
	return runInApp(() => useDataSourceAPI())
}

function detailKey(dataSourceId: string) {
	return ["data-sources", dataSourceId] as const
}

function makeDataSource(overrides: Partial<DataSource> = {}): DataSource {
	return {
		id: DS_ID,
		name: "Prometheus",
		type: DataSourceType.Prometheus,
		url: "https://prom.test",
		status: DataSourceStatus.Success,
		createdAt: "2026-01-01T00:00:00.000Z",
		updatedAt: null,
		...overrides,
	}
}

function makeCreateRequest(): DataSourceCreateRequest {
	return {
		type: DataSourceType.Prometheus,
		name: "New Prometheus",
		url: "https://new-prom.test",
		credentials: { username: "u1", password: "p1" },
	}
}

function makeQueryParams(): GenericQueryParams {
	return {
		q: "up",
		chartType: GenericQueryChartType.Line,
		from: "2026-01-01T00:00:00.000Z",
		to: "2026-01-02T00:00:00.000Z",
		timeRangeKey: "24h",
	}
}

function makeMultiQueryParams(): GenericMultipleQueriesParams {
	return {
		queries: ["up", "down"],
		chartType: GenericQueryChartType.Line,
		from: "2026-01-01T00:00:00.000Z",
		to: "2026-01-02T00:00:00.000Z",
		timeRangeKey: "24h",
	}
}

function makeQueryResult(value: number): GenericQueryResult {
	return {
		status: GenericQueryResultStatus.Ok,
		data: [{ labels: { job: "api" }, metrics: [[1767225600, value]] }],
	}
}

// creating the composable eagerly loads the data source list once (empty
// cache); refresh() joins that in-flight load instead of forcing a second
// request. Tests that are not about the list seed the list cache first so
// the eager load is skipped, and pin that with a zero-count assertion.
describe("useDataSourceAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchDataSources", () => {
		it("fetches the data sources", async ({ expect }) => {
			const dataSource = makeDataSource()
			const listCalls = mockEndpoint("GET", LIST_URL, () => [dataSource])
			const api = makeDataSourceAPI()

			const result = await api.fetchDataSources.refresh()

			expect(result.data).toEqual([dataSource])
			expect(listCalls).toHaveLength(1)
		})

		it("returns an empty list when the response body is null", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", LIST_URL, () => null)
			const api = makeDataSourceAPI()

			const result = await api.fetchDataSources.refresh()

			expect(result.data).toEqual([])
			expect(listCalls).toHaveLength(1)
		})
	})

	describe("useFetchDataSourceById", () => {
		it("returns null without a data source id", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const detailCalls = mockEndpoint("GET", DETAIL_URL, () =>
				makeDataSource(),
			)
			const api = makeDataSourceAPI()
			const dataSource = runInApp(() => api.useFetchDataSourceById(null))

			const result = await dataSource.refresh()

			expect(result.data).toBeNull()
			expect(detailCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("fetches the data source by id", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const expected = makeDataSource()
			const detailCalls = mockEndpoint("GET", DETAIL_URL, () => expected)
			const api = makeDataSourceAPI()
			const dataSource = runInApp(() => api.useFetchDataSourceById(DS_ID))

			const result = await dataSource.refresh()

			expect(result.data).toEqual(expected)
			expect(detailCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("testDataSourceConnection", () => {
		it("returns null without a data source id", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const connectionCalls = mockEndpoint("GET", CONNECTION_URL, () => ({
				status: "success",
			}))
			const api = makeDataSourceAPI()
			const connection = runInApp(() => api.testDataSourceConnection(null))

			const result = await connection.refresh()

			expect(result.data).toBeNull()
			expect(connectionCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("fetches the connection status of the data source", async ({
			expect,
		}) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const connectionCalls = mockEndpoint("GET", CONNECTION_URL, () => ({
				status: "success",
			}))
			const api = makeDataSourceAPI()
			// staleTime is 0 for this query, so refresh() must join the
			// eager load immediately — an await in between would let it
			// finish and go stale, forcing a second request
			const connection = runInApp(() => api.testDataSourceConnection(DS_ID))

			const result = await connection.refresh()

			expect(result.data).toEqual({ status: "success" })
			expect(connectionCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("createDataSource", () => {
		it("inserts the data source optimistically and refreshes the list on success", async ({
			expect,
		}) => {
			const request = makeCreateRequest()
			const existing = makeDataSource()
			const created = makeDataSource({
				id: OTHER_DS_ID,
				name: request.name,
				url: request.url,
			})
			seedQueryData(LIST_KEY, [existing])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [created, existing])

			let resolveCreate: (value: unknown) => void = () => undefined
			let createReached: () => void = () => undefined
			const createReachedSignal = new Promise<void>((resolve) => {
				createReached = resolve
			})
			const createCalls = mockEndpoint("POST", LIST_URL, () => {
				createReached()

				return new Promise((resolve) => {
					resolveCreate = resolve
				})
			})
			const api = makeDataSourceAPI()

			const pending = api.createDataSource.mutateAsync(request)
			await createReachedSignal

			// the optimistic insert is prepended with a local nanoid id and
			// a pending status until the server confirms
			expect(readQueryData(LIST_KEY)).toEqual([
				{
					id: ANY_STRING,
					name: request.name,
					type: request.type,
					url: request.url,
					status: DataSourceStatus.LocalOptimisticInsert,
					createdAt: ANY_DATE,
					updatedAt: null,
				},
				existing,
			])
			resolveCreate(created)

			await pending

			expect(createCalls).toHaveLength(1)
			expect(createCalls[0]?.body).toEqual(request)
			// the success invalidation refetches the active list query
			expect(listCalls).toHaveLength(1)
			expect(api.fetchDataSources.data.value).toEqual([created, existing])
		})

		it("rolls back the optimistic insert when the request fails", async ({
			expect,
		}) => {
			const existing = makeDataSource()
			seedQueryData(LIST_KEY, [existing])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [existing])
			const createCalls = mockEndpoint("POST", LIST_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeDataSourceAPI()

			await expect(
				api.createDataSource.mutateAsync(makeCreateRequest()),
			).rejects.toThrow()

			expect(createCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(api.fetchDataSources.data.value).toEqual([existing])
		})

		it("skips the rollback when the list changed after the optimistic insert", async ({
			expect,
		}) => {
			const existing = makeDataSource()
			const divergent = makeDataSource({ id: OTHER_DS_ID, name: "Divergent" })
			seedQueryData(LIST_KEY, [existing])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [existing])

			let rejectCreate: (err: unknown) => void = () => undefined
			let createReached: () => void = () => undefined
			const createReachedSignal = new Promise<void>((resolve) => {
				createReached = resolve
			})
			const createCalls = mockEndpoint("POST", LIST_URL, () => {
				createReached()

				return new Promise((_resolve, reject) => {
					rejectCreate = reject
				})
			})
			const api = makeDataSourceAPI()

			const pending = api.createDataSource.mutateAsync(makeCreateRequest())
			await createReachedSignal

			// divergent data written after the optimistic insert must
			// survive the failure
			seedQueryData(LIST_KEY, [divergent])
			rejectCreate(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()

			expect(createCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(api.fetchDataSources.data.value).toEqual([divergent])
		})
	})

	describe("updateDataSource", () => {
		it("sends the update without touching the caches for an optimistic-insert id", async ({
			expect,
		}) => {
			const optimistic = makeDataSource({
				id: OPTIMISTIC_ID,
				status: DataSourceStatus.LocalOptimisticInsert,
			})
			seedQueryData(LIST_KEY, [optimistic])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const updateCalls = mockEndpoint(
				"PUT",
				`${LIST_URL}/${OPTIMISTIC_ID}`,
				() => makeDataSource(),
			)
			const api = makeDataSourceAPI()

			await api.updateDataSource.mutateAsync({
				dataSourceId: OPTIMISTIC_ID,
				req: { name: "Renamed", url: "https://renamed.test" },
			})

			expect(updateCalls).toHaveLength(1)
			expect(updateCalls[0]?.body).toEqual({
				name: "Renamed",
				url: "https://renamed.test",
			})
			expect(listCalls).toHaveLength(0)
			expect(api.fetchDataSources.data.value).toEqual([optimistic])
		})

		it("updates the caches optimistically and refreshes them on success", async ({
			expect,
		}) => {
			const target = makeDataSource()
			const other = makeDataSource({ id: OTHER_DS_ID, name: "Other" })
			const updated = makeDataSource({
				name: "Renamed",
				url: "https://renamed.test",
				updatedAt: "2026-02-01T00:00:00.000Z",
			})
			seedQueryData(LIST_KEY, [target, other])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [updated, other])
			const detailCalls = mockEndpoint("GET", DETAIL_URL, () => updated)

			let resolveUpdate: (value: unknown) => void = () => undefined
			let updateReached: () => void = () => undefined
			const updateReachedSignal = new Promise<void>((resolve) => {
				updateReached = resolve
			})
			const updateCalls = mockEndpoint("PUT", DETAIL_URL, () => {
				updateReached()

				return new Promise((resolve) => {
					resolveUpdate = resolve
				})
			})
			const api = makeDataSourceAPI()
			const dataSource = runInApp(() => api.useFetchDataSourceById(DS_ID))

			const pending = api.updateDataSource.mutateAsync({
				dataSourceId: DS_ID,
				req: { name: "Renamed", url: "https://renamed.test" },
			})
			await updateReachedSignal

			expect(readQueryData(LIST_KEY)).toEqual([
				{
					...target,
					name: "Renamed",
					url: "https://renamed.test",
					updatedAt: ANY_DATE,
				},
				other,
			])
			expect(readQueryData(detailKey(DS_ID))).toEqual({
				...target,
				name: "Renamed",
				url: "https://renamed.test",
				updatedAt: ANY_DATE,
			})
			resolveUpdate(updated)

			await pending

			expect(updateCalls).toHaveLength(1)
			expect(updateCalls[0]?.body).toEqual({
				name: "Renamed",
				url: "https://renamed.test",
			})
			// key filters prefix-match, so invalidating the list key also
			// refetches the active detail query once, and the explicit detail
			// invalidation refetches it a second time
			expect(listCalls).toHaveLength(1)
			expect(detailCalls).toHaveLength(2)
			expect(api.fetchDataSources.data.value).toEqual([updated, other])
			expect(dataSource.data.value).toEqual(updated)
		})

		it("updates only the fields present in the request", async ({ expect }) => {
			const target = makeDataSource()
			seedQueryData(LIST_KEY, [target])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [target])

			let resolveUpdate: (value: unknown) => void = () => undefined
			let updateReached: () => void = () => undefined
			const updateReachedSignal = new Promise<void>((resolve) => {
				updateReached = resolve
			})
			const updateCalls = mockEndpoint("PUT", DETAIL_URL, () => {
				updateReached()

				return new Promise((resolve) => {
					resolveUpdate = resolve
				})
			})
			const api = makeDataSourceAPI()

			const pending = api.updateDataSource.mutateAsync({
				dataSourceId: DS_ID,
				req: { name: "Renamed" },
			})
			await updateReachedSignal

			// url is not part of the request, so it keeps its cached value
			expect(readQueryData(LIST_KEY)).toEqual([
				{ ...target, name: "Renamed", updatedAt: ANY_DATE },
			])
			resolveUpdate(makeDataSource({ name: "Renamed" }))

			await pending

			expect(updateCalls).toHaveLength(1)
			expect(updateCalls[0]?.body).toEqual({ name: "Renamed" })
			// the detail cache was empty, so only the list is invalidated
			expect(listCalls).toHaveLength(1)
		})

		it("rolls back the list and detail caches when the request fails", async ({
			expect,
		}) => {
			const target = makeDataSource()
			seedQueryData(LIST_KEY, [target])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [target])
			const updateCalls = mockEndpoint("PUT", DETAIL_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeDataSourceAPI()

			await expect(
				api.updateDataSource.mutateAsync({
					dataSourceId: DS_ID,
					req: { name: "Renamed", url: "https://renamed.test" },
				}),
			).rejects.toThrow()

			expect(updateCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(readQueryData(LIST_KEY)).toEqual([target])
			expect(readQueryData(detailKey(DS_ID))).toEqual(target)
		})

		it("skips the list rollback when the list changed after the optimistic update", async ({
			expect,
		}) => {
			const target = makeDataSource()
			const divergent = makeDataSource({ id: OTHER_DS_ID, name: "Divergent" })
			seedQueryData(LIST_KEY, [target])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [target])

			let rejectUpdate: (err: unknown) => void = () => undefined
			let updateReached: () => void = () => undefined
			const updateReachedSignal = new Promise<void>((resolve) => {
				updateReached = resolve
			})
			const updateCalls = mockEndpoint("PUT", DETAIL_URL, () => {
				updateReached()

				return new Promise((_resolve, reject) => {
					rejectUpdate = reject
				})
			})
			const api = makeDataSourceAPI()

			const pending = api.updateDataSource.mutateAsync({
				dataSourceId: DS_ID,
				req: { name: "Renamed", url: "https://renamed.test" },
			})
			await updateReachedSignal

			// the diverged list must survive the failure, while the
			// untouched detail cache is still rolled back
			seedQueryData(LIST_KEY, [divergent])
			rejectUpdate(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()

			expect(updateCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(readQueryData(LIST_KEY)).toEqual([divergent])
			expect(readQueryData(detailKey(DS_ID))).toEqual(target)
		})
	})

	describe("deleteDataSource", () => {
		it("sends the delete without touching the caches for an optimistic-insert id", async ({
			expect,
		}) => {
			const optimistic = makeDataSource({
				id: OPTIMISTIC_ID,
				status: DataSourceStatus.LocalOptimisticInsert,
			})
			seedQueryData(LIST_KEY, [optimistic])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const deleteCalls = mockEndpoint(
				"DELETE",
				`${LIST_URL}/${OPTIMISTIC_ID}`,
				() => null,
			)
			const api = makeDataSourceAPI()

			await api.deleteDataSource.mutateAsync(OPTIMISTIC_ID)

			expect(deleteCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(api.fetchDataSources.data.value).toEqual([optimistic])
		})

		it("removes the data source optimistically and refreshes the caches on success", async ({
			expect,
		}) => {
			const target = makeDataSource()
			const other = makeDataSource({ id: OTHER_DS_ID, name: "Other" })
			seedQueryData(LIST_KEY, [target, other])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [other])
			const detailCalls = mockEndpoint("GET", DETAIL_URL, () => target)

			let resolveDelete: (value: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})
			const deleteCalls = mockEndpoint("DELETE", DETAIL_URL, () => {
				deleteReached()

				return new Promise((resolve) => {
					resolveDelete = resolve
				})
			})
			const api = makeDataSourceAPI()
			const dataSource = runInApp(() => api.useFetchDataSourceById(DS_ID))

			const pending = api.deleteDataSource.mutateAsync(DS_ID)
			await deleteReachedSignal

			expect(readQueryData(LIST_KEY)).toEqual([other])
			expect(readQueryData(detailKey(DS_ID))).toBeUndefined()
			resolveDelete(null)

			await pending

			expect(deleteCalls).toHaveLength(1)
			// key filters prefix-match, so invalidating the list key also
			// refetches the active detail query once, and the explicit detail
			// invalidation refetches it a second time
			expect(listCalls).toHaveLength(1)
			expect(detailCalls).toHaveLength(2)
			expect(api.fetchDataSources.data.value).toEqual([other])
			expect(dataSource.data.value).toEqual(target)
		})

		it("rolls back the list and detail caches when the request fails", async ({
			expect,
		}) => {
			const target = makeDataSource()
			seedQueryData(LIST_KEY, [target])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [target])
			const deleteCalls = mockEndpoint("DELETE", DETAIL_URL, () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeDataSourceAPI()

			await expect(api.deleteDataSource.mutateAsync(DS_ID)).rejects.toThrow()

			expect(deleteCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(readQueryData(LIST_KEY)).toEqual([target])
			expect(readQueryData(detailKey(DS_ID))).toEqual(target)
		})

		it("skips the rollback when the caches changed after the optimistic removal", async ({
			expect,
		}) => {
			const target = makeDataSource()
			const divergent = makeDataSource({ id: OTHER_DS_ID, name: "Divergent" })
			const divergentDetail = makeDataSource({ name: "Divergent Detail" })
			seedQueryData(LIST_KEY, [target])
			seedQueryData(detailKey(DS_ID), target)
			const listCalls = mockEndpoint("GET", LIST_URL, () => [target])

			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})
			const deleteCalls = mockEndpoint("DELETE", DETAIL_URL, () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeDataSourceAPI()

			const pending = api.deleteDataSource.mutateAsync(DS_ID)
			await deleteReachedSignal

			// divergent data written after the optimistic removal must
			// survive the failure in both caches
			seedQueryData(LIST_KEY, [divergent])
			seedQueryData(detailKey(DS_ID), divergentDetail)
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()

			expect(deleteCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(readQueryData(LIST_KEY)).toEqual([divergent])
			expect(readQueryData(detailKey(DS_ID))).toEqual(divergentDetail)
		})
	})

	describe("useGenericQuery", () => {
		it("returns null without query parameters", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const queryCalls = mockEndpoint("GET", QUERY_URL, () =>
				makeQueryResult(1),
			)
			const api = makeDataSourceAPI()
			const query = runInApp(() => api.useGenericQuery(DS_ID, null))

			const result = await query.refresh()

			expect(result.data).toBeNull()
			expect(queryCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("fetches the query result with the time range parameters", async ({
			expect,
		}) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const expected = makeQueryResult(1)
			const queryCalls = mockEndpoint("GET", QUERY_URL, () => expected)
			const api = makeDataSourceAPI()
			const query = runInApp(() =>
				api.useGenericQuery(DS_ID, makeQueryParams()),
			)

			const result = await query.refresh()

			expect(result.data).toEqual(expected)
			expect(queryCalls).toHaveLength(1)
			expect(queryCalls[0]?.query).toEqual({
				q: "up",
				chartType: "line_chart",
				from: "2026-01-01T00:00:00.000Z",
				to: "2026-01-02T00:00:00.000Z",
			})
			expect(listCalls).toHaveLength(0)
		})

		it("returns a query-error result when the api reports a query error", async ({
			expect,
		}) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const queryCalls = mockEndpoint("GET", QUERY_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { message: "bad query", code: "query.error" }
			})
			const api = makeDataSourceAPI()
			const query = runInApp(() =>
				api.useGenericQuery(DS_ID, makeQueryParams()),
			)

			const result = await query.refresh()

			expect(result.data).toEqual({
				status: GenericQueryResultStatus.QueryError,
				data: [],
				queryErrorMessage: "bad query",
			})
			expect(result.error).toBeNull()
			expect(queryCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
		})

		it("reports a non-query error through the error state", async ({
			expect,
		}) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const successCalls = mockEndpoint("GET", QUERY_URL, () =>
				makeQueryResult(1),
			)
			const api = makeDataSourceAPI()
			const query = runInApp(() =>
				api.useGenericQuery(DS_ID, makeQueryParams()),
			)
			// a failing query cannot dedupe the eager creation load into the
			// act, so the entry is warmed with a success first and the failure
			// is a single forced refetch through a later-wins registration
			await query.refresh()
			const failedCalls = mockEndpoint("GET", QUERY_URL, () => {
				throw createError({ statusCode: 500 })
			})

			const result = await query.refetch()

			expect(result.error).toMatchObject({ statusCode: 500 })
			// a non-"query.error" failure keeps the previously fetched result
			// instead of mapping to a QueryError result
			expect(query.data.value).toEqual(makeQueryResult(1))
			expect(successCalls).toHaveLength(1)
			// ofetch retries a failed GET once by default on a 500 response
			expect(failedCalls).toHaveLength(2)
			expect(listCalls).toHaveLength(0)
		})
	})

	describe("useMultipleGenericQueries", () => {
		it("returns no results without query parameters", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const queryCalls = mockEndpoint("GET", QUERY_URL, () =>
				makeQueryResult(1),
			)
			const api = makeDataSourceAPI()
			const queries = runInApp(() =>
				api.useMultipleGenericQueries(DS_ID, null, false),
			)

			const result = await queries.refresh()

			expect(result.data).toEqual([])
			expect(queryCalls).toHaveLength(0)
			expect(listCalls).toHaveLength(0)
		})

		it("fetches one result per query in order", async ({ expect }) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const upResult = makeQueryResult(1)
			const downResult = makeQueryResult(2)
			const queryCalls = mockEndpoint("GET", QUERY_URL, (call) =>
				call.query.q === "up" ? upResult : downResult,
			)
			const api = makeDataSourceAPI()
			const queries = runInApp(() =>
				api.useMultipleGenericQueries(DS_ID, makeMultiQueryParams(), false),
			)

			const result = await queries.refresh()

			expect(result.data).toEqual([upResult, downResult])
			expect(queryCalls).toHaveLength(2)
			expect(queryCalls[0]?.query).toEqual({
				q: "up",
				chartType: "line_chart",
				from: "2026-01-01T00:00:00.000Z",
				to: "2026-01-02T00:00:00.000Z",
			})
			expect(queryCalls[1]?.query).toEqual({
				q: "down",
				chartType: "line_chart",
				from: "2026-01-01T00:00:00.000Z",
				to: "2026-01-02T00:00:00.000Z",
			})
			expect(listCalls).toHaveLength(0)
		})

		it("turns a failing query into a query-error result while the others succeed", async ({
			expect,
		}) => {
			seedQueryData(LIST_KEY, [])
			const listCalls = mockEndpoint("GET", LIST_URL, () => [])
			const upResult = makeQueryResult(1)
			const queryCalls = mockEndpoint("GET", QUERY_URL, (call, event) => {
				if (call.query.q === "down") {
					setResponseStatus(event, 400)

					return { message: "down failed", code: "query.error" }
				}

				return upResult
			})
			const api = makeDataSourceAPI()
			const queries = runInApp(() =>
				api.useMultipleGenericQueries(DS_ID, makeMultiQueryParams(), false),
			)

			const result = await queries.refresh()

			expect(result.data).toEqual([
				upResult,
				{
					status: GenericQueryResultStatus.QueryError,
					data: [],
					queryErrorMessage: "down failed",
				},
			])
			expect(result.error).toBeNull()
			expect(queryCalls).toHaveLength(2)
			expect(listCalls).toHaveLength(0)
		})
	})
})
