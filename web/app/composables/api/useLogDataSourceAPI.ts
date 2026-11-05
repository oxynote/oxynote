import {
	mergeBounds,
	pastIn,
	timeNanos,
	type QueryLogs,
} from "~/components/editor/blocks/logs/rows"

const LOG_QUERY_KEYS = {
	lines: (
		dataSourceId: string,
		queries: string[],
		timeRange: string,
		limit: number | null,
		order: LogOrder,
		variableValues: Record<string, string>,
	) =>
		[
			"logs",
			dataSourceId,
			"lines",
			...queries,
			timeRange,
			limit,
			order,
			variableValues,
		] as const,
	volume: (
		dataSourceId: string,
		queries: string[],
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		[
			"logs",
			dataSourceId,
			"volume",
			...queries,
			timeRange,
			variableValues,
		] as const,
	fields: (
		dataSourceId: string,
		query: string,
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		["logs", dataSourceId, "fields", query, timeRange, variableValues] as const,
	fieldValues: (
		dataSourceId: string,
		field: string,
		query: string,
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		[
			"logs",
			dataSourceId,
			"field-values",
			field,
			query,
			timeRange,
			variableValues,
		] as const,
}

// a refresh reads each query in one request, with a limit of this many
// pages. More logs may have arrived than that request can hold. Newest
// first, the read then replaces the list, so no gap is left below it.
// Oldest first, the rest load as the next page when the reader scrolls
// down.
const REFRESH_PAGES = 5

// how far before the end of the last read a refresh starts reading. Logs
// that reach the source this late are still caught.
const REFRESH_LOOKBACK_MS = 30_000

// the page size core uses when a request names none
const DEFAULT_LOGS_LIMIT = 100

// what a page after the first one needs to load. Each query has a cursor
// that says where its page starts. A query without a cursor is skipped. A
// previous page loads logs that were dropped from the top of the list.
// So it reads in the other order, going up from the top.
interface LogsPageParam {
	cursors: (string | null)[]
	window: Record<string, string>
	previous?: boolean
}

// canRunLogQueries reports whether a block has a query and none is blank.
// Exported because a caller deciding whether to fetch has to ask the same
// question the queries ask.
export function canRunLogQueries(
	queries: string[] | null | undefined,
): queries is string[] {
	return !!queries?.length && queries.every((q) => q.trim().length > 0)
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	function formatLogQueryParams(
		params: LogFieldsParams,
	): Record<string, string> {
		return {
			...(params.q ? { q: params.q } : {}),
			...formatQueryTimeRange(params),
			...formatQueryVariableValues(params.variableValues),
		}
	}

	// useLogs reads the logs of every query one page at a time. The
	// first page is the top of the list. The next page continues below the
	// logs already loaded. The previous page reloads logs that were
	// dropped from the top to save memory.
	//
	// A relative time range such as "last hour" keeps moving while the
	// reader scrolls. So every page reads the time range of the first page.
	// Otherwise the logs at the old start of the range would never load.
	function useLogs(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogsParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		const queryCache = useQueryCache()
		const key = () => {
			const params = toValue(paramsRef)

			return LOG_QUERY_KEYS.lines(
				toValue(dataSourceIdRef) || "",
				params?.queries ?? [],
				params?.timeRangeKey || "",
				params?.limit ?? null,
				params?.order ?? LogOrder.Desc,
				params?.variableValues ?? {},
			)
		}
		const query = useInfiniteQuery({
			key: key,
			query: async ({ pageParam }): Promise<LogsPage | null> => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params || !canRunLogQueries(params.queries)) {
					return null
				}

				const window = pageParam?.window ?? formatQueryTimeRange(params)
				const results = await Promise.all(
					params.queries.map(async (q, index) => {
						const cursor = pageParam?.cursors[index]

						if (pageParam && !cursor) {
							return null
						}

						if (pageParam?.previous) {
							const res = await readLogs(
								dataSourceId,
								params,
								q,
								window,
								cursor,
								oppositeOrder(params.order ?? LogOrder.Desc),
							)

							// this result was read in the other order. So its logs are
							// reversed, and its two cursors swap places.
							return {
								...res,
								lines: [...res.lines].reverse(),
								nextCursor: res.previousCursor,
								previousCursor: res.nextCursor,
							}
						}

						const res = await readLogs(
							dataSourceId,
							params,
							q,
							window,
							cursor,
							params.order,
						)

						// the first page is the top of the list. Nothing sits above it yet.
						return pageParam ? res : { ...res, previousCursor: null }
					}),
				)

				return { results: results, window: window }
			},
			initialPageParam: null as LogsPageParam | null,
			getNextPageParam: (_lastPage, pages) =>
				nextPageParam(pages, toValue(paramsRef)),
			getPreviousPageParam: (_firstPage, pages) =>
				previousPageParam(pages, toValue(paramsRef)),
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				canRunLogQueries(toValue(paramsRef)?.queries) &&
				toValue(enableFetch),
			// a narrowed or widened query keeps the logs it replaces until
			// its own arrive, so the view does not flash empty
			placeholderData: (previousData) => previousData,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			// log blocks dragged around the editor sometimes remount when
			// dropped, which would refresh their data; the long stale time
			// keeps that from happening
			staleTime: 60 * 60 * 24 * 1000, // 24 hours (longest refresh interval allowed)
			autoRefetch: false,
		})

		function setPages(pages: (LogsPage | null)[], pageParams: unknown[]) {
			queryCache.setQueryData(key(), { pages: pages, pageParams: pageParams })
		}

		// reload reads the top of the list again and drops all other pages.
		// We cannot just empty the cache first. A refetch reads as many
		// pages as the cache holds, so with no pages it would read nothing.
		// Instead we keep exactly one page and give it the param null. That
		// is the param of the very first read. The refetch then reads one
		// page from the top, and that new page replaces the kept one.
		async function reload() {
			const data = query.data.value
			if (data) {
				setPages(data.pages.slice(0, 1), [null])
			}

			return await query.refetch()
		}

		// loadNewer adds the logs that arrived since the last read. Newest
		// first, they go at the top of the list. Oldest first, they go at the
		// bottom. But an oldest-first query that still has unread pages is
		// skipped, because the new logs will come with those pages anyway.
		//
		// The read goes back a little before the last read ended. Logs often
		// reach a source after their own time, so the last read may have
		// missed some. The new logs form a page of their own, so a trim can
		// drop them like any other page. The other pages lose the logs the
		// read went back over, because the read holds newer copies of them.
		//
		// The time range moves forward with each refresh. So logs that are
		// now older than its start are dropped from the other end of the
		// list. Newest first, a burst of more new logs than one read holds
		// replaces the list with the newest logs.
		async function loadNewer() {
			const dataSourceId = toValue(dataSourceIdRef)
			const params = toValue(paramsRef)
			const data = query.data.value
			const lastTo = data?.pages[0]?.window.to
			const window = params ? formatQueryTimeRange(params) : undefined

			if (
				!dataSourceId ||
				!params ||
				!data ||
				!lastTo ||
				!window?.from ||
				!window.to ||
				query.isLoading.value
			) {
				return
			}

			const startKey = JSON.stringify(key())
			const asc = params.order === LogOrder.Asc
			const since = Math.max(
				Date.parse(lastTo) - REFRESH_LOOKBACK_MS,
				Date.parse(window.from),
			)
			const readParams = {
				...params,
				limit: (params.limit ?? DEFAULT_LOGS_LIMIT) * REFRESH_PAGES,
			}
			const sinceWindow = { from: new Date(since).toISOString(), to: window.to }
			const newer = await Promise.all(
				params.queries.map(async (q, index) => {
					// newest first, the read starts at the top of the window. Its
					// next cursor then points into the window, which is what the
					// pages below it need.
					if (!asc) {
						return await readLogs(
							dataSourceId,
							readParams,
							q,
							window,
							null,
							LogOrder.Desc,
						)
					}

					if (queryResults(data.pages, index).at(-1)?.nextCursor) {
						return null
					}

					return await readLogs(
						dataSourceId,
						readParams,
						q,
						sinceWindow,
						null,
						LogOrder.Asc,
					)
				}),
			)

			// the query or its pages changed while the read was waiting, so the
			// read no longer fits them. The next refresh reads from the same
			// point again.
			if (JSON.stringify(key()) !== startKey || query.data.value !== data) {
				return
			}

			const moved = { ...data.pages[0]?.window, ...window }
			const sinceNanos = BigInt(since) * 1_000_000n
			const burst =
				!asc &&
				newer.some((read) => {
					const oldest = read?.lines.at(-1)

					return (
						!!read?.nextCursor &&
						!!oldest &&
						timeNanos(oldest.time) > sinceNanos
					)
				})

			if (burst) {
				setPages(
					[
						{
							results: newer.map(
								(read) => read && { ...read, previousCursor: null },
							),
							window: moved,
						},
					],
					[null],
				)

				return
			}

			const pages = data.pages.map(
				(page) => page && { results: [...page.results], window: { ...moved } },
			)
			const pageParams = data.pageParams.map(
				(param) => param && { ...param, window: moved },
			)
			const fresh = newer.map((read, index) => {
				if (!read?.lines.length) {
					return null
				}

				return asc
					? stitchOldestFirst(pages, index, read)
					: stitchNewestFirst(pages, index, read)
			})

			if (fresh.some(Boolean)) {
				const page = { results: fresh, window: { ...moved } }

				if (asc) {
					pages.push(page)
					pageParams.push(null)
				} else {
					pages.unshift(page)
					pageParams.unshift(null)
				}
			}

			dropBefore(pages, window.from, asc)

			const kept = pages.flatMap((page, at) =>
				page?.results.some(Boolean) ? [at] : [],
			)

			setPages(
				kept.map((at) => pages[at] ?? null),
				kept.map((at) => pageParams[at] ?? null),
			)
		}

		// trimStart drops results from the top of the list. It stops once the
		// list holds at most maxLogs logs. The reader is moving down, so the
		// top is far from what they see. Each query always keeps its last
		// result. The result dropped first is the one whose last log sits
		// highest. That way the fewest visible logs go.
		function trimStart(maxLogs: number) {
			trim(maxLogs, "first")
		}

		// trimEnd drops results from the bottom of the list. It stops once the
		// list holds at most maxLogs logs. The reader is moving up, so the
		// bottom is far from what they see. Each query always keeps its first
		// result.
		function trimEnd(maxLogs: number) {
			trim(maxLogs, "last")
		}

		function trim(maxLogs: number, edge: "first" | "last") {
			const data = query.data.value
			const params = toValue(paramsRef)

			if (!data || !params) {
				return
			}

			const pages = data.pages.map(
				(page) => page && { ...page, results: [...page.results] },
			)
			const results = pages.flatMap((page, at) =>
				(page?.results ?? []).flatMap((result, index) => {
					// the result is ranked by its log nearest the rest of the list
					const log = edge === "first" ? result?.lines.at(-1) : result?.lines[0]

					return result
						? [
								{
									at: at,
									index: index,
									result: result,
									nanos: log ? timeNanos(log.time) : null,
								},
							]
						: []
				}),
			)
			let total = results.reduce((sum, r) => sum + r.result.lines.length, 0)

			if (total <= maxLogs) {
				return
			}

			// the results nearest the trimmed edge go first. An empty result has
			// no time, and it goes first too, because dropping it costs nothing.
			// Results with the same time are ordered by their page instead.
			const fromEdge = (edge === "first") === (params.order === LogOrder.Asc)

			results.sort((a, b) => {
				if (a.nanos === null || b.nanos === null) {
					return (a.nanos === null ? 0 : 1) - (b.nanos === null ? 0 : 1)
				}

				if (a.nanos === b.nanos) {
					return edge === "first" ? a.at - b.at : b.at - a.at
				}

				return a.nanos < b.nanos === fromEdge ? -1 : 1
			})

			// each query keeps its last result
			const left = Array.from(
				{ length: queryCount(pages) },
				(_, index) => results.filter((r) => r.index === index).length,
			)

			for (const { at, index, result } of results) {
				const page = pages[at]

				if (total <= maxLogs) {
					break
				}

				if (!page || (left[index] ?? 0) < 2) {
					continue
				}

				page.results[index] = null
				total -= result.lines.length
				left[index] = (left[index] ?? 0) - 1
			}

			const kept = pages.flatMap((page, at) =>
				page?.results.some(Boolean) ? [at] : [],
			)

			setPages(
				kept.map((at) => pages[at] ?? null),
				kept.map((at) => data.pageParams[at] ?? null),
			)
		}

		// colada checks for more pages only when it fetches. But trims and
		// refreshes change the pages without a fetch. So these two are worked
		// out from the pages each time.
		const hasNextPage = computed(
			() =>
				nextPageParam(query.data.value?.pages ?? [], toValue(paramsRef)) !==
				null,
		)
		const hasPreviousPage = computed(
			() =>
				previousPageParam(query.data.value?.pages ?? [], toValue(paramsRef)) !==
				null,
		)

		return {
			...query,
			hasNextPage: hasNextPage,
			hasPreviousPage: hasPreviousPage,
			reload: reload,
			loadNewer: loadNewer,
			trimStart: trimStart,
			trimEnd: trimEnd,
		}
	}

	// nextPageParam asks for the next page of each query with logs left
	// past its loaded ones. A query whose loaded logs already reach a page
	// past the end of the merged list is skipped. More of its logs would
	// only wait out of sight while the other queries catch up.
	function nextPageParam(
		pages: (LogsPage | null)[],
		params: LogsParams | null | undefined,
	): LogsPageParam | null {
		const last = pages.at(-1)
		const order = params?.order ?? LogOrder.Desc
		const limit = params?.limit ?? DEFAULT_LOGS_LIMIT
		const queries = loadedQueries(pages)
		const past = pastIn(order)
		const { frontier } = mergeBounds(queries, order)
		const cursors = queries.map((query, index) => {
			const cursor = queryResults(pages, index).at(-1)?.nextCursor ?? null
			const ahead =
				frontier === null
					? 0
					: query.logs.filter((log) => past(timeNanos(log.time), frontier))
							.length

			return ahead < limit ? cursor : null
		})

		return last && cursors.some(Boolean)
			? { cursors: cursors, window: last.window }
			: null
	}

	// previousPageParam does the same as nextPageParam above the loaded
	// logs
	function previousPageParam(
		pages: (LogsPage | null)[],
		params: LogsParams | null | undefined,
	): LogsPageParam | null {
		const first = pages[0]
		const order = params?.order ?? LogOrder.Desc
		const limit = params?.limit ?? DEFAULT_LOGS_LIMIT
		const queries = loadedQueries(pages)
		const past = pastIn(order)
		const { top } = mergeBounds(queries, order)
		const cursors = queries.map((query, index) => {
			const cursor = queryResults(pages, index)[0]?.previousCursor ?? null
			const ahead =
				top === null
					? 0
					: query.logs.filter((log) => past(top, timeNanos(log.time))).length

			return ahead < limit ? cursor : null
		})

		return first && cursors.some(Boolean)
			? { cursors: cursors, window: first.window, previous: true }
			: null
	}

	async function readLogs(
		dataSourceId: string,
		params: LogsParams,
		q: string,
		window: Record<string, string>,
		cursor: string | null | undefined,
		order: LogOrder | undefined,
	): Promise<LogsResult> {
		const queryParams = new URLSearchParams({
			q: q,
			...window,
			...formatQueryVariableValues(params.variableValues),
		})

		if (params.limit !== undefined) {
			queryParams.set("limit", String(params.limit))
		}

		if (order !== undefined) {
			queryParams.set("order", order)
		}

		if (cursor) {
			queryParams.set("cursor", cursor)
		}

		return await $coreAPIClient<LogsResult>(
			`/api/data-sources/${dataSourceId}/logs?${queryParams.toString()}`,
			{ method: "GET" },
		)
	}

	// counts the logs of every query per bucket, one result per query
	function useLogVolume(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogQueriesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)

				return LOG_QUERY_KEYS.volume(
					toValue(dataSourceIdRef) || "",
					params?.queries ?? [],
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params || !canRunLogQueries(params.queries)) {
					return null
				}

				return await Promise.all(
					params.queries.map(async (q) => {
						const queryParams = new URLSearchParams({
							q: q,
							...formatQueryTimeRange(params),
							...formatQueryVariableValues(params.variableValues),
						})

						return await $coreAPIClient<GenericQueryResult>(
							`/api/data-sources/${dataSourceId}/logs/volume?${queryParams.toString()}`,
							{ method: "GET" },
						)
					}),
				)
			},
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				canRunLogQueries(toValue(paramsRef)?.queries) &&
				toValue(enableFetch),
			// a narrowed or widened query keeps the logs it replaces until
			// its own arrive, so the view does not flash empty
			placeholderData: (previousData) => previousData,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			// log blocks dragged around the editor sometimes remount when
			// dropped, which would refresh their data; the long stale time
			// keeps that from happening
			staleTime: 60 * 60 * 24 * 1000, // 24 hours (longest refresh interval allowed)
			autoRefetch: false,
		})
	}

	function useLogFields(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogFieldsParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)

				return LOG_QUERY_KEYS.fields(
					toValue(dataSourceIdRef) || "",
					params?.q || "",
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params) {
					return null
				}

				const queryParams = new URLSearchParams(formatLogQueryParams(params))

				return await $coreAPIClient<LogFieldsResult>(
					`/api/data-sources/${dataSourceId}/logs/fields?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				!!toValue(paramsRef) &&
				toValue(enableFetch),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 5 * 1000, // 5 secs
			autoRefetch: false,
		})
	}

	function useLogFieldValues(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogFieldValuesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)

				return LOG_QUERY_KEYS.fieldValues(
					toValue(dataSourceIdRef) || "",
					params?.field || "",
					params?.q || "",
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.field) {
					return null
				}

				const queryParams = new URLSearchParams(formatLogQueryParams(params))

				return await $coreAPIClient<LogFieldValuesResult>(
					`/api/data-sources/${dataSourceId}/logs/fields/${encodeURIComponent(params.field)}/values?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				!!toValue(paramsRef)?.field &&
				toValue(enableFetch),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 5 * 1000, // 5 secs
			autoRefetch: false,
		})
	}

	return {
		useLogs,
		useLogVolume,
		useLogFields,
		useLogFieldValues,
	}
}

