const METRIC_QUERY_KEYS = {
	metadata: (dataSourceId: string) =>
		["metrics", dataSourceId, "metadata"] as const,
	labels: (
		dataSourceId: string,
		timeRange: string,
		matchers: string,
		variableValues: Record<string, string>,
	) =>
		[
			"metrics",
			dataSourceId,
			"labels",
			timeRange,
			matchers,
			variableValues,
		] as const,
	labelValues: (
		dataSourceId: string,
		label: string,
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		[
			"metrics",
			dataSourceId,
			"label-values",
			label,
			timeRange,
			variableValues,
		] as const,
	series: (
		dataSourceId: string,
		matchers: string,
		timeRange: string,
		variableValues: Record<string, string>,
	) =>
		[
			"metrics",
			dataSourceId,
			"series",
			matchers,
			timeRange,
			variableValues,
		] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	function useMetricMetadata(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => METRIC_QUERY_KEYS.metadata(toValue(dataSourceIdRef) || ""),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				if (!dataSourceId) {
					return null
				}

				return await $coreAPIClient<PrometheusMetadataResult>(
					`/api/data-sources/${dataSourceId}/metrics/metadata`,
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

	function useMetricLabels(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusLabelParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
		staleTimeMs: number = 5 * 1000, // 5 secs
	) {
		return useQuery({
			key: () =>
				METRIC_QUERY_KEYS.labels(
					toValue(dataSourceIdRef) || "",
					toValue(paramsRef)?.timeRangeKey || "",
					(toValue(paramsRef)?.matchers ?? []).join(","),
					toValue(paramsRef)?.variableValues ?? {},
				),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.matchers?.length) {
					return null
				}

				const queryParams = new URLSearchParams({
					...formatQueryTimeRange(params),
					...formatQueryVariableValues(params.variableValues),
				})

				for (const matcher of params.matchers) {
					queryParams.append("matchers", matcher)
				}

				const url = queryParams.toString()
					? `/api/data-sources/${dataSourceId}/metrics/labels?${queryParams.toString()}`
					: `/api/data-sources/${dataSourceId}/metrics/labels`

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

	function useMetricLabelValues(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusLabelValuesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return METRIC_QUERY_KEYS.labelValues(
					dataSourceId || "",
					params?.label || "",
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.label) {
					return null
				}

				const queryParams = new URLSearchParams({
					...formatQueryTimeRange(params),
					...formatQueryVariableValues(params.variableValues),
				})

				if (params.matchers?.length) {
					for (const matcher of params.matchers) {
						queryParams.append("matchers", matcher)
					}
				}

				const url = queryParams.toString()
					? `/api/data-sources/${dataSourceId}/metrics/labels/${params.label}/values?${queryParams.toString()}`
					: `/api/data-sources/${dataSourceId}/metrics/labels/${params.label}/values`

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

	function useMetricSeries(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<PrometheusSeriesParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
		staleTimeMs: number = 5 * 1000, // 5 secs
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return METRIC_QUERY_KEYS.series(
					dataSourceId || "",
					params?.matchers.join(",") || "",
					params?.timeRangeKey || "",
					params?.variableValues ?? {},
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.matchers.length) {
					return null
				}

				const queryParams = new URLSearchParams({
					...formatQueryTimeRange(params),
					...formatQueryVariableValues(params.variableValues),
				})

				for (const matcher of params.matchers) {
					queryParams.append("matchers", matcher)
				}

				return await $coreAPIClient<PrometheusSeriesResult>(
					`/api/data-sources/${dataSourceId}/metrics/series?${queryParams.toString()}`,
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
		useMetricMetadata,
		useMetricLabels,
		useMetricLabelValues,
		useMetricSeries,
	}
}
