import { createAuthClient } from "better-auth/client"
import { electronClient } from "@better-auth/electron/client"
import { organizationClient } from "better-auth/client/plugins"
import Store from "electron-store"

// electron-store persists JSON under the app's userData path. The renderer
// has no access to this file or to in-memory cookies — sessions are reachable
// only through the IPC handlers in electron/auth-ipc.ts.
const store = new Store({ name: "oxynote-auth" })

export const authClient = createAuthClient({
	baseURL: `${__API_BASE_URL__}/api/auth`,
	plugins: [
		organizationClient(),
		electronClient({
			// Must match the web app's login page route — this is opened in the
			// system browser to start the sign-in flow.
			signInURL: `${__APP_BASE_URL__}/login`,
			protocol: { scheme: "oxynote" },
			storage: {
				getItem: (key) => store.get(key) ?? null,
				setItem: (key, value) => store.set(key, value),
			},
			storagePrefix: "auth",
			// Must match the auth server's global cookie prefix — the client
			// uses it to recognize which Set-Cookie headers are session
			// cookies and need to be persisted into the local store.
			cookiePrefix: "auth",
		}),
	],
})