// the results of the query at index, from the top of the list down. A
// page that did not read this query has no result for it.
function queryResults(pages: (LogsPage | null)[], index: number): LogsResult[] {
	return pages.flatMap((page) => page?.results[index] ?? [])
}

function queryCount(pages: (LogsPage | null)[]): number {
	return pages.find((page) => page)?.results.length ?? 0
}

// loadedQueries gives the logs each query loaded, as the merge reads them
function loadedQueries(pages: (LogsPage | null)[]): QueryLogs[] {
	return Array.from({ length: queryCount(pages) }, (_, index) => {
		const results = queryResults(pages, index)

		return {
			logs: results.flatMap((result) => result.lines),
			hasMore: !!results.at(-1)?.nextCursor,
			hasPrevious: !!results[0]?.previousCursor,
		}
	})
}

// edgeResult finds the first or last result of the query at index. It
// also returns the page that result is on.
function edgeResult(
	pages: (LogsPage | null)[],
	index: number,
	edge: "first" | "last",
): [number | null, LogsResult | null] {
	const order = pages.map((_, at) => at)

	for (const at of edge === "first" ? order : order.reverse()) {
		const result = pages[at]?.results[index]
		if (result) {
			return [at, result]
		}
	}

	return [null, null]
}

// stitchNewestFirst fits a newest-first read from the top of the window
// onto the query's pages, and returns the read as the query's new top
// result. The read holds every log from its oldest one up, so the pages
// keep only the logs older than that. The top result left reads back up
// with the read's next cursor. That cursor marks where the read stopped,
// and read the other way it starts just above that result's logs. A read
// without a next cursor holds the whole window, so it replaces the pages.
function stitchNewestFirst(
	pages: (LogsPage | null)[],
	index: number,
	read: LogsResult,
): LogsResult {
	const oldest = timeNanos(read.lines.at(-1)?.time ?? "")
	let top = true

	for (const page of pages) {
		const result = page?.results[index]
		if (!page || !result) {
			continue
		}

		const logs = read.nextCursor
			? result.lines.filter((log) => timeNanos(log.time) < oldest)
			: []

		if (!logs.length) {
			page.results[index] = null

			continue
		}

		page.results[index] = {
			...result,
			status: resultStatus(logs),
			lines: logs,
			previousCursor: top ? read.nextCursor : result.previousCursor,
		}
		top = false
	}

	return { ...read, previousCursor: null }
}

