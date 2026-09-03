import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import AuthError from "./auth-error.vue"
import { t } from "../components/test-helpers"

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock } = vi.hoisted(() => ({ useRouteMock: vi.fn() }))

mockNuxtImport("useRoute", () => useRouteMock)

async function mountError(query = "?error=invalid_client") {
	useRouteMock.mockReturnValue({
		query: Object.fromEntries(new URLSearchParams(query).entries()),
	})

	return await mountSuspended(AuthError)
}

// the useRoute module mock is an app-wide singleton every mount in the
// file shares
describe("<auth-error>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		useRouteMock.mockReset()
	})

	it("tells the visitor the authorization failed", async ({ expect }) => {
		const wrapper = await mountError()

		expect(wrapper.text()).toContain(t("oauth.error.heading"))
		expect(wrapper.text()).toContain(t("oauth.error.description"))
	})

	it("names the error code the server reported", async ({ expect }) => {
		const wrapper = await mountError()

		expect(wrapper.text()).toContain("invalid_client")
	})

	it("leaves the error description out of the page", async ({ expect }) => {
		const wrapper = await mountError(
			"?error=invalid_client&error_description=call+1-800-not-oxynote",
		)

		expect(wrapper.text()).not.toContain("1-800-not-oxynote")
	})
})
