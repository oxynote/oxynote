import { showToastMessage } from "~/components/toast"
import { AUTH_SESSION_QUERY_KEYS } from "~/composables/useAuthSession"
import { ROLE_REFUSAL_CODE } from "~/utils/auth"

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

	const coreAPI = $fetch.create({
		baseURL:
			(import.meta.server && config.coreAPIInternalHttpURL) ||
			config.public.coreAPIBaseHttpURL,
		credentials: "include",
		onRequest({ options }) {
			if (!ssrHeaders) {
				return
			}

			// NOCOV: ssrHeaders is only captured during SSR; tests run the
			// client bundle, so the merge below is unreachable there.
			const h = new Headers(options.headers)

			for (const [k, v] of Object.entries(ssrHeaders)) {
				if (v && !h.has(k)) {
					h.set(k, v)
				}
			}

			options.headers = h
		},
		onResponseError: async ({ response }: { response: Response }) => {
			if (response.status === 401) {
				const current = route.fullPath || "/"
				await redirectToLogin(current, false, nuxtApp)
			}

			notifyRoleNotAllowed(response, nuxtApp)
		},
	})

	const authRealtimeAPI = $fetch.create({
		baseURL:
			(import.meta.server && config.authRealtimeAPIInternalHttpURL) ||
			config.public.authRealtimeAPIBaseHttpURL,
		credentials: "include",
		onRequest({ options }) {
			if (!ssrHeaders) {
				return
			}

			// NOCOV: ssrHeaders is only captured during SSR; tests run the
			// client bundle, so the merge below is unreachable there.
			const h = new Headers(options.headers)

			for (const [k, v] of Object.entries(ssrHeaders)) {
				if (v && !h.has(k)) {
					h.set(k, v)
				}
			}

			options.headers = h
		},
		onResponseError: async ({ response }: { response: Response }) => {
			if (response.status === 401) {
				const current = route.fullPath || "/"
				await redirectToLogin(current, false, nuxtApp)
			}

			notifyRoleNotAllowed(response, nuxtApp)
		},
	})

	return {
		provide: {
			coreAPIClient: coreAPI,
			authRealtimeAPIClient: authRealtimeAPI,
		},
	}
})

// invalidating a query cancels its fetch in flight and starts a new one.
// So without this, every refusal in a burst would send its own request.
let organizationReread: Promise<unknown> | null = null

// a role refusal reaches the user as one toast, whichever call hit it.
// The role may have changed since the member list was read, so the list is
// read again and the hidden actions follow it. Refusals that arrive while
// that read runs share it.
function notifyRoleNotAllowed(
	response: Response & { _data?: unknown },
	nuxtApp: {
		$i18n: { t: (key: string) => string }
		runWithContext: <T>(fn: () => Promise<T>) => Promise<T>
	},
) {
	if (
		!import.meta.client ||
		response.status !== 403 ||
		(response._data as { code?: unknown } | undefined)?.code !==
			ROLE_REFUSAL_CODE
	) {
		return
	}

	showToastMessage(
		"error",
		nuxtApp.$i18n.t("general.errors.role-not-allowed"),
		nuxtApp.$i18n.t("general.errors.role-not-allowed-description"),
	)

	organizationReread ??= nuxtApp
		.runWithContext(() =>
			useQueryCache().invalidateQueries({
				key: AUTH_SESSION_QUERY_KEYS.organization,
			}),
		)
		// a failed read keeps its error in the query state and the old list
		// stays, so nothing else needs the rejection
		.catch(() => undefined)
		.finally(() => {
			organizationReread = null
		})
}

export function redirectToLogin(
	currentPath: string | undefined,
	replace: boolean,
	nuxtApp: {
		runWithContext: <T>(fn: () => Promise<T>) => Promise<T>
	},
) {
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

export async function redirectToDocId(
	docId: string,
	fetchOrganization: ReturnType<typeof useAuthSession>["fetchOrganization"],
) {
	const orgData = await fetchOrganization.refresh()
	const orgRealName = orgData.data?.data?.slug || ""

	// without an organization slug the document path would start with a
	// double slash, which the router treats as a protocol-relative url —
	// fall back to the root and let the redirect middleware resolve it
	if (!orgRealName) {
		return navigateTo("/", { replace: true })
	}

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
	// during SSR the nuxt context is a module global that a concurrent
	// request clears. It is only reliable before the first await here, so
	// take the app now and navigate through it.
	const nuxtApp = useNuxtApp()
	const redirect = (path: string) =>
		nuxtApp.runWithContext(() => navigateTo(path, { replace: true }))

	if (orgRealName === null) {
		const orgData = await fetchOrganization.refresh()
		orgRealName = orgData.data?.data?.slug || ""
	}

	// without an organization slug the document path would start with a
	// double slash, which the router treats as a protocol-relative url —
	// fall back to the root and let the redirect middleware resolve it
	if (!orgRealName) {
		return redirect("/")
	}

	const docTree = await fetchDocumentTree.refresh()
	if (!docTree.data?.length) {
		return redirect(`/${createNameSlug(orgRealName)}`)
	}

	const firstDoc = docTree.data.find((v) => v.id !== pendingDeletionId)
	if (!firstDoc) {
		return redirect(`/${createNameSlug(orgRealName)}`)
	}

	return redirect(
		`/${createNameSlug(orgRealName)}/${createNameSlugWithId(
			firstDoc.documentName,
			firstDoc.id,
		)}`,
	)
}