// stitchOldestFirst fits an oldest-first read of the newest logs onto
// the query's pages, and returns the read as the query's new last result.
// The read holds every log from its first one on, so the pages keep only
// the logs older than that. The last result left reads on with the read's
// previous cursor. That cursor marks the read's first log, and read the
// other way it starts right there.
function stitchOldestFirst(
	pages: (LogsPage | null)[],
	index: number,
	read: LogsResult,
): LogsResult | null {
	const first = timeNanos(read.lines[0]?.time ?? "")
	let last: { page: LogsPage; result: LogsResult } | null = null

	for (const page of pages) {
		const result = page?.results[index]
		if (!page || !result) {
			continue
		}

		const logs = result.lines.filter((log) => timeNanos(log.time) < first)
		if (!logs.length) {
			page.results[index] = null

			continue
		}

		const kept = { ...result, status: resultStatus(logs), lines: logs }
		page.results[index] = kept
		last = { page: page, result: kept }
	}

	if (!last) {
		return read
	}

	// a read whose first log sits right at its window's start has no
	// previous cursor. That is rare, so its logs join the last result.
	if (!read.previousCursor) {
		last.page.results[index] = {
			...last.result,
			status: GenericQueryResultStatus.Ok,
			lines: [...last.result.lines, ...read.lines],
			nextCursor: read.nextCursor,
		}

		return null
	}

	last.page.results[index] = { ...last.result, nextCursor: read.previousCursor }

	return read
}

