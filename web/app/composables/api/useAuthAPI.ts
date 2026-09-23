export const AUTH_QUERY_KEYS = {
	config: ["auth", "config"] as const,
}

export default function () {
	const { $authRealtimeAPIClient } = useNuxtApp()

	const fetchAuthConfig = useQuery({
		key: AUTH_QUERY_KEYS.config,
		query: async () => {
			return await $authRealtimeAPIClient<AuthConfig>(`/api/auth-config`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 5 * 60 * 1000, // 5 mins
	})

	// until the config arrives email is assumed to be delivered, so a page
	// renders the flows every configured server supports.
	const isEmailEnabled = computed(
		() => fetchAuthConfig.state.value.data?.emailEnabled ?? true,
	)

	// until the config arrives the instance is assumed to allow signup and
	// any number of members; the server enforces its own limits either way.
	const isSingleOrganization = computed(
		() => fetchAuthConfig.state.value.data?.singleOrganization ?? false,
	)
	const maxOrganizationMembers = computed(
		() => fetchAuthConfig.state.value.data?.maxOrganizationMembers ?? null,
	)

	const defaultAdmin = computed(() =>
		isSingleOrganization.value
			? (fetchAuthConfig.state.value.data?.defaultAdmin ?? null)
			: null,
	)

	return {
		defaultAdmin,
		fetchAuthConfig,
		isEmailEnabled,
		isSingleOrganization,
		maxOrganizationMembers,
	}
}
