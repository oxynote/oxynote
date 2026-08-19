import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import useExternalLinks from "./useExternalLinks"

const { useDetectHostMock } = vi.hoisted(() => {
	// plain value objects instead of vue refs: the default implementation
	// runs during nuxt bootstrap (the 01.process-user-agent plugin), where
	// only property reads happen
	return {
		useDetectHostMock: vi.fn((): any => ({
			platformType: { value: "web" },
			isWeb: { value: true },
			isDesktop: { value: false },
			osType: { value: "other" },
			browserType: { value: "non-browser" },
			setOsType: () => undefined,
			setBrowserType: () => undefined,
		})),
	}
})

mockNuxtImport("useDetectHost", () => useDetectHostMock)

function arrange(opts: {
	isDesktop: boolean
	host?: { openExternal: (url: string) => Promise<void> }
}) {
	useDetectHostMock.mockReturnValue({ isDesktop: { value: opts.isDesktop } })

	// $host is normally provided by the host.client plugin in desktop
	// builds; assign it directly so each test controls the bridge
	const nuxtApp = useNuxtApp() as unknown as { $host?: typeof opts.host }
	nuxtApp.$host = opts.host

	const windowOpen = vi.spyOn(window, "open").mockImplementation(() => null)

	return { windowOpen }
}

// the tests arrange a shared module-level mock (mockNuxtImport singleton)
// and the shared nuxt app instance, so they cannot interleave
describe("useExternalLinks", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mock explicitly
	beforeEach(() => {
		useDetectHostMock.mockReset()
	})

	it("opens links in a new browser tab on the web", ({ expect }) => {
		const { windowOpen } = arrange({ isDesktop: false })
		const { openExternalLink } = useExternalLinks()

		openExternalLink("https://example.com/docs")

		expect(windowOpen).toHaveBeenCalledExactlyOnceWith(
			"https://example.com/docs",
			"_blank",
			"noopener",
		)
	})

	it("routes links through the host bridge on desktop", ({ expect }) => {
		const openExternal = vi.fn().mockResolvedValue(undefined)
		const { windowOpen } = arrange({
			isDesktop: true,
			host: { openExternal },
		})
		const { openExternalLink } = useExternalLinks()

		openExternalLink("https://example.com/docs")

		expect(openExternal).toHaveBeenCalledExactlyOnceWith(
			"https://example.com/docs",
		)
		expect(windowOpen).toHaveBeenCalledTimes(0)
	})

	it("ignores desktop links when the host bridge is missing", ({ expect }) => {
		const { windowOpen } = arrange({ isDesktop: true })
		const { openExternalLink } = useExternalLinks()

		openExternalLink("https://example.com/docs")

		expect(windowOpen).toHaveBeenCalledTimes(0)
	})
})
