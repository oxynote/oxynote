import { timeNanos } from "~/components/editor/blocks/logs/rows"

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

// a refresh reads only the logs that arrived since the last read. Each
// page of new logs costs one request. So after a burst of logs, one
// refresh could send hundreds of requests. This caps the pages it reads.
// Past the cap, the list is reloaded from the top instead. The cap only
// matters newest first. Oldest first, a refresh reads one page, and the
// rest load when the reader scrolls down.
const MAX_NEWER_PAGES = 5

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
			getNextPageParam: (_lastPage, pages) => nextPageParam(pages),
			getPreviousPageParam: (_firstPage, pages) => previousPageParam(pages),
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
		// The time range moves forward with each refresh. So logs that are
		// now older than its start are dropped from the other end of the
		// list. In newest first, a burst of more new logs than a few pages
		// hold reloads the list from the top instead.
		async function loadNewer() {
			const dataSourceId = toValue(dataSourceIdRef)
			const params = toValue(paramsRef)
			const data = query.data.value
			// the new logs start where the last read ended
			const from = data?.pages[0]?.window.to
			const window = params ? formatQueryTimeRange(params) : undefined
			const to = window?.to

			if (
				!dataSourceId ||
				!params ||
				!data ||
				!from ||
				!window?.from ||
				!to ||
				query.isLoading.value
			) {
				return
			}

			const asc = params.order === LogOrder.Asc
			const newer = await Promise.all(
				params.queries.map(async (q, index) => {
					if (asc && queryResults(data.pages, index).at(-1)?.nextCursor) {
						return null
					}

					return await readNewLogs(
						dataSourceId,
						params,
						q,
						{ from: from, to: to },
						asc ? 1 : MAX_NEWER_PAGES,
					)
				}),
			)

			// newest first, logs left after the last page mean a burst
			if (!asc && newer.some((added) => added?.nextCursor)) {
				return await reload()
			}

			const moved = { ...data.pages[0]?.window, ...window }
			const pages = data.pages.map(
				(page) => page && { results: [...page.results], window: { ...moved } },
			)

			newer.forEach((added, index) => {
				const [at, result] = edgeResult(pages, index, asc ? "last" : "first")
				const page = pages[at ?? (asc ? pages.length - 1 : 0)]

				if (!added || !page) {
					return
				}

				const logs = asc
					? [...(result?.lines ?? []), ...added.lines]
					: [...added.lines, ...(result?.lines ?? [])]

				// a query without a result yet takes the new one. Newest first,
				// it is the top of the list, so nothing sits above it.
				page.results[index] = {
					...(result ?? {
						...added,
						previousCursor: asc ? added.previousCursor : null,
					}),
					status: resultStatus(logs),
					lines: logs,
					nextCursor: asc ? added.nextCursor : (result?.nextCursor ?? null),
				}
			})
			dropBefore(pages, window.from, asc)
			setPages(
				pages,
				data.pageParams.map((param) => param && { ...param, window: moved }),
			)
		}

		// readNewLogs reads up to maxPages pages of one query in a row. The
		// result holds the logs of all of them. Its nextCursor is still set
		// when logs were left after the last page.
		async function readNewLogs(
			dataSourceId: string,
			params: LogsParams,
			q: string,
			window: Record<string, string>,
			maxPages: number,
		): Promise<LogsResult> {
			const first = await readLogs(
				dataSourceId,
				params,
				q,
				window,
				null,
				params.order,
			)
			const logs = [...first.lines]
			let last = first

			for (let page = 1; page < maxPages && last.nextCursor; page++) {
				last = await readLogs(
					dataSourceId,
					params,
					q,
					window,
					last.nextCursor,
					params.order,
				)
				logs.push(...last.lines)
			}

			return {
				...last,
				status: resultStatus(logs),
				lines: logs,
				previousCursor: first.previousCursor,
			}
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
			() => nextPageParam(query.data.value?.pages ?? []) !== null,
		)
		const hasPreviousPage = computed(
			() => previousPageParam(query.data.value?.pages ?? []) !== null,
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

	function nextPageParam(pages: (LogsPage | null)[]): LogsPageParam | null {
		const last = pages.at(-1)
		const cursors = Array.from(
			{ length: queryCount(pages) },
			(_, index) => queryResults(pages, index).at(-1)?.nextCursor ?? null,
		)

		return last && cursors.some(Boolean)
			? { cursors: cursors, window: last.window }
			: null
	}

	function previousPageParam(pages: (LogsPage | null)[]): LogsPageParam | null {
		const first = pages[0]
		const cursors = Array.from(
			{ length: queryCount(pages) },
			(_, index) => queryResults(pages, index)[0]?.previousCursor ?? null,
		)

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
