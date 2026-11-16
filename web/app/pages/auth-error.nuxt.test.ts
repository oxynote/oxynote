import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import AuthError from "./auth-error.vue"
import { findButtonByText, seedAuthConfig, t } from "../components/test-helpers"

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
		seedAuthConfig()
	})

	it("sends a visitor without an error code home", async ({ expect }) => {
		const to = { query: {} } as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledWith("/")
	})

	it("lets a visitor with an error code through", async ({ expect }) => {
		const to = {
			query: { error: "invalid_client" },
		} as unknown as RouteLocationNormalized

		const result = await pageMiddleware()(to, to)

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
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

	it.for([
		{ input: "SIGN_UP_DISABLED", expected: "oauth.error.sign-up-disabled" },
		{
			input: "account_not_linked",
			expected: "oauth.error.account-not-linked",
		},
		{
			input: "email_not_verified",
			expected: "oauth.error.email-not-verified",
		},
	])(
		"explains a refused sign-in reported as $input",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountError(`?error=${input}`)

			expect.soft(wrapper.text()).toContain(t(expected))
			expect.soft(wrapper.text()).not.toContain(t("oauth.error.description"))
			expect.soft(wrapper.text()).toContain(input)
		},
	)

	it("ends the identity provider's session to sign in as someone else", async ({
		expect,
	}) => {
		const wrapper = await mountError("?error=SIGN_UP_DISABLED&provider=oidc")

		await findButtonByText(wrapper, t("oauth.error.other-account")).trigger(
			"click",
		)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			`${useRuntimeConfig().public.authRealtimeAPIBaseHttpURL}/api/oidc/end-session`,
			{ external: true },
		)
	})

	it.for([
		{
			name: "after another provider refused the sign-in",
			input: "?error=SIGN_UP_DISABLED",
		},
		{
			name: "for an error that is not a refused sign-in",
			input: "?error=invalid_client&provider=oidc",
		},
	])("offers no other account $name", async ({ input }, { expect }) => {
		const wrapper = await mountError(input)

		expect(wrapper.text()).not.toContain(t("oauth.error.other-account"))
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
