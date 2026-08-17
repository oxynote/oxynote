import path from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"
import { beforeEach, describe, expect, it, vi } from "vitest"

const state = vi.hoisted(() => ({
	isPackaged: false,
	squirrelStartup: false,
	resolveWhenReady: null as ((value?: unknown) => void) | null,
}))

const mocks = vi.hoisted(() => {
	const responseMarker = { marker: "response" }

	const quitMock = vi.fn()
	const setNameMock = vi.fn()
	const setAboutPanelOptionsMock = vi.fn()
	const appOnMock = vi.fn()
	const whenReadyMock = vi.fn(
		() =>
			new Promise((resolve) => {
				state.resolveWhenReady = resolve
			}),
	)
	const ipcHandleMock = vi.fn()
	const netFetchMock = vi.fn(() => Promise.resolve(responseMarker))
	const registerSchemesAsPrivilegedMock = vi.fn()
	const protocolHandleMock = vi.fn()
	const onHeadersReceivedMock = vi.fn()
	const onBeforeSendHeadersMock = vi.fn()
	const openExternalMock = vi.fn()
	const browserWindowCtorMock = vi.fn()
	const loadURLMock = vi.fn()
	const setWindowOpenHandlerMock = vi.fn()
	const webContentsOnMock = vi.fn()
	const openDevToolsMock = vi.fn()
	const getAllWindowsMock = vi.fn((): unknown[] => [])
	const setupMainMock = vi.fn()
	const getCookieMock = vi.fn(() => "; ")
	const registerAuthIpcHandlersMock = vi.fn()
	const existsSyncMock = vi.fn(() => false)

	class BrowserWindowMock {
		static getAllWindows = getAllWindowsMock

		webContents = {
			setWindowOpenHandler: setWindowOpenHandlerMock,
			on: webContentsOnMock,
			openDevTools: openDevToolsMock,
		}

		loadURL = loadURLMock

		constructor(options: unknown) {
			browserWindowCtorMock(options)
		}
	}

	return {
		responseMarker,
		quitMock,
		setNameMock,
		setAboutPanelOptionsMock,
		appOnMock,
		whenReadyMock,
		ipcHandleMock,
		netFetchMock,
		registerSchemesAsPrivilegedMock,
		protocolHandleMock,
		onHeadersReceivedMock,
		onBeforeSendHeadersMock,
		openExternalMock,
		browserWindowCtorMock,
		loadURLMock,
		setWindowOpenHandlerMock,
		webContentsOnMock,
		openDevToolsMock,
		getAllWindowsMock,
		setupMainMock,
		getCookieMock,
		registerAuthIpcHandlersMock,
		existsSyncMock,
		BrowserWindowMock,
	}
})

vi.mock("electron", () => ({
	app: {
		quit: mocks.quitMock,
		get isPackaged() {
			return state.isPackaged
		},
		setName: mocks.setNameMock,
		setAboutPanelOptions: mocks.setAboutPanelOptionsMock,
		getVersion: () => "1.2.3",
		whenReady: mocks.whenReadyMock,
		on: mocks.appOnMock,
	},
	BrowserWindow: mocks.BrowserWindowMock,
	ipcMain: { handle: mocks.ipcHandleMock },
	net: { fetch: mocks.netFetchMock },
	protocol: {
		registerSchemesAsPrivileged: mocks.registerSchemesAsPrivilegedMock,
		handle: mocks.protocolHandleMock,
	},
	session: {
		defaultSession: {
			webRequest: {
				onHeadersReceived: mocks.onHeadersReceivedMock,
				onBeforeSendHeaders: mocks.onBeforeSendHeadersMock,
			},
		},
	},
	shell: { openExternal: mocks.openExternalMock },
}))

vi.mock("electron-squirrel-startup", () => ({
	get default() {
		return state.squirrelStartup
	},
}))

vi.mock("./auth-client", () => ({
	authClient: {
		setupMain: mocks.setupMainMock,
		getCookie: mocks.getCookieMock,
	},
}))

vi.mock("./auth-ipc", () => ({
	registerAuthIpcHandlers: mocks.registerAuthIpcHandlersMock,
}))

vi.mock("node:fs", () => ({
	existsSync: mocks.existsSyncMock,
}))

// main resolves both paths from its own directory, which this test file
// shares
const electronDir = path.dirname(fileURLToPath(import.meta.url))
const rendererDist = path.join(electronDir, "..", "..", ".output", "public")

// evaluating the module is the unit's act. Resolving the whenReady mock
// promise afterwards runs the ready phase; one microtask tick is enough
// because the ready callback is synchronous.
async function importMain({ packaged = false, ready = true } = {}) {
	state.isPackaged = packaged

	await import("./main")

	if (ready) {
		state.resolveWhenReady?.()
		await Promise.resolve()
	}
}

