import { FetchError } from "ofetch"
import isDeepEqual from "fast-deep-equal"

const SLACK_QUERY_KEYS = {
	connected: ["slack", "connected"] as const,
	userLinkSettings: ["slack", "user-link-settings"] as const,
}

export default function () {
	const { $apiClient } = useNuxtApp()
	const queryCache = useQueryCache()

	const fetchSlackConnectionStatus = useQuery<SlackConnectionStatus>({
		key: SLACK_QUERY_KEYS.connected,
		query: async () => {
			return await $apiClient<SlackConnectionStatus>(`/api/slack`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	async function fetchSlackInstallURL(): Promise<SlackInstallResponse> {
		// we don't want to use useQuery here as installs are
		// typically one-off and we don't want to cache them
		return await $apiClient<SlackInstallResponse>(`/api/slack/install`, {
			method: "GET",
		})
	}

	const connectSlack = useMutation({
		// no optimistic updates possible here
		mutation: async (queryParams: URLSearchParams) => {
			const query = queryParams.toString()
			const url = query ? `/api/slack/connect?${query}` : "/api/slack/connect"

			return await $apiClient(url, { method: "GET" })
		},
	})

	const linkSlackUser = useMutation({
		// no optimistic updates possible here
		mutation: async (queryParams: URLSearchParams) => {
			const query = queryParams.toString()
			const url = query
				? `/api/slack/users/link?${query}`
				: "/api/slack/users/link"

			return await $apiClient(url, { method: "GET" })
		},
	})

	const disconnectSlack = useMutation({
		onMutate: async () => {
			const oldStatus = clone(
				queryCache.getQueryData<SlackConnectionStatus>(
					SLACK_QUERY_KEYS.connected,
				),
			)
			const newStatus: SlackConnectionStatus = { connected: false }

			queryCache.setQueryData(SLACK_QUERY_KEYS.connected, newStatus)
			queryCache.cancelQueries({
				key: SLACK_QUERY_KEYS.connected,
			})

			return { newStatus, oldStatus }
		},
		mutation: async () => {
			return await $apiClient(`/api/slack`, { method: "DELETE" })
		},
		async onSuccess() {
			await queryCache.invalidateQueries({ key: SLACK_QUERY_KEYS.connected })
		},
		onError(_err, _data, { oldStatus, newStatus }) {
			const cachedStatus = queryCache.getQueryData<SlackConnectionStatus>(
				SLACK_QUERY_KEYS.connected,
			)
			if (!isDeepEqual(newStatus, cachedStatus)) {
				return
			}

			// rollback
			queryCache.setQueryData(SLACK_QUERY_KEYS.connected, oldStatus)
		},
	})

	const fetchSlackUserLinkSettings = useQuery<SlackUserLinkSettings | null>({
		key: SLACK_QUERY_KEYS.userLinkSettings,
		query: async () => {
			try {
				return await $apiClient<SlackUserLinkSettings>(`/api/slack/users`, {
					method: "GET",
				})
			} catch (error) {
				if (error instanceof FetchError && error.statusCode === 404) {
					return null
				}

				throw error
			}
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 6 * 60 * 1000, // 6mins
		autoRefetch: true,
	})

	const updateSlackUserLinkSettings = useMutation({
		onMutate: async (req) => {
			const oldSettings = clone(
				queryCache.getQueryData<SlackUserLinkSettings | null>(
					SLACK_QUERY_KEYS.userLinkSettings,
				),
			)
			const newSettings: SlackUserLinkSettings = clone(req)

			queryCache.setQueryData(SLACK_QUERY_KEYS.userLinkSettings, newSettings)
			queryCache.cancelQueries({
				key: SLACK_QUERY_KEYS.userLinkSettings,
			})

			return { newSettings, oldSettings }
		},
		mutation: async (req: SlackUserLinkSettings) => {
			return await $apiClient(`/api/slack/users/settings`, {
				method: "PUT",
				body: req,
			})
		},
		async onSuccess() {
			await queryCache.invalidateQueries({
				key: SLACK_QUERY_KEYS.userLinkSettings,
			})
		},
		onError(_err, _data, { oldSettings, newSettings }) {
			const cachedSettings =
				queryCache.getQueryData<SlackUserLinkSettings | null>(
					SLACK_QUERY_KEYS.userLinkSettings,
				)
			if (!isDeepEqual(newSettings, cachedSettings)) {
				return
			}

			// rollback
			queryCache.setQueryData(SLACK_QUERY_KEYS.userLinkSettings, oldSettings)
		},
	})

	const unlinkSlackUser = useMutation({
		onMutate: async () => {
			const oldSettings = clone(
				queryCache.getQueryData<SlackUserLinkSettings | null>(
					SLACK_QUERY_KEYS.userLinkSettings,
				),
			)

			queryCache.setQueryData(SLACK_QUERY_KEYS.userLinkSettings, null)
			queryCache.cancelQueries({
				key: SLACK_QUERY_KEYS.userLinkSettings,
			})

			return { oldSettings }
		},
		mutation: async () => {
			return await $apiClient(`/api/slack/users`, { method: "DELETE" })
		},
		async onSuccess() {
			await queryCache.invalidateQueries({
				key: SLACK_QUERY_KEYS.userLinkSettings,
			})
		},
		onError(_err, _data, { oldSettings }) {
			const cachedSettings =
				queryCache.getQueryData<SlackUserLinkSettings | null>(
					SLACK_QUERY_KEYS.userLinkSettings,
				)
			if (!isDeepEqual(null, cachedSettings)) {
				return
			}

			// rollback
			queryCache.setQueryData(SLACK_QUERY_KEYS.userLinkSettings, oldSettings)
		},
	})

	return {
		fetchSlackConnectionStatus,
		fetchSlackInstallURL,
		connectSlack,
		linkSlackUser,
		disconnectSlack,
		fetchSlackUserLinkSettings,
		updateSlackUserLinkSettings,
		unlinkSlackUser,
	}
}
