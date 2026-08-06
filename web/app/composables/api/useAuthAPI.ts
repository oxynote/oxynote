const AUTH_QUERY_KEYS = {
	methods: ["auth", "methods"] as const,
}

export default function () {
	const { $authRealtimeAPIClient } = useNuxtApp()

	const fetchAuthMethods = useQuery({
		key: AUTH_QUERY_KEYS.methods,
		query: async () => {
			return await $authRealtimeAPIClient<AuthMethods>(`/api/auth-methods`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 5 * 60 * 1000, // 5 mins
	})

	return { fetchAuthMethods }
}
