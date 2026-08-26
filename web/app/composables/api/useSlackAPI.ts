import { FetchError } from "ofetch"
import isDeepEqual from "fast-deep-equal"

const SLACK_QUERY_KEYS = {
	connected: ["slack", "connected"] as const,
	userLinkSettings: ["slack", "user-link-settings"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()
	const { isSlackEnabled } = useCapabilitiesAPI()

	// whether the server has the Slack App integration configured at all.
	const slackConfigured = isSlackEnabled

	const fetchSlackConnectionStatus = useQuery<SlackConnectionStatus>({
		key: SLACK_QUERY_KEYS.connected,
		query: async () => {
			return await $coreAPIClient<SlackConnectionStatus>(`/api/slack`, {
				method: "GET",
			})
		},
		enabled: () => isSlackEnabled.value,
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	async function fetchSlackInstallURL(): Promise<SlackInstallResponse> {
		// we don't want to use useQuery here as installs are
		// typically one-off and we don't want to cache them
		return await $coreAPIClient<SlackInstallResponse>(`/api/slack/install`, {
			method: "GET",
		})
	}

	const connectSlack = useMutation({
		// no optimistic updates possible here
		mutation: async (queryParams: URLSearchParams) => {
			const query = queryParams.toString()
			const url = query ? `/api/slack/connect?${query}` : "/api/slack/connect"

			return await $coreAPIClient<unknown>(url, { method: "GET" })
		},
	})

	const linkSlackUser = useMutation({
		// no optimistic updates possible here
		mutation: async (queryParams: URLSearchParams) => {
			const query = queryParams.toString()
			const url = query
				? `/api/slack/users/link?${query}`
				: "/api/slack/users/link"

			return await $coreAPIClient<{ linked: boolean }>(url, { method: "GET" })
		},
	})

	const disconnectSlack = useMutation({
		onMutate: () => {
			const oldStatus = clone(
				queryCache.getQueryData<SlackConnectionStatus>(
					SLACK_QUERY_KEYS.connected,
				),
			)
			const newStatus: SlackConnectionStatus = {
				connected: false,
				configured: oldStatus?.configured ?? true,
			}

			queryCache.setQueryData(SLACK_QUERY_KEYS.connected, newStatus)
			queryCache.cancelQueries({
				key: SLACK_QUERY_KEYS.connected,
			})

			return { newStatus, oldStatus }
		},
		mutation: async () => {
			return await $coreAPIClient<unknown>(`/api/slack`, { method: "DELETE" })
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
			if (!(await fetchSlackConnectionStatus.refresh()).data?.configured) {
				return null
			}

			try {
				return await $coreAPIClient<SlackUserLinkSettings>(`/api/slack/users`, {
					method: "GET",
				})
			} catch (error) {
				if (error instanceof FetchError && error.statusCode === 404) {
					return null
				}

				throw error
			}
		},
		enabled: () => isSlackEnabled.value,
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 6 * 60 * 1000, // 6mins
		autoRefetch: true,
	})

	const updateSlackUserLinkSettings = useMutation({
		onMutate: (req) => {
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
			return await $coreAPIClient<SlackUserLinkSettings>(
				`/api/slack/users/settings`,
				{
					method: "PUT",
					body: req,
				},
			)
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
		onMutate: () => {
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
			return await $coreAPIClient<unknown>(`/api/slack/users`, {
				method: "DELETE",
			})
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
		slackConfigured,
		fetchSlackInstallURL,
		connectSlack,
		linkSlackUser,
		disconnectSlack,
		fetchSlackUserLinkSettings,
		updateSlackUserLinkSettings,
		unlinkSlackUser,
	}
}
