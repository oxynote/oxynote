const RELEASE_QUERY_KEYS = {
	latest: ["releases", "latest"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	function useFetchLatestRelease(enabledRef: MaybeRefOrGetter<boolean>) {
		return useQuery({
			key: RELEASE_QUERY_KEYS.latest,
			query: async () => {
				return await $coreAPIClient<LatestRelease | null>(
					`/api/releases/latest`,
					{ method: "GET" },
				)
			},
			enabled: () => toValue(enabledRef),
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 60 * 1000, // 3 hours
			autoRefetch: true,
		})
	}

	return { useFetchLatestRelease }
}
