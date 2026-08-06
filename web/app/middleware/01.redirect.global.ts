import { redirectToLogin } from "~/plugins/03.api-fetch"

export default defineNuxtRouteMiddleware(async (to) => {
	// The desktop-auth handoff page is only ever reached as the OAuth
	// callback target; let it mount unconditionally so it can either fire
	// the deep link or fall back to /login itself.
	if (to.name === "desktop-auth") {
		return
	}

	const nuxtApp = useNuxtApp()
	const { fetchAuthSession, fetchOrganization } = useAuthSession()

	const sessionReq = await fetchAuthSession.refresh()
	const session = sessionReq?.data?.data?.session

	if (to.name === "accept-invite") {
		// only 1 organization can be joined per user
		if (session && session.activeOrganizationId) {
			return nuxtApp.runWithContext(() => navigateTo("/", { replace: true }))
		}

		return // do nothing, allow to proceed
	}

	if (session && !session.activeOrganizationId && to.name !== "onboarding") {
		return nuxtApp.runWithContext(() =>
			navigateTo({ name: "onboarding" }, { replace: true }),
		)
	} else if (
		session &&
		session.activeOrganizationId &&
		to.name === "onboarding"
	) {
		return nuxtApp.runWithContext(() => navigateTo("/", { replace: true }))
	}

	if (!session && !to.meta.skipAuth) {
		// "/" is the post-login default anyway, so a `next` param pointing at
		// it is noise in the login URL
		return redirectToLogin(
			to.fullPath === "/" ? undefined : to.fullPath,
			true,
			nuxtApp,
		)
	} else if (session && to.meta.skipAuth) {
		const nextUrl = to.query.next as string | undefined
		if (nextUrl) {
			return nuxtApp.runWithContext(() => {
				return navigateTo(decodeURIComponent(nextUrl), { replace: true })
			})
		}

		return nuxtApp.runWithContext(() => navigateTo("/", { replace: true }))
	}

	if (session && to.path === "/") {
		const orgName = (await fetchOrganization.refresh()).data?.data?.slug
		if (!orgName) {
			return redirectToLogin(to.fullPath, true, nuxtApp)
		}

		// created here, not at the top of the middleware: a query
		// instantiated outside a component fetches immediately, and an
		// unauthenticated tree fetch 401s, which the API client turns into a
		// redirect to /login — looping forever on the login page itself.
		const { fetchDocumentTree } = useDocumentAPI()
		const docTree = await fetchDocumentTree.refresh()
		if (!docTree.data?.length) {
			return nuxtApp.runWithContext(() => {
				return navigateTo(`/${createNameSlug(orgName)}`, {
					replace: true,
				})
			})
		}

		const firstDoc = docTree.data[0]!

		return nuxtApp.runWithContext(() => {
			return navigateTo(
				`/${createNameSlug(orgName)}/${createNameSlugWithId(firstDoc.documentName, firstDoc.id)}`,
				{
					replace: true,
				},
			)
		})
	} else if (!session && to.path === "/") {
		return redirectToLogin(undefined, true, nuxtApp)
	}
})
