const LOG_QUERY_KEYS = {
	lines: (
		dataSourceId: string,
		queries: string[],
		timeRange: string,
		limit: number | null,
		variableValues: Record<string, string>,
	) =>
		[
			"logs",
			dataSourceId,
			"lines",
			...queries,
			timeRange,
			limit,
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

// the next page's cursor of each query, null once a query has no older
// lines, with the window of the first page
interface LogLinesPageParam {
	cursors: (string | null)[]
	window: Record<string, string>
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

	// loads the newest page of every query first. Each further page holds
	// the lines older than a query's last page, for each query that has
	// any. A relative window such as the last hour moves while the reader
	// scrolls, so every further page reads the window of the first one, or
	// lines at its old start would never load.
	function useLogLines(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogLinesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useInfiniteQuery({
			key: () => {
				const params = toValue(paramsRef)

				return LOG_QUERY_KEYS.lines(
					toValue(dataSourceIdRef) || "",
					params?.queries ?? [],
					params?.timeRangeKey || "",
					params?.limit ?? null,
					params?.variableValues ?? {},
				)
			},
			query: async ({ pageParam }): Promise<LogLinesPage | null> => {
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

						const queryParams = new URLSearchParams({
							q: q,
							...window,
							...formatQueryVariableValues(params.variableValues),
						})

						if (params.limit !== undefined) {
							queryParams.set("limit", String(params.limit))
						}

						if (cursor) {
							queryParams.set("cursor", cursor)
						}

						return await $coreAPIClient<LogLinesResult>(
							`/api/data-sources/${dataSourceId}/logs?${queryParams.toString()}`,
							{ method: "GET" },
						)
					}),
				)

				return { results: results, window: window }
			},
			initialPageParam: null as LogLinesPageParam | null,
			getNextPageParam: (lastPage) => {
				const cursors = lastPage?.results.map((r) => r?.cursor ?? null) ?? []

				return lastPage && cursors.some(Boolean)
					? { cursors: cursors, window: lastPage.window }
					: null
			},
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				canRunLogQueries(toValue(paramsRef)?.queries) &&
				toValue(enableFetch),
			// a narrowed or widened query keeps the lines it replaces until
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

	// counts the lines of every query per bucket, one result per query
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
			// a narrowed or widened query keeps the lines it replaces until
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
		useLogLines,
		useLogVolume,
		useLogFields,
		useLogFieldValues,
	}
}
