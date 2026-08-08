const AUTH_SESSION_QUERY_KEYS = {
	root: ["auth", "session"] as const,
	organization: ["auth", "organization"] as const,
}

export default function () {
	const nuxtApp = useNuxtApp()
	const { $authClient } = nuxtApp
	const queryCache = useQueryCache()

	// `$host` is typed as optional because web builds don't have it. In
	// desktop builds the preload always exposes it; a missing bridge here is
	// a setup bug, not a recoverable state — fail loudly so we never silently
	// fall through to the renderer's $authClient (which has no credentials).
	function getHost(): NonNullable<typeof nuxtApp.$host> {
		if (!nuxtApp.$host) {
			throw new Error("useAuthSession: $host bridge missing in desktop build")
		}
		return nuxtApp.$host
	}

	// All branches below collapse at build time via Vite's __DESKTOP_BUILD__
	// define. The desktop bundle ends up with only IPC calls; the web bundle
	// with only $authClient calls. This is the security boundary: in the
	// desktop renderer there is no code path that could carry a session
	// cookie — every authenticated op round-trips through main.
	const fetchAuthSession = useQuery({
		key: AUTH_SESSION_QUERY_KEYS.root,
		query: async () => {
			// `as never` keeps the function's inferred return type equal to the
			// direct-call branch — using `Awaited<ReturnType<typeof X>>` here
			// resolves generic defaults differently from a real call expression
			// and widens the type, breaking CFA narrowing at consumer sites.
			// Runtime is unaffected: the IPC handler returns the same `{ data,
			// error }` shape as the direct call.
			if (__DESKTOP_BUILD__) {
				return (await getHost().auth.getSession()) as never
			}
			return await $authClient.getSession()
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})
	const fetchOrganization = useQuery({
		key: AUTH_SESSION_QUERY_KEYS.organization,
		query: async () => {
			// See `as never` rationale on fetchAuthSession above.
			if (__DESKTOP_BUILD__) {
				return (await getHost().auth.getFullOrganization()) as never
			}
			return await $authClient.organization.getFullOrganization()
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	async function updateSessionOnInviteAccept(orgId: string) {
		const args = { organizationId: orgId }
		const res = __DESKTOP_BUILD__
			? ((await getHost().auth.setActiveOrganization(args)) as Awaited<
					ReturnType<typeof $authClient.organization.setActive>
				>)
			: await $authClient.organization.setActive(args)
		if (res.error) {
			return false
		}

		await fetchAuthSession.refetch()
		const orgRes = await fetchOrganization.refetch()

		return orgRes.data?.data?.id === orgId
	}

	async function safeSignOut() {
		let res: Awaited<ReturnType<typeof $authClient.signOut>>
		if (__DESKTOP_BUILD__) {
			// window.signOut clears main's electron-store session; renderer
			// query cache is still cleared below.
			await window.signOut()
			res = { data: null, error: null } as Awaited<
				ReturnType<typeof $authClient.signOut>
			>
		} else {
			res = await $authClient.signOut()
		}
		if (res.error) {
			return res
		}

		queryCache.cancelQueries({ key: AUTH_SESSION_QUERY_KEYS.root })
		queryCache
			.getEntries({ key: AUTH_SESSION_QUERY_KEYS.root })
			.forEach((e) => queryCache.remove(e))
		queryCache.cancelQueries({ key: AUTH_SESSION_QUERY_KEYS.organization })
		queryCache
			.getEntries({ key: AUTH_SESSION_QUERY_KEYS.organization })
			.forEach((e) => queryCache.remove(e))

		return res
	}

	// In desktop, sign-in is kicked off via Better Auth's window.requestAuth
	// (opens the system browser, deep-links back to oxynote:// on success).
	// The composable returns a synthetic success so call sites don't need to
	// branch; the real outcome arrives later via onAuthenticated/onAuthError
	// (wired in app/plugins/electron-auth.client.ts).
	function signInSocial(...args: Parameters<typeof $authClient.signIn.social>) {
		if (__DESKTOP_BUILD__) {
			const params = args[0] as { provider?: string }
			void window.requestAuth({ provider: params.provider })
			return Promise.resolve({ data: null, error: null }) as ReturnType<
				typeof $authClient.signIn.social
			>
		}
		return $authClient.signIn.social(...args)
	}

	// no __DESKTOP_BUILD__ branch: with email verification required, the
	// signup response carries no session material, so the renderer's
	// credential-less client is safe to call directly in both bundles.
	function signUpEmailPassword(
		...args: Parameters<typeof $authClient.signUp.email>
	) {
		return $authClient.signUp.email(...args)
	}

	// Web-only effect: when this page was opened by Electron via requestAuth(),
	// $authClient.ensureElectronRedirect() arms the redirect back to oxynote://
	// once OAuth completes. No-op when the page wasn't opened from Electron.
	// In desktop builds, the sign-in page lives inside the Electron window and
	// triggers the system-browser flow itself, so there's nothing to redirect.
	function setupSignInRedirect() {
		if (__DESKTOP_BUILD__) return undefined
		return $authClient.ensureElectronRedirect()
	}

	function updateUser(...args: Parameters<typeof $authClient.updateUser>) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.updateUser(args[0]) as ReturnType<
				typeof $authClient.updateUser
			>
		}
		return $authClient.updateUser(...args)
	}

	function changeEmail(...args: Parameters<typeof $authClient.changeEmail>) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.changeEmail(args[0]) as ReturnType<
				typeof $authClient.changeEmail
			>
		}
		return $authClient.changeEmail(...args)
	}

	function deleteUser(...args: Parameters<typeof $authClient.deleteUser>) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.deleteUser(args[0]) as ReturnType<
				typeof $authClient.deleteUser
			>
		}
		return $authClient.deleteUser(...args)
	}

	function checkOrganizationSlug(
		...args: Parameters<typeof $authClient.organization.checkSlug>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.checkOrganizationSlug(args[0]) as ReturnType<
				typeof $authClient.organization.checkSlug
			>
		}
		return $authClient.organization.checkSlug(...args)
	}

	function createOrganization(
		...args: Parameters<typeof $authClient.organization.create>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.createOrganization(args[0]) as ReturnType<
				typeof $authClient.organization.create
			>
		}
		return $authClient.organization.create(...args)
	}

	function setActiveOrganization(
		...args: Parameters<typeof $authClient.organization.setActive>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.setActiveOrganization(args[0]) as ReturnType<
				typeof $authClient.organization.setActive
			>
		}
		return $authClient.organization.setActive(...args)
	}

	function acceptOrganizationInvitation(
		...args: Parameters<typeof $authClient.organization.acceptInvitation>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.acceptOrganizationInvitation(args[0]) as ReturnType<
				typeof $authClient.organization.acceptInvitation
			>
		}
		return $authClient.organization.acceptInvitation(...args)
	}

	function updateOrganization(
		...args: Parameters<typeof $authClient.organization.update>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.updateOrganization(args[0]) as ReturnType<
				typeof $authClient.organization.update
			>
		}
		return $authClient.organization.update(...args)
	}

	function inviteOrganizationMember(
		...args: Parameters<typeof $authClient.organization.inviteMember>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.inviteOrganizationMember(args[0]) as ReturnType<
				typeof $authClient.organization.inviteMember
			>
		}
		return $authClient.organization.inviteMember(...args)
	}

	function cancelOrganizationInvitation(
		...args: Parameters<typeof $authClient.organization.cancelInvitation>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.cancelOrganizationInvitation(args[0]) as ReturnType<
				typeof $authClient.organization.cancelInvitation
			>
		}
		return $authClient.organization.cancelInvitation(...args)
	}

	function removeOrganizationMember(
		...args: Parameters<typeof $authClient.organization.removeMember>
	) {
		if (__DESKTOP_BUILD__) {
			return getHost().auth.removeOrganizationMember(args[0]) as ReturnType<
				typeof $authClient.organization.removeMember
			>
		}
		return $authClient.organization.removeMember(...args)
	}

	return {
		fetchAuthSession,
		fetchOrganization,
		updateSessionOnInviteAccept,
		safeSignOut,
		signInSocial,
		signUpEmailPassword,
		setupSignInRedirect,
		updateUser,
		changeEmail,
		deleteUser,
		checkOrganizationSlug,
		createOrganization,
		setActiveOrganization,
		acceptOrganizationInvitation,
		updateOrganization,
		inviteOrganizationMember,
		cancelOrganizationInvitation,
		removeOrganizationMember,
	}
}
