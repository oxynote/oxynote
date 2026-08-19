import { describe, it } from "vitest"
import { isReadonly } from "vue"
import useDetectHost, {
	HostBrowserType,
	HostOsType,
	HostPlatformType,
} from "./useDetectHost"

// the composable is backed by app-wide useState, which every test in this
// file shares — the tests cannot interleave, and each arranges the state
// it asserts
describe("useDetectHost", { concurrent: false }, () => {
	it("reports the web platform in web builds", ({ expect }) => {
		const { platformType, isWeb, isDesktop } = useDetectHost()

		expect(platformType.value).toBe(HostPlatformType.Web)
		expect(isWeb.value).toBe(true)
		expect(isDesktop.value).toBe(false)
	})

	it("updates the os type", ({ expect }) => {
		const { osType, setOsType } = useDetectHost()

		setOsType(HostOsType.MacOS)

		expect(osType.value).toBe(HostOsType.MacOS)
	})

	it("updates the browser type", ({ expect }) => {
		const { browserType, setBrowserType } = useDetectHost()

		setBrowserType(HostBrowserType.Firefox)

		expect(browserType.value).toBe(HostBrowserType.Firefox)
	})

	it("exposes the detection state as readonly refs", ({ expect }) => {
		const { platformType, osType, browserType } = useDetectHost()

		expect(isReadonly(platformType)).toBe(true)
		expect(isReadonly(osType)).toBe(true)
		expect(isReadonly(browserType)).toBe(true)
	})
})
