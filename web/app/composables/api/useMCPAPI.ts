const MCP_QUERY_KEYS = {
	consents: ["mcp", "consents"] as const,
}

export default function () {
	const { $authRealtimeAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	// the OAuth clients the user has authorized; revoking one deletes
	// this consent, which cuts off the client's tokens immediately.
	const fetchConsents = useQuery({
		key: MCP_QUERY_KEYS.consents,
		query: async () => {
			return await $authRealtimeAPIClient<OAuthConsent[]>(
				`/api/auth/oauth2/get-consents`,
				{ method: "GET" },
			)
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 60 * 1000, // 1 min
	})

	async function fetchPublicClient(clientId: string) {
		return await $authRealtimeAPIClient<OAuthPublicClient>(
			`/api/auth/oauth2/public-client`,
			{ method: "GET", query: { client_id: clientId } },
		)
	}

	const revokeConsent = useMutation({
		mutation: async (id: string) => {
			// the endpoint answers with an empty body; the refetch below is
			// what the caller actually waits on.
			await $authRealtimeAPIClient(`/api/auth/oauth2/delete-consent`, {
				method: "POST",
				body: { id },
			})
		},
		onSettled: () =>
			queryCache.invalidateQueries({ key: MCP_QUERY_KEYS.consents }),
	})

	// answers a pending authorization request from the consent page.
	// oauthQuery is the consent page's own (signed) query string, which
	// tells the server which request is being decided.
	async function decideConsent(accept: boolean, oauthQuery: string) {
		return await $authRealtimeAPIClient<OAuthConsentDecision>(
			`/api/auth/oauth2/consent`,
			{
				method: "POST",
				body: { accept, oauth_query: oauthQuery },
			},
		)
	}

	return { fetchConsents, fetchPublicClient, revokeConsent, decideConsent }
}
