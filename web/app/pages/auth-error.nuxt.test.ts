import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import AuthError from "./auth-error.vue"
import { findButtonByText, t } from "../components/test-helpers"

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock, navigateToMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
	navigateToMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)
mockNuxtImport("navigateTo", () => navigateToMock)

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "auth-error")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the auth-error page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

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
		navigateToMock.mockReset()
	})

	it("sends a visitor without an error code home", async ({ expect }) => {
		const to = { query: {} } as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledWith("/")
	})

	it("offers a way back home", async ({ expect }) => {
		const wrapper = await mountError()

		await findButtonByText(wrapper, t("oauth.error.button")).trigger("click")

		expect(navigateToMock).toHaveBeenCalledWith("/")
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