type Listener = (...args: never[]) => unknown

function registeredListener(
	mock: { mock: { calls: unknown[][] } },
	event?: string,
): Listener {
	const call =
		event === undefined
			? mock.mock.calls[0]
			: mock.mock.calls.find(([name]) => name === event)

	if (!call) {
		throw new Error(
			`listener${event ? ` for ${event}` : ""} was not registered`,
		)
	}

	return (event === undefined ? call[0] : call[1]) as Listener
}

// sequential by exception: these tests assert call accounting on shared
// module-level mocks (vi.mock singletons), which cannot be isolated
// across concurrently interleaving tests
describe("main", { concurrent: false }, () => {
	// restoreMocks only covers vi.spyOn spies, so these hand-made vi.fn
	// singletons are reset here explicitly (mockReset restores the
	// implementations passed at creation)
	beforeEach(() => {
		vi.resetModules()
		state.isPackaged = false
		state.squirrelStartup = false
		state.resolveWhenReady = null
		delete process.env.ELECTRON_DISABLE_SECURITY_WARNINGS

		for (const value of Object.values(mocks)) {
			if (typeof value === "function" && "mockReset" in value) {
				value.mockReset()
			}
		}
	})

	it("quits immediately under squirrel startup", async () => {
		state.squirrelStartup = true

		await importMain({ ready: false })

		expect(mocks.quitMock).toHaveBeenCalledTimes(1)
	})

	it("does not quit during normal startup", async () => {
		await importMain()

		expect(mocks.quitMock).toHaveBeenCalledTimes(0)
	})

	it("silences renderer security warnings in dev only", async () => {
		await importMain({ packaged: false, ready: false })

		expect(process.env.ELECTRON_DISABLE_SECURITY_WARNINGS).toBe("true")
	})

	it("keeps renderer security warnings enabled when packaged", async () => {
		await importMain({ packaged: true, ready: false })

		expect(process.env.ELECTRON_DISABLE_SECURITY_WARNINGS).toBeUndefined()
	})

	it("registers the deep-link handler with the plugin csp disabled", async () => {
		await importMain({ ready: false })

		expect(mocks.setupMainMock).toHaveBeenCalledTimes(1)
		expect(mocks.setupMainMock).toHaveBeenCalledWith({
			csp: false,
			scheme: true,
			bridges: true,
		})
	})

	it("sets the app identity", async () => {
		await importMain({ ready: false })

		expect(mocks.setNameMock).toHaveBeenCalledWith("Oxynote")
		expect(mocks.setAboutPanelOptionsMock).toHaveBeenCalledWith({
			applicationName: "Oxynote",
			applicationVersion: "1.2.3",
			copyright: "© Oxynote",
		})
	})

	it("registers the oxynote scheme as privileged", async () => {
		await importMain({ ready: false })

		expect(mocks.registerSchemesAsPrivilegedMock).toHaveBeenCalledWith([
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
	})

	it("wires the ipc and auth handlers only once ready", async () => {
		await importMain({ ready: false })

		expect(mocks.ipcHandleMock).toHaveBeenCalledTimes(0)
		expect(mocks.registerAuthIpcHandlersMock).toHaveBeenCalledTimes(0)

		state.resolveWhenReady?.()
		await Promise.resolve()

		expect(mocks.registerAuthIpcHandlersMock).toHaveBeenCalledTimes(1)
		expect(mocks.ipcHandleMock).toHaveBeenCalledTimes(1)

		const openExternal = registeredListener(
			mocks.ipcHandleMock,
			"shell:openExternal",
		) as (event: unknown, url: string) => unknown
		openExternal({}, "https://example.com")

		expect(mocks.openExternalMock).toHaveBeenCalledTimes(1)
		expect(mocks.openExternalMock).toHaveBeenCalledWith("https://example.com")
	})

	describe("content security policy", () => {
		function receivedHeaders(existing: Record<string, string[]>) {
			const listener = registeredListener(mocks.onHeadersReceivedMock) as (
				details: { responseHeaders: Record<string, string[]> },
				callback: (response: unknown) => void,
			) => void
			const callback = vi.fn()

			listener({ responseHeaders: existing }, callback)

			expect(callback).toHaveBeenCalledTimes(1)
			return callback.mock.calls[0]?.[0] as {
				responseHeaders: Record<string, string[]>
			}
		}

		it("appends the relaxed dev policy to existing headers", async () => {
			await importMain()

			const response = receivedHeaders({ "X-Existing": ["1"] })

			expect(response.responseHeaders["X-Existing"]).toEqual(["1"])
			expect(response.responseHeaders["Content-Security-Policy"]).toEqual([
				"default-src 'self'; script-src 'self' 'unsafe-eval' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self' http://test.local ws://test.local https:; img-src 'self' http://test.local user-image: data: https:; font-src 'self' data:; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'",
			])
		})

		it("locks script-src down when packaged", async () => {
			await importMain({ packaged: true })

			const response = receivedHeaders({})

			expect(response.responseHeaders["Content-Security-Policy"]).toEqual([
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' http://test.local ws://test.local https:; img-src 'self' http://test.local user-image: data: https:; font-src 'self' data:; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'",
			])
		})
	})

	describe("session cookie injection", () => {
		function injectedHeaders(
			url: string,
			requestHeaders: Record<string, string>,
		) {
			const call = mocks.onBeforeSendHeadersMock.mock.calls[0] as
				| [unknown, unknown]
				| undefined

			if (!call) {
				throw new Error("onBeforeSendHeaders listener was not registered")
			}

			const listener = call[1] as (
				details: { url: string; requestHeaders: Record<string, string> },
				callback: (response: unknown) => void,
			) => void
			const callback = vi.fn()

			listener({ url, requestHeaders }, callback)

			expect(callback).toHaveBeenCalledTimes(1)
			return (
				callback.mock.calls[0]?.[0] as {
					requestHeaders: Record<string, string>
				}
			).requestHeaders
		}

		it("watches the api http and websocket origins", async () => {
			await importMain()

			expect(mocks.onBeforeSendHeadersMock).toHaveBeenCalledTimes(1)
			expect(mocks.onBeforeSendHeadersMock.mock.calls[0]?.[0]).toEqual({
				urls: ["http://test.local/*", "ws://test.local/*"],
			})
		})

		it("leaves auth endpoint requests untouched", async () => {
			await importMain()

			const headers = injectedHeaders(
				"http://test.local/api/auth/get-session",
				{ Accept: "application/json" },
			)

			expect(headers).toEqual({ Accept: "application/json" })
			expect(mocks.getCookieMock).toHaveBeenCalledTimes(0)
		})

		it("passes requests through when no session cookie exists", async () => {
			await importMain()
			mocks.getCookieMock.mockReturnValue("; ")

			const headers = injectedHeaders("http://test.local/api/documents", {})

			expect(headers).toEqual({})
		})

		it("injects the session cookie and drops stale auth cookies", async () => {
			await importMain()
			mocks.getCookieMock.mockReturnValue("; auth.session_token=fresh")

			const headers = injectedHeaders("http://test.local/api/documents", {
				cookie: "i18n_redirected=en; auth.session_token=stale; foo=bar",
			})

			expect(headers).toEqual({
				Cookie: "i18n_redirected=en; foo=bar; auth.session_token=fresh",
			})
		})

		it("sets the cookie header when the request has none", async () => {
			await importMain()
			mocks.getCookieMock.mockReturnValue("; auth.session_token=fresh")

			const headers = injectedHeaders("ws://test.local/hocuspocus", {
				"User-Agent": "test",
			})

			expect(headers).toEqual({
				"User-Agent": "test",
				Cookie: "auth.session_token=fresh",
			})
		})
	})

	describe("oxynote protocol", () => {
		function protocolHandler() {
			return registeredListener(
				mocks.protocolHandleMock,
				"oxynote",
			) as (request: { url: string }) => Promise<unknown>
		}

		it("does not register the protocol handler in dev", async () => {
			await importMain()

			expect(mocks.protocolHandleMock).toHaveBeenCalledTimes(0)
		})

		it("serves existing files from the renderer bundle", async () => {
			await importMain({ packaged: true })
			mocks.existsSyncMock.mockReturnValue(true)
			const candidate = path.join(rendererDist, "/assets/app.js")

			const result = protocolHandler()({ url: "oxynote://app/assets/app.js" })

			await expect(result).resolves.toBe(mocks.responseMarker)
			expect(mocks.existsSyncMock).toHaveBeenCalledWith(candidate)
			expect(mocks.netFetchMock).toHaveBeenCalledTimes(1)
			expect(mocks.netFetchMock).toHaveBeenCalledWith(
				pathToFileURL(candidate).toString(),
			)
		})

		it("serves index.html for the root path", async () => {
			await importMain({ packaged: true })
			mocks.existsSyncMock.mockReturnValue(true)
			const candidate = path.join(rendererDist, "/index.html")

			const result = protocolHandler()({ url: "oxynote://app/" })

			await expect(result).resolves.toBe(mocks.responseMarker)
			expect(mocks.netFetchMock).toHaveBeenCalledWith(
				pathToFileURL(candidate).toString(),
			)
		})

		it("falls back to index.html for unresolved paths", async () => {
			await importMain({ packaged: true })
			mocks.existsSyncMock.mockReturnValue(false)

			const result = protocolHandler()({ url: "oxynote://app/notes/123" })

			await expect(result).resolves.toBe(mocks.responseMarker)
			expect(mocks.netFetchMock).toHaveBeenCalledWith(
				pathToFileURL(path.join(rendererDist, "index.html")).toString(),
			)
		})
	})

	describe("window creation", () => {
		it("opens the main window against the dev server in dev", async () => {
			await importMain()

			expect(mocks.browserWindowCtorMock).toHaveBeenCalledTimes(1)
			expect(mocks.browserWindowCtorMock).toHaveBeenCalledWith({
				width: 1400,
				height: 900,
				minWidth: 1024,
				minHeight: 640,
				webPreferences: {
					preload: path.join(electronDir, "preload.cjs"),
					contextIsolation: true,
					nodeIntegration: false,
					sandbox: true,
				},
			})
			expect(mocks.loadURLMock).toHaveBeenCalledWith("http://localhost:3000")
			expect(mocks.openDevToolsMock).toHaveBeenCalledWith({ mode: "detach" })
		})

		it("loads the packaged app over the oxynote scheme", async () => {
			await importMain({ packaged: true })

			expect(mocks.loadURLMock).toHaveBeenCalledWith("oxynote://app/index.html")
			expect(mocks.openDevToolsMock).toHaveBeenCalledTimes(0)
		})

		it.for([
			{ url: "https://example.com/page", external: true },
			{ url: "oxynote://app/index.html", external: false },
			{ url: "oxynote://elsewhere/index.html", external: true },
			{ url: "http://localhost:3000/docs", external: false },
			{ url: "about:blank", external: false },
			{ url: "not a url", external: true },
		])(
			"window.open for $url is denied and $external routes it to the os",
			async ({ url, external }, { expect }) => {
				await importMain()
				const handler = registeredListener(
					mocks.setWindowOpenHandlerMock,
				) as (details: { url: string }) => { action: string }

				expect(handler({ url })).toEqual({ action: "deny" })
				expect(mocks.openExternalMock).toHaveBeenCalledTimes(external ? 1 : 0)

				if (external) {
					expect(mocks.openExternalMock).toHaveBeenCalledWith(url)
				}
			},
		)

		it("redirects external top-level navigations to the os", async () => {
			await importMain()
			const handler = registeredListener(
				mocks.webContentsOnMock,
				"will-navigate",
			) as (event: { preventDefault: () => void }, url: string) => void
			const preventDefault = vi.fn()

			handler({ preventDefault }, "https://example.com/page")

			expect(preventDefault).toHaveBeenCalledTimes(1)
			expect(mocks.openExternalMock).toHaveBeenCalledTimes(1)
			expect(mocks.openExternalMock).toHaveBeenCalledWith(
				"https://example.com/page",
			)
		})

		it("allows internal top-level navigations", async () => {
			await importMain()
			const handler = registeredListener(
				mocks.webContentsOnMock,
				"will-navigate",
			) as (event: { preventDefault: () => void }, url: string) => void
			const preventDefault = vi.fn()

			handler({ preventDefault }, "http://localhost:3000/docs")

			expect(preventDefault).toHaveBeenCalledTimes(0)
			expect(mocks.openExternalMock).toHaveBeenCalledTimes(0)
		})

		it("opens a window on activate when none are open", async () => {
			await importMain()
			mocks.getAllWindowsMock.mockReturnValue([])

			const activate = registeredListener(mocks.appOnMock, "activate")
			activate()

			expect(mocks.browserWindowCtorMock).toHaveBeenCalledTimes(2)
		})

		it("does not open another window on activate when one exists", async () => {
			await importMain()
			mocks.getAllWindowsMock.mockReturnValue([{}])

			const activate = registeredListener(mocks.appOnMock, "activate")
			activate()

			expect(mocks.browserWindowCtorMock).toHaveBeenCalledTimes(1)
		})
	})

	describe("window-all-closed", () => {
		// the handler reads process.platform at invocation time, so each
		// case stubs it just around the call
		function invokeOnPlatform(platform: string) {
			const original = Object.getOwnPropertyDescriptor(process, "platform")

			if (!original) {
				throw new Error("process.platform descriptor is missing")
			}

			Object.defineProperty(process, "platform", {
				value: platform,
				configurable: true,
			})

			try {
				registeredListener(mocks.appOnMock, "window-all-closed")()
			} finally {
				Object.defineProperty(process, "platform", original)
			}
		}

		it("quits when the last window closes on non-mac platforms", async () => {
			await importMain()

			invokeOnPlatform("win32")

			expect(mocks.quitMock).toHaveBeenCalledTimes(1)
		})

		it("keeps running when the last window closes on macOS", async () => {
			await importMain()

			invokeOnPlatform("darwin")

			expect(mocks.quitMock).toHaveBeenCalledTimes(0)
		})
	})
})
