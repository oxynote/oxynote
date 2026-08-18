import { describe, it, vi } from "vitest"
import plugin from "./electron-auth.client"

describe("electron-auth.client", () => {
	// the desktop wiring is compile-time dead in the client web bundle
	// tests run in — see the NOCOV marker in the plugin
	it("does not subscribe to the auth bridges in web builds", ({ expect }) => {
		const onAuthenticated = vi.fn()
		const onUserUpdated = vi.fn()
		const onAuthError = vi.fn()
		vi.stubGlobal("onAuthenticated", onAuthenticated)
		vi.stubGlobal("onUserUpdated", onUserUpdated)
		vi.stubGlobal("onAuthError", onAuthError)

		const result = plugin(useNuxtApp())

		expect(result).toBeUndefined()
		expect(onAuthenticated).toHaveBeenCalledTimes(0)
		expect(onUserUpdated).toHaveBeenCalledTimes(0)
		expect(onAuthError).toHaveBeenCalledTimes(0)
	})
})
