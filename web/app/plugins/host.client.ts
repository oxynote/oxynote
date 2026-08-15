// renderer-side shape of the bridge exposed by electron/preload.ts via
// contextBridge.exposeInMainWorld("__host", ...) — keep in sync with it.
// Referencing NuxtApp["$host"] here instead would be circular ($host's type
// is generated from this plugin's return value), which noImplicitAny rejects.
interface HostBridge {
	osType: "macOS" | "windows" | "linux" | "other"
	openExternal: (url: string) => Promise<void>
	auth: Record<string, (args?: unknown) => Promise<any>>
}

// Provides `$host`, the renderer-side handle to the Electron preload bridge.
// Electron's preload writes `window.__host` via `contextBridge.exposeInMainWorld`;
// this plugin re-exposes it through Nuxt's provide system. Web builds skip the
// body entirely (the constant is `false` and Vite tree-shakes the dead branch).
export default defineNuxtPlugin(() => {
	if (!__DESKTOP_BUILD__) return

	const host = (window as { __host?: HostBridge }).__host
	if (!host) {
		console.warn("[host] preload bridge not found on window.__host")
		return
	}

	return { provide: { host } }
})
