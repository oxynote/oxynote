const ORGANIZATION_QUERY_KEYS = {
	list: ["organizations", "stats"] as const,
}

export default function () {
	const { $apiClient, $nodejsAPIClient } = useNuxtApp()

	const uploadOrganizationLogo = useMutation({
		// no optimistic updates possible here
		mutation: async (file: File) => {
			const body = new FormData()
			body.append("logo", file)

			const response = await $apiClient.raw(`/api/organizations/logo`, {
				method: "PUT",
				body,
			})
			const location = response.headers.get("location")

			if (!location) {
				throw new Error("missing location header")
			}

			return location
		},
	})

	const fetchOrganizationStats = useQuery({
		key: ORGANIZATION_QUERY_KEYS.list,
		query: async () => {
			return await $nodejsAPIClient<OrganizationStats>(
				`/api/organizations/stats`,
				{
					method: "GET",
				},
			)
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 5 * 60 * 1000, // 5 mins
		autoRefetch: true,
	})

	return { uploadOrganizationLogo, fetchOrganizationStats }
}
