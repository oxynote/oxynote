const PROMETHEUS_QUERY_KEYS = {
	query: (dataSourceId: string, query: string, timeRange: string) =>
		["prometheus", dataSourceId, "query", query, timeRange] as const,
	queries: (dataSourceId: string, queryStrings: string[], timeRange: string) =>
		[
			"prometheus",
			dataSourceId,
			"queries",
			...queryStrings,
			timeRange,
		] as const,
	metadata: (dataSourceId: string) =>
		["prometheus", dataSourceId, "metadata"] as const,
	labels: (dataSourceId: string, timeRange: string, matchers: string) =>
		["prometheus", dataSourceId, "labels", timeRange, matchers] as const,
	labelValues: (dataSourceId: string, label: string, timeRange: string) =>
		["prometheus", dataSourceId, "label-values", label, timeRange] as const,
	series: (dataSourceId: string, matchers: string, timeRange: string) =>
		["prometheus", dataSourceId, "series", matchers, timeRange] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	function usePrometheusQuery(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusQueryParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return PROMETHEUS_QUERY_KEYS.query(
					dataSourceId || "",
					params?.q || "",
					params?.timeRangeKey || "",
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.q) {
					return null
				}

				const queryParams = new URLSearchParams({
					q: params.q,
					...formatQueryTimeRange(params),
				})

				return await $coreAPIClient<PrometheusQueryResult>(
					`/api/data-sources/${dataSourceId}/prometheus/query?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const enable = toValue(enableFetch)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.q !== undefined &&
					params.q.trim().length > 0 &&
					enable
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 4.5 * 1000, // 4.5 secs
			autoRefetch: false,
		})
	}

	function usePrometheusMultipleQueries(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<
			PrometheusMultipleQueriesParams | null | undefined
		>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const queries = clone(params?.queries)

				return PROMETHEUS_QUERY_KEYS.queries(
					dataSourceId || "",
					queries?.sort() ?? [],
					params?.timeRangeKey || "",
				)
			},
			query: async (): Promise<PrometheusQueryResult[]> => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.queries.length) {
					return []
				}

				const timeRangeParams = formatQueryTimeRange(params)
				const results = await Promise.all(
					params.queries.map(async (q) => {
						const queryParams = new URLSearchParams({
							q: q,
							...timeRangeParams,
						})

						return await $coreAPIClient<PrometheusQueryResult>(
							`/api/data-sources/${dataSourceId}/prometheus/query?${queryParams.toString()}`,
							{ method: "GET" },
						)
					}),
				)

				return results
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const enable = toValue(enableFetch)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.queries !== undefined &&
					params.queries.length > 0 &&
					params.queries.every((q) => q.trim().length > 0) &&
					enable
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			autoRefetch: false,
			placeholderData: (previousData) => previousData, // to avoid "no data" state when params change
			// metric blocks dragged around the editor sometimes remount when
			// dropped, which would refresh their data; the long stale time
			// keeps that from happening
			staleTime: 60 * 60 * 24 * 1000, // 24 hours (longest refresh interval allowed)
		})
	}

	function usePrometheusMetadata(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => PROMETHEUS_QUERY_KEYS.metadata(toValue(dataSourceIdRef) || ""),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				if (!dataSourceId) {
					return null
				}

				return await $coreAPIClient<PrometheusMetadataResult>(
					`/api/data-sources/${dataSourceId}/prometheus/metadata`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				toValue(dataSourceIdRef) !== null &&
				toValue(dataSourceIdRef) !== undefined &&
				toValue(enableFetch),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 5 * 1000, // 5 secs
			autoRefetch: false,
		})
	}

	function usePrometheusLabels(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusLabelParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
		staleTimeMs: number = 5 * 1000, // 5 secs
	) {
		return useQuery({
			key: () =>
				PROMETHEUS_QUERY_KEYS.labels(
					toValue(dataSourceIdRef) || "",
					toValue(paramsRef)?.timeRangeKey || "",
					(toValue(paramsRef)?.matchers ?? []).join(","),
				),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.matchers?.length) {
					return null
				}

				const queryParams = new URLSearchParams(formatQueryTimeRange(params))

				for (const matcher of params.matchers) {
					queryParams.append("matchers", matcher)
				}

				const url = queryParams.toString()
					? `/api/data-sources/${dataSourceId}/prometheus/labels?${queryParams.toString()}`
					: `/api/data-sources/${dataSourceId}/prometheus/labels`

				return await $coreAPIClient<PrometheusLabelNamesResult>(url, {
					method: "GET",
				})
			},
			enabled: () =>
				toValue(dataSourceIdRef) !== null &&
				toValue(dataSourceIdRef) !== undefined &&
				toValue(enableFetch),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: staleTimeMs,
		})
	}

	function usePrometheusLabelValues(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusLabelValuesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return PROMETHEUS_QUERY_KEYS.labelValues(
					dataSourceId || "",
					params?.label || "",
					params?.timeRangeKey || "",
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.label) {
					return null
				}

				const queryParams = new URLSearchParams(formatQueryTimeRange(params))

				if (params.matchers?.length) {
					for (const matcher of params.matchers) {
						queryParams.append("matchers", matcher)
					}
				}

				const url = queryParams.toString()
					? `/api/data-sources/${dataSourceId}/prometheus/labels/${params.label}/values?${queryParams.toString()}`
					: `/api/data-sources/${dataSourceId}/prometheus/labels/${params.label}/values`

				return await $coreAPIClient<PrometheusLabelValuesResult>(url, {
					method: "GET",
				})
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const enable = toValue(enableFetch)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.label !== undefined &&
					enable
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 5 * 1000, // 5 secs
			autoRefetch: false,
		})
	}

	function usePrometheusSeries(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusSeriesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
		staleTimeMs: number = 5 * 1000, // 5 secs
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return PROMETHEUS_QUERY_KEYS.series(
					dataSourceId || "",
					params?.matchers.join(",") || "",
					params?.timeRangeKey || "",
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.matchers.length) {
					return null
				}

				const queryParams = new URLSearchParams(formatQueryTimeRange(params))

				for (const matcher of params.matchers) {
					queryParams.append("matchers", matcher)
				}

				return await $coreAPIClient<PrometheusSeriesResult>(
					`/api/data-sources/${dataSourceId}/prometheus/series?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const enable = toValue(enableFetch)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.matchers !== undefined &&
					params.matchers.length > 0 &&
					enable
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: staleTimeMs,
			autoRefetch: false,
		})
	}

	return {
		usePrometheusQuery,
		usePrometheusMultipleQueries,
		usePrometheusMetadata,
		usePrometheusLabels,
		usePrometheusLabelValues,
		usePrometheusSeries,
	}
}
