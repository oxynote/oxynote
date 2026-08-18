import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, expect, it, vi } from "vitest"
import useCurrentUrl from "./useCurrentUrl"

const { useRouteMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)

// the app base url comes from the runtimeConfig override in
// vitest.config.ts ("http://test.local"); every test arranges its own route
function arrange(fullPath: string) {
	useRouteMock.mockReturnValue({ fullPath })
}

describe("useCurrentUrl", () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mock explicitly
	beforeEach(() => {
		useRouteMock.mockReset()
	})

	describe("createCurrentUrl", () => {
		it("builds the url from the app base and the route path", () => {
			arrange("/docs/abc?x=1")

			const { createCurrentUrl } = useCurrentUrl()

			expect(createCurrentUrl().toString()).toBe(
				"http://test.local/docs/abc?x=1",
			)
		})

		it("falls back to the root path when the route path is empty", () => {
			arrange("")

			const { createCurrentUrl } = useCurrentUrl()

			expect(createCurrentUrl().toString()).toBe("http://test.local/")
		})

		it("sets the hash when one is given", () => {
			arrange("/docs/abc")

			const { createCurrentUrl } = useCurrentUrl()

			expect(createCurrentUrl({ hash: "section-2" }).hash).toBe("#section-2")
		})

		it("clears the hash when null is given", () => {
			arrange("/docs/abc#old")

			const { createCurrentUrl } = useCurrentUrl()

			expect(createCurrentUrl({ hash: null }).hash).toBe("")
		})
	})

	describe("currentUrl", () => {
		it("exposes the current route as a computed url", () => {
			arrange("/docs/abc")

			const { currentUrl } = useCurrentUrl()

			expect(currentUrl.value.toString()).toBe("http://test.local/docs/abc")
		})
	})
})
