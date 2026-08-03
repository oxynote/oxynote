import type { NuxtApp } from "#app"

// Provides `$host`, the renderer-side handle to the Electron preload bridge.
// Electron's preload writes `window.__host` via `contextBridge.exposeInMainWorld`;
// this plugin re-exposes it through Nuxt's provide system. Web builds skip the
// body entirely (the constant is `false` and Vite tree-shakes the dead branch).
export default defineNuxtPlugin(() => {
	if (!__DESKTOP_BUILD__) return

	const host = (window as { __host?: NuxtApp["$host"] }).__host
	if (!host) {
		console.warn("[host] preload bridge not found on window.__host")
		return
	}

	return { provide: { host } }
})
