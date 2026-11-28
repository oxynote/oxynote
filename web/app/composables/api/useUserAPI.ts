import isDeepEqual from "fast-deep-equal"

const USER_QUERY_KEYS = {
	spotlights: ["users", "spotlights"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	function useFetchUserSpotlights() {
		return useQuery({
			key: USER_QUERY_KEYS.spotlights,
			query: async () => {
				return await $coreAPIClient<UserSpotlights>(`/api/users/spotlights`, {
					method: "GET",
				})
			},
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 60 * 1000, // 3 hours
			autoRefetch: true,
		})
	}

	const updateUserSpotlights = useMutation({
		onMutate(req: UserSpotlightsUpdateRequest) {
			const oldSpotlights = queryCache.getQueryData<UserSpotlights>(
				USER_QUERY_KEYS.spotlights,
			)

			const newSpotlights = { ...oldSpotlights, ...req }

			queryCache.setQueryData(USER_QUERY_KEYS.spotlights, newSpotlights)
			queryCache.cancelQueries({ key: USER_QUERY_KEYS.spotlights })

			return { oldSpotlights, newSpotlights }
		},
		mutation: async (req: UserSpotlightsUpdateRequest) => {
			await $coreAPIClient(`/api/users/spotlights`, {
				method: "PUT",
				body: req,
			})
		},
		async onSuccess() {
			await queryCache.invalidateQueries({ key: USER_QUERY_KEYS.spotlights })
		},
		onError(_err, _req, { oldSpotlights, newSpotlights }) {
			const cachedSpotlights = queryCache.getQueryData(
				USER_QUERY_KEYS.spotlights,
			)
			if (!isDeepEqual(newSpotlights, cachedSpotlights)) {
				return
			}

			// rollback
			queryCache.setQueryData(USER_QUERY_KEYS.spotlights, oldSpotlights)
		},
	})

	const uploadUserImage = useMutation({
		// no optimistic updates possible here
		mutation: async (file: File) => {
			const body = new FormData()
			body.append("image", file)

			const response = await $coreAPIClient.raw(`/api/users/image`, {
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

	return { useFetchUserSpotlights, updateUserSpotlights, uploadUserImage }
}
