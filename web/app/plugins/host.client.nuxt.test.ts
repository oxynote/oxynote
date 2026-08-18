import { describe, it, vi } from "vitest"
import plugin from "./host.client"

describe("host.client", () => {
	// the bridge lookup is compile-time dead in the client web bundle tests
	// run in — see the NOCOV marker in the plugin
	it("ignores the preload bridge in web builds", ({ expect }) => {
		vi.stubGlobal("__host", { osType: "macOS" })

		const result = plugin(useNuxtApp())

		expect(result).toBeUndefined()
	})
})
