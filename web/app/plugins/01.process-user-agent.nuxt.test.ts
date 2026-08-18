import { describe, it } from "vitest"
import plugin from "./01.process-user-agent"

describe("01.process-user-agent", () => {
	// the server and desktop branches are compile-time dead in the client
	// web bundle tests run in — see the NOCOV markers in the plugin. The
	// detection logic itself is covered in app/utils/user-agent.test.ts.
	it("leaves host detection at its defaults on web clients", ({ expect }) => {
		const { osType, browserType } = useDetectHost()

		const result = plugin(useNuxtApp())

		expect(result).toBeUndefined()
		expect(osType.value).toBe(HostOsType.Other)
		expect(browserType.value).toBe(HostBrowserType.NonBrowser)
	})
})
