const SQL_QUERY_KEYS = {
	metadata: (dataSourceId: string) =>
		["sql", dataSourceId, "metadata"] as const,
	labels: (dataSourceId: string, query: string, timeRange: string) =>
		["sql", dataSourceId, "labels", query, timeRange] as const,
}

export default function () {
	const { $apiClient } = useNuxtApp()

	function useSQLMetadata(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => SQL_QUERY_KEYS.metadata(toValue(dataSourceIdRef) || ""),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				if (!dataSourceId) {
					return null
				}

				return await $apiClient<SQLMetadataResult>(
					`/api/data-sources/${dataSourceId}/sql/metadata`,
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
			staleTime: 30 * 1000, // 30 secs
			autoRefetch: false,
		})
	}

	function useSQLLabels(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<SQLLabelsParams | null | undefined>,
		enableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () =>
				SQL_QUERY_KEYS.labels(
					toValue(dataSourceIdRef) || "",
					toValue(paramsRef)?.q || "",
					toValue(paramsRef)?.timeRangeKey || "",
				),
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

				return await $apiClient<SQLLabelsResult>(
					`/api/data-sources/${dataSourceId}/sql/query-labels?${queryParams.toString()}`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				toValue(dataSourceIdRef) !== null &&
				toValue(dataSourceIdRef) !== undefined &&
				toValue(paramsRef)?.q !== null &&
				toValue(paramsRef)?.q !== undefined &&
				toValue(enableFetch),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 30 * 1000, // 30 secs
			autoRefetch: false,
		})
	}

	return {
		useSQLMetadata,
		useSQLLabels,
	}
}
