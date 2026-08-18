import { showToastMessage } from "~/components/toast"

// Desktop-only: subscribes to Better Auth's renderer bridges exposed by
// electron/preload.ts (setupRenderer). When main signals that a sign-in or
// user-update completed via the oxynote:// deep-link, this kicks the
// composable to refetch — cache-key knowledge stays inside useAuthSession.
// Tree-shaken out of web builds via the __DESKTOP_BUILD__ literal.
export default defineNuxtPlugin(() => {
	if (!__DESKTOP_BUILD__) return

	// NOCOV: desktop-only wiring below; __DESKTOP_BUILD__ is literal false
	// in web and test bundles.
	const { fetchAuthSession, fetchOrganization } = useAuthSession()

	const unsubscribeAuthenticated = window.onAuthenticated(async () => {
		await Promise.all([fetchAuthSession.refetch(), fetchOrganization.refetch()])
		// The renderer was sitting on /login or /signup while the OAuth
		// handoff happened out-of-band; the global middleware only fires on
		// route changes, so trigger a navigation so it re-evaluates against
		// the now-authenticated state.
		await navigateTo("/", { replace: true })
	})

	const unsubscribeUserUpdated = window.onUserUpdated(() => {
		void fetchAuthSession.refetch()
	})

	const unsubscribeAuthError = window.onAuthError((ctx) => {
		// TODO redirect to login?
		showToastMessage("error", ctx.message)
	})

	if (import.meta.hot) {
		import.meta.hot.dispose(() => {
			unsubscribeAuthenticated()
			unsubscribeUserUpdated()
			unsubscribeAuthError()
		})
	}
})
