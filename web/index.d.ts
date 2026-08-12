// Replaced at build time by Vite's `define`. In pure builds (DESKTOP_BUILD=1
// or unset/0) this is a literal `true` or `false` so unused branches DCE
// away. In hybrid dev (DESKTOP_BUILD=hybrid, one Nuxt dev server shared
// between the electron renderer and the system browser opened for OAuth)
// it's substituted as a runtime probe of `window.__isElectron` that picks
// per-context.
declare global {
	const __DESKTOP_BUILD__: boolean
}

// https://nuxt.com/docs/4.x/guide/going-further/runtime-config#typing-runtime-config
declare module "nuxt/schema" {
	interface PublicRuntimeConfig {
		sentryDSN: string
		appBaseURL: string
		linkToMoreInfoAboutProduct: string
		authRealtimeAPIBaseHttpURL: string
		authRealtimeAPIBaseWsURL: string
		coreAPIBaseHttpURL: string
		coreAPIBaseWsURL: string
		termsOfServiceURL: string
		privacyPolicyURL: string
		experimentalFeatures: string // example: feature1:org1,org2;feature2:org3
		postgresqlReadOnlyUserSetupGuideURL: string
		mysqlReadOnlyUserSetupGuideURL: string
		mariadbReadOnlyUserSetupGuideURL: string
		prometheusQueryGuideURL: string
		postgresqlQueryGuideURL: string
		mysqlQueryGuideURL: string
		mariadbQueryGuideURL: string
	}
}

declare module "@pinia/colada" {
	interface UseQueryOptions {
		autoRefetch?: MaybeRefOrGetter<boolean>
	}
}

// Renderer-side contract for the Electron preload bridge. `$host` is undefined
// on web; consumers must guard with `?.` or an `isDesktop` check.
declare module "#app" {
	interface NuxtApp {
		$host?: {
			osType: HostOsType
			openExternal(url: string): Promise<void>
			// Auth IPC bridge — every call round-trips to the main process's
			// `authClient` (electron/auth-client.ts) which has the
			// electron-store-backed session. The renderer never sees cookies
			// or tokens.
			auth: {
				getSession(): Promise<unknown>
				signInEmailPassword(args: unknown): Promise<unknown>
				signUpEmailPassword(args: unknown): Promise<unknown>
				requestPasswordReset(args: unknown): Promise<unknown>
				updateUser(args: unknown): Promise<unknown>
				changeEmail(args: unknown): Promise<unknown>
				deleteUser(args: unknown): Promise<unknown>
				getFullOrganization(): Promise<unknown>
				checkOrganizationSlug(args: unknown): Promise<unknown>
				createOrganization(args: unknown): Promise<unknown>
				setActiveOrganization(args: unknown): Promise<unknown>
				acceptOrganizationInvitation(args: unknown): Promise<unknown>
				updateOrganization(args: unknown): Promise<unknown>
				inviteOrganizationMember(args: unknown): Promise<unknown>
				cancelOrganizationInvitation(args: unknown): Promise<unknown>
				removeOrganizationMember(args: unknown): Promise<unknown>
			}
		}
	}
}

// Better Auth's setupRenderer() in the preload exposes these directly on
// window — same shape as @better-auth/electron's ExposedBridges. Only present
// in desktop builds; web consumers shouldn't reach for them.
//
// `__isElectron` is the dedicated runtime signal that we're inside the
// electron renderer; `nuxt.config.ts`'s hybrid-mode `__DESKTOP_BUILD__` define
// probes it.
declare global {
	interface Window {
		__isElectron?: true
		requestAuth: (opts?: { provider?: string }) => Promise<void>
		signOut: () => Promise<void>
		authenticate: (data: { token: string }) => Promise<void>
		getUser: () => Promise<unknown>
		onAuthenticated: (cb: (user: unknown) => unknown) => () => void
		onUserUpdated: (cb: (user: unknown) => unknown) => () => void
		onAuthError: (cb: (ctx: { message: string }) => unknown) => () => void
	}
}

// It is always important to ensure you import/export something when
// augmenting a type
export {}
