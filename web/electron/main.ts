import {
	app,
	BrowserWindow,
	ipcMain,
	net,
	protocol,
	session,
	shell,
} from "electron"
import path from "node:path"
import { existsSync } from "node:fs"
import { pathToFileURL } from "node:url"
import squirrelStartup from "electron-squirrel-startup"
import { authClient } from "./auth-client"
import { registerAuthIpcHandlers } from "./auth-ipc"

if (squirrelStartup) {
	app.quit()
}

// Vite/Nuxt HMR requires `unsafe-eval` and `unsafe-inline` in the dev CSP,
// which means Electron's renderer security check always trips in dev. The
// warning never appears in packaged builds, so silencing it in dev only
// keeps real prod-build warnings visible.
if (!app.isPackaged) {
	process.env.ELECTRON_DISABLE_SECURITY_WARNINGS = "true"
}

// Must run before `app.whenReady`. Registers the oxynote:// deep-link handler
// that exchanges the OAuth code for a session and writes it to electron-store.
// `csp: false` disables the plugin's built-in CSP injection so we can write
// our own below — the plugin's version only allows the HTTP origin and blocks
// the websocket connections to the API.
authClient.setupMain({ csp: false, scheme: true, bridges: true })

app.setName("Oxynote")
app.setAboutPanelOptions({
	applicationName: "Oxynote",
	applicationVersion: app.getVersion(),
	copyright: "© Oxynote",
})

// __dirname at runtime is `.vite/build/` (inside the asar when packaged).
// `.output/public/` is bundled alongside via the forge `ignore` allowlist.
const RENDERER_DIST = path.join(__dirname, "..", "..", ".output", "public")

// Same-origin checks for navigation interception. Anything that isn't the
// app's own origin gets handed to the OS via shell.openExternal.
const isInternalUrl = (url: string): boolean => {
	if (url === "about:blank") return true
	try {
		const u = new URL(url)
		if (u.protocol === "oxynote:" && u.host === "app") return true
		if (u.protocol === "http:" && u.host === "localhost:3000") return true
		return false
	} catch {
		return false
	}
}

protocol.registerSchemesAsPrivileged([
	{
		scheme: "oxynote",
		privileges: {
			standard: true,
			secure: true,
			supportFetchAPI: true,
			stream: true,
		},
	},
])

const createWindow = () => {
	const win = new BrowserWindow({
		width: 1400,
		height: 900,
		minWidth: 1024,
		minHeight: 640,
		webPreferences: {
			preload: path.join(__dirname, "preload.cjs"),
			contextIsolation: true,
			nodeIntegration: false,
			sandbox: true,
		},
	})

	// Route every `target="_blank"` / `window.open` to the OS instead of
	// spawning an in-app browser window. Internal target=_blank links are just
	// denied — the app shouldn't be opening new in-app windows.
	win.webContents.setWindowOpenHandler(({ url }) => {
		if (!isInternalUrl(url)) {
			void shell.openExternal(url)
		}

		return { action: "deny" }
	})

	// Top-level navigations to anything outside the app's origin (e.g. raw
	// <a href="https://…"> clicks) go to the system browser too.
	win.webContents.on("will-navigate", (event, url) => {
		if (!isInternalUrl(url)) {
			event.preventDefault()
			void shell.openExternal(url)
		}
	})

	if (app.isPackaged) {
		void win.loadURL("oxynote://app/index.html")
	} else {
		void win.loadURL("http://localhost:3000")
		win.webContents.openDevTools({ mode: "detach" })
	}
}

