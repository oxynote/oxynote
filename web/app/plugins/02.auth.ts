import { createAuthClient } from "better-auth/vue"
import { organizationClient } from "better-auth/client/plugins"
import { electronProxyClient } from "@better-auth/electron/proxy"

// Usage example:
// const { $authClient } = useNuxtApp()
export default defineNuxtPlugin({
	setup: () => {
		const config = useRuntimeConfig()
		const headers = import.meta.server ? useRequestHeaders() : undefined
		const authClient = create(
			(import.meta.server &&
				(config.authRealtimeAPIInternalHttpURL as string)) ||
				(config.public.authRealtimeAPIBaseHttpURL as string),
			headers,
		)

		return { provide: { authClient } }
	},
})

function create(baseURL: string, headers?: Record<string, string>) {
	return createAuthClient({
		baseURL: `${baseURL}/api/auth`,
		fetchOptions: {
			headers: headers,
			// In desktop builds, the renderer must never send credentials. The
			// session lives only in main's electron-store and is reachable
			// solely through the IPC bridge; an authenticated request from
			// the renderer would be a confused-deputy bug.
			credentials: __DESKTOP_BUILD__ ? "omit" : "include",
		},
		plugins: [
			organizationClient(),
			// Web-only effect: when the sign-in page is opened by Electron via
			// requestAuth(), this redirects back to oxynote:// once OAuth
			// completes. No-op when no Electron query params are present.
			electronProxyClient({ protocol: { scheme: "oxynote" } }),
		],
	})
}

export type AuthClient = ReturnType<typeof create>
