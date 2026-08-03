export default defineNuxtPlugin((nuxtApp) => {
	const config = useRuntimeConfig()
	const route = useRoute()

	// capture request headers eagerly during plugin setup. calling
	// useRequestHeaders() lazily inside onRequest callbacks fails on
	// Cloudflare's workerd runtime because the H3 request context is lost
	// by the time the callback fires.
	const ssrHeaders = import.meta.server
		? useRequestHeaders(["cookie", "accept-language"])
		: undefined

	const goAPI = $fetch.create({
		baseURL: config.public.goAPIBaseHttpURL as string,
		credentials: "include",
		onRequest({ options }) {
			if (!ssrHeaders) {
				return
			}

			const h = new Headers(options.headers as HeadersInit | undefined)

			for (const [k, v] of Object.entries(ssrHeaders)) {
				if (v && !h.has(k)) {
					h.set(k, v)
				}
			}

			options.headers = h
		},
		onResponseError: async ({ response }: { response: Response }) => {
			if (response?.status === 401) {
				const current = route.fullPath || "/"
				await redirectToLogin(current, false, nuxtApp)
			}
		},
	})

	const nodejsAPI = $fetch.create({
		baseURL: config.public.nodejsAPIBaseHttpURL as string,
		credentials: "include",
		onRequest({ options }) {
			if (!ssrHeaders) {
				return
			}

			const h = new Headers(options.headers as HeadersInit | undefined)

			for (const [k, v] of Object.entries(ssrHeaders)) {
				if (v && !h.has(k)) {
					h.set(k, v)
				}
			}

			options.headers = h
		},
		onResponseError: async ({ response }: { response: Response }) => {
			if (response?.status === 401) {
				const current = route.fullPath || "/"
				await redirectToLogin(current, false, nuxtApp)
			}
		},
	})

	return {
		provide: {
			apiClient: goAPI,
			nodejsAPIClient: nodejsAPI,
		},
	}
})

export function redirectToLogin(
	currentPath: string | undefined,
	replace: boolean,
	nuxtApp: {
		runWithContext: <T>(fn: () => Promise<T>) => Promise<T>
	},
) {
	if (nuxtApp) {
		return nuxtApp.runWithContext(async () => {
			return navigateTo(
				{
					name: "login",
					query: {
						next: currentPath ? encodeURIComponent(currentPath) : undefined,
					},
				},
				{ replace },
			)
		})
	}

	return navigateTo(
		{
			name: "login",
			query: {
				next: currentPath ? encodeURIComponent(currentPath) : undefined,
			},
		},
		{ replace },
	)
}

export async function redirectToDocId(
	docId: string,
	fetchOrganization: ReturnType<typeof useAuthSession>["fetchOrganization"],
) {
	const orgData = await fetchOrganization.refresh()
	const orgRealName = orgData.data?.data?.slug || ""

	return navigateTo(`/${createNameSlug(orgRealName)}/${docId}`, {
		replace: true,
	})
}

export async function redirectToOrgRoot(
	fetchOrganization: ReturnType<typeof useAuthSession>["fetchOrganization"],
) {
	const orgData = await fetchOrganization.refresh()
	const orgRealName = orgData.data?.data?.slug || ""

	return navigateTo(`/${createNameSlug(orgRealName)}`, {
		replace: true,
	})
}

export async function redirectToFirst(
	orgRealName: string | null,
	fetchOrganization: ReturnType<typeof useAuthSession>["fetchOrganization"],
	fetchDocumentTree: ReturnType<typeof useDocumentAPI>["fetchDocumentTree"],
	pendingDeletionId: string | null,
) {
	if (orgRealName === null) {
		const orgData = await fetchOrganization.refresh()
		orgRealName = orgData.data?.data?.slug || ""
	}

	const docTree = await fetchDocumentTree.refresh()
	if (!docTree.data?.length) {
		return navigateTo(`/${createNameSlug(orgRealName)}`, {
			replace: true,
		})
	}

	const firstDoc = docTree.data.find((v) => v.id !== pendingDeletionId)
	if (!firstDoc) {
		return navigateTo(`/${createNameSlug(orgRealName)}`, {
			replace: true,
		})
	}

	return navigateTo(
		`/${createNameSlug(orgRealName)}/${createNameSlugWithId(
			firstDoc.documentName,
			firstDoc.id,
		)}`,
		{
			replace: true,
		},
	)
}
