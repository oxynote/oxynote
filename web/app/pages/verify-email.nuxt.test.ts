import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import VerifyEmail from "./verify-email.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	seedAuthSession,
	t,
} from "../components/test-helpers"

const ADDRESS = "new@example.com"

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock, navigateToMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
	navigateToMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)
mockNuxtImport("navigateTo", () => navigateToMock)

// the useRoute and navigateTo module mocks and the query cache are app-wide
// singletons every mount in the file shares
describe("<verify-email>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		useRouteMock.mockReset()
		navigateToMock.mockReset()
	})

	afterEach(disposeMockEndpoints)

	it.for([
		{ name: "sends a visitor without an address home", query: {} },
		{
			name: "sends a visitor with a repeated address home",
			query: { new: [ADDRESS, ADDRESS] },
		},
	])("$name", async ({ query }, { expect }) => {
		const to = { query: query } as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/")
	})

	it("lets a visitor with an address through", async ({ expect }) => {
		const to = { query: { new: ADDRESS } } as unknown as RouteLocationNormalized

		const result = await pageMiddleware()(to, to)

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it("reads the session before rendering", async ({ expect }) => {
		const calls = mockAuthEndpoint(
			"get-session",
			() => ({ session: { id: "session-1" }, user: { email: ADDRESS } }),
			"GET",
		)

		const wrapper = await mountVerifyEmail({ new: ADDRESS })

		expect(calls).toHaveLength(1)
		expect(wrapper.text()).toContain(t("onboarding.verify-email.heading"))
	})

	it.for<{
		name: string
		query: Record<string, string>
		sessionEmail: string | null
		heading: string
		body: string
	}>([
		{
			name: "asks a new signup to check the inbox",
			query: { new: ADDRESS, sent: "true" },
			sessionEmail: null,
			heading: "onboarding.verify-email.sent-heading",
			body: "onboarding.verify-email.sent-title",
		},
		{
			name: "thanks the user once the session carries the new address",
			query: { new: ADDRESS },
			sessionEmail: ADDRESS,
			heading: "onboarding.verify-email.heading",
			body: "onboarding.verify-email.title",
		},
		{
			name: "asks the user to open the second link while the old address still holds",
			query: { new: ADDRESS },
			sessionEmail: "old@example.com",
			heading: "onboarding.verify-email.approved-heading",
			body: "onboarding.verify-email.approved-title",
		},
		{
			name: "asks a signed-out visitor to open the second link",
			query: { new: ADDRESS },
			sessionEmail: null,
			heading: "onboarding.verify-email.approved-heading",
			body: "onboarding.verify-email.approved-title",
		},
	])("$name", async ({ query, sessionEmail, heading, body }, { expect }) => {
		seedAuthSession(sessionEmail ? { email: sessionEmail } : null)

		const wrapper = await mountVerifyEmail(query)

		expect(wrapper.text()).toContain(t(heading))
		expect(wrapper.text()).toContain(t(body, { email: ADDRESS }))
	})

	it("leads a new signup back to the login page", async ({ expect }) => {
		seedAuthSession(null)
		const wrapper = await mountVerifyEmail({ new: ADDRESS, sent: "true" })

		await findButtonByText(
			wrapper,
			t("onboarding.verify-email.back-to-login"),
		).trigger("click")

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({ name: "login" })
	})

	it("leads a verified user into the app", async ({ expect }) => {
		seedAuthSession({ email: ADDRESS })
		const wrapper = await mountVerifyEmail({ new: ADDRESS })

		await findButtonByText(
			wrapper,
			t("onboarding.verify-email.button"),
		).trigger("click")

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/")
	})
})

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "verify-email")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the verify-email page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

async function mountVerifyEmail(query: Record<string, string>) {
	useRouteMock.mockReturnValue({ query: query })

	return await mountSuspended(VerifyEmail)
}
