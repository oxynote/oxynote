const LOG_QUERY_KEYS = {
	lines: (
		dataSourceId: string,
		query: string,
		timeRange: string,
		limit: number | null,
		variableValues: Record<string, string>,
	) =>
		[
			"logs",
			dataSourceId,
			"lines",
			query,
			timeRange,
			limit,
			variableValues,
		] as const,
	volume: (
		dataSourceId: string,
		query: string,
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		["logs", dataSourceId, "volume", query, timeRange, variableValues] as const,
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

interface LogLinesPageParam {
	cursor: string
	window: Record<string, string>
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

	// loads the newest page first. Each further page holds the lines older
	// than the last one. A relative window such as the last hour moves
	// while the reader scrolls, so every further page reads the window of
	// the first one, or lines at its old start would never load.
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
					params?.q || "",
					params?.timeRangeKey || "",
					params?.limit ?? null,
					params?.variableValues ?? {},
				)
			},
			query: async ({ pageParam }): Promise<LogLinesPage | null> => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.q.trim()) {
					return null
				}

				const window = pageParam?.window ?? formatQueryTimeRange(params)
				const queryParams = new URLSearchParams({
					q: params.q,
					...window,
					...formatQueryVariableValues(params.variableValues),
				})

				if (params.limit !== undefined) {
					queryParams.set("limit", String(params.limit))
				}

				if (pageParam) {
					queryParams.set("cursor", pageParam.cursor)
				}

				const result = await $coreAPIClient<LogLinesResult>(
					`/api/data-sources/${dataSourceId}/logs?${queryParams.toString()}`,
					{ method: "GET" },
				)

				return { ...result, window: window }
			},
			initialPageParam: null as LogLinesPageParam | null,
			getNextPageParam: (lastPage) =>
				lastPage?.cursor
					? { cursor: lastPage.cursor, window: lastPage.window }
					: null,
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				!!toValue(paramsRef)?.q.trim() &&
				toValue(enableFetch),
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

	function useLogVolume(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<LogQueryParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)

				return LOG_QUERY_KEYS.volume(
					toValue(dataSourceIdRef) || "",
					params?.q || "",
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.q.trim()) {
					return null
				}

				const queryParams = new URLSearchParams(formatLogQueryParams(params))

				return await $coreAPIClient<GenericQueryResult>(
					`/api/data-sources/${dataSourceId}/logs/volume?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				!!toValue(dataSourceIdRef) &&
				!!toValue(paramsRef)?.q.trim() &&
				toValue(enableFetch),
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