void app.whenReady().then(() => {
	ipcMain.handle("shell:openExternal", (_event, url: string) =>
		shell.openExternal(url),
	)
	registerAuthIpcHandlers()

	// Attach the better-auth session cookie to renderer-originated requests
	// to the API. The cookie value lives in main's electron-store (encrypted
	// via safeStorage); Chromium's network stack runs in main, so injection
	// happens before the request crosses into the renderer's world — the
	// renderer never holds the cookie in its JS heap. /api/auth/* is carved
	// out so renderer code can't accidentally use the session against the
	// auth endpoints (those must keep going through main's IPC bridge).
	const apiOrigin = new URL(__API_BASE_URL__).origin
	const apiWsOrigin = apiOrigin.replace(/^http/, "ws")

	// CSP for the renderer. The plugin's default (disabled in setupMain
	// above) only covers the HTTP API origin, which blocks the API
	// websockets, the iconify icon CDN, and remote avatars/images. Allow:
	//   - API HTTP + WS for our own backend
	//   - https: for external services (iconify, OAuth provider avatars,
	//     image hosts, etc.)
	//   - user-image: scheme for @better-auth/electron's image proxy
	//   - data: for inline-SVG icons rendered by the editor
	// `script-src 'self'` is required even though no foreign scripts are
	// loaded — without it, scripts default to unrestricted and eval() works,
	// which Electron probes for and warns about. In dev we relax it because
	// Vite/Nuxt HMR needs eval and inline scripts.
	// The trailing `'none'` directives are zero-cost hardening: nothing in
	// the app uses <base>, <object>/<embed>, framing, or forms.
	const scriptSrc = app.isPackaged
		? `script-src 'self'`
		: `script-src 'self' 'unsafe-eval' 'unsafe-inline'`
	const styleSrc = `style-src 'self' 'unsafe-inline'`
	session.defaultSession.webRequest.onHeadersReceived((details, callback) => {
		callback({
			responseHeaders: {
				...details.responseHeaders,
				"Content-Security-Policy": [
					`default-src 'self'; ${scriptSrc}; ${styleSrc}; connect-src 'self' ${apiOrigin} ${apiWsOrigin} https:; img-src 'self' ${apiOrigin} user-image: data: https:; font-src 'self' data:; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'`,
				],
			},
		})
	})

	session.defaultSession.webRequest.onBeforeSendHeaders(
		// `ws://` is a distinct URL scheme from `http://` for filter-matching
		// purposes, so we list both — otherwise WS upgrade requests bypass
		// the cookie injection and the Go API rejects the connection.
		{ urls: [`${apiOrigin}/*`, `${apiWsOrigin}/*`] },
		(details, callback) => {
			const url = new URL(details.url)
			if (url.pathname.startsWith("/api/auth/")) {
				callback({ requestHeaders: details.requestHeaders })
				return
			}

			const authCookieHeader = authClient.getCookie().replace(/^;\s*/, "") // strip the leading "; " getCookie always emits
			if (!authCookieHeader) {
				callback({ requestHeaders: details.requestHeaders })
				return
			}

			// Merge: drop any `auth.*` entries the renderer's own cookie jar
			// might have accumulated (stale from previous web sessions, etc.)
			// and append the fresh ones from electron-store. Non-auth cookies
			// like `i18n_redirected` are preserved.
			const cookieKey = Object.keys(details.requestHeaders).find(
				(k) => k.toLowerCase() === "cookie",
			)
			const existing = cookieKey ? details.requestHeaders[cookieKey] : ""
			const cleaned = existing
				.split(";")
				.map((s) => s.trim())
				.filter((s) => s && !s.startsWith("auth."))
				.join("; ")
			const merged = cleaned
				? `${cleaned}; ${authCookieHeader}`
				: authCookieHeader

			if (cookieKey) {
				delete details.requestHeaders[cookieKey]
			}

			details.requestHeaders.Cookie = merged

			callback({ requestHeaders: details.requestHeaders })
		},
	)

	if (app.isPackaged) {
		// Serve the bundled SPA from `.output/public/`. Falls back to index.html
		// for any path that doesn't resolve to a file — required so deep-link
		// reloads (e.g. oxynote://app/notes/123) work under Nuxt history mode.
		protocol.handle("oxynote", async (request) => {
			const url = new URL(request.url)
			const requestedPath = url.pathname === "/" ? "/index.html" : url.pathname
			const candidate = path.join(RENDERER_DIST, requestedPath)
			const target = existsSync(candidate)
				? candidate
				: path.join(RENDERER_DIST, "index.html")

			return net.fetch(pathToFileURL(target).toString())
		})
	}

	createWindow()

	app.on("activate", () => {
		if (BrowserWindow.getAllWindows().length === 0) {
			createWindow()
		}
	})
})

app.on("window-all-closed", () => {
	if (process.platform !== "darwin") app.quit()
})
