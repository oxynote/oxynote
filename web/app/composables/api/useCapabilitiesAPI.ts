const CAPABILITIES_QUERY_KEYS = {
	capabilities: ["capabilities"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	const fetchCapabilities = useQuery<Capabilities>({
		key: CAPABILITIES_QUERY_KEYS.capabilities,
		query: async () => {
			return await $coreAPIClient<Capabilities>(`/api/capabilities`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: Infinity,
	})

	function enabled(pick: (capabilities: Capabilities) => boolean) {
		return computed(() => {
			const capabilities = fetchCapabilities.data.value

			return capabilities ? pick(capabilities) : true
		})
	}

	const isGithubEnabled = enabled((c) => c.github)
	const isSlackEnabled = enabled((c) => c.slack)
	const isChangeDetectionEnabled = enabled((c) => c.changeDetection)
	const isSearchEnabled = enabled((c) => c.search)

	const assistantStatus = computed(
		() => fetchCapabilities.data.value?.aiAssistant.status ?? null,
	)
	const assistantModel = computed(
		() => fetchCapabilities.data.value?.aiAssistant.model ?? "",
	)
	const isAssistantEnabled = enabled(
		(c) =>
			c.aiAssistant.status === AssistantStatus.Active ||
			c.aiAssistant.status === AssistantStatus.ActiveButWeak,
	)

	return {
		fetchCapabilities,
		isGithubEnabled,
		isSlackEnabled,
		isChangeDetectionEnabled,
		isSearchEnabled,
		isAssistantEnabled,
		assistantStatus,
		assistantModel,
	}
}