// dropBefore drops the logs that are older than from. Newest first,
// these logs are at the bottom of the list. Oldest first, they are at
// the top. A query that lost logs has nothing more to read on that
// side, so its cursor there is cleared.
function dropBefore(pages: (LogsPage | null)[], from: string, asc: boolean) {
	const start = timeNanos(from)

	for (let index = 0; index < queryCount(pages); index++) {
		let dropped = false

		for (const page of pages) {
			const result = page?.results[index]

			if (!page || !result) {
				continue
			}

			const logs = result.lines.filter((log) => timeNanos(log.time) >= start)

			if (logs.length < result.lines.length) {
				dropped = true
				page.results[index] = {
					...result,
					status: resultStatus(logs),
					lines: logs,
				}
			}
		}

		if (!dropped) {
			continue
		}

		const [at, result] = edgeResult(pages, index, asc ? "first" : "last")
		const page = at === null ? null : pages[at]

		if (page && result) {
			page.results[index] = asc
				? { ...result, previousCursor: null }
				: { ...result, nextCursor: null }
		}
	}
}

function resultStatus(
	logs: Log[],
): GenericQueryResultStatus.Ok | GenericQueryResultStatus.NoData {
	return logs.length > 0
		? GenericQueryResultStatus.Ok
		: GenericQueryResultStatus.NoData
}

function oppositeOrder(order: LogOrder): LogOrder {
	return order === LogOrder.Asc ? LogOrder.Desc : LogOrder.Asc
}
