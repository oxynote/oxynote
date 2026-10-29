import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import Signup from "./signup.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	raisedToasts,
	seedAuthConfig,
	t,
	WAIT_FOR_OPTIONS,
} from "../components/test-helpers"

const PASSWORD = "correct-horse-battery-1!"
const SESSION = {
	session: { id: "session-1" },
	user: { id: "user-1", email: "ada@example.com" },
}

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock, navigateToMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
	navigateToMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)
mockNuxtImport("navigateTo", () => navigateToMock)

// the query cache, the runtime config and the vue-sonner, useRoute and
// navigateTo module mocks are app-wide singletons every mount in the file
// shares
describe("<signup>", { concurrent: false }, () => {
	// the page polls for the desktop redirect until it unmounts
	enableAutoUnmount(afterEach)

	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		useRouteMock.mockReset()
		arrangeRoute()
		seedSlots(null)
	})

	afterEach(() => {
		disposeMockEndpoints()
		setConditionURLs("", "")
	})

	it.for([
		{
			name: "sends a visitor home in single-workspace mode",
			single: true,
			next: undefined,
			redirects: true,
		},
		{
			name: "lets an invitee through in single-workspace mode",
			single: true,
			next: "/accept-invite?id=invite-1",
			redirects: false,
		},
		{
			name: "lets anyone through outside single-workspace mode",
			single: false,
			next: undefined,
			redirects: false,
		},
	])("$name", async ({ single, next, redirects }, { expect }) => {
		seedAuthConfig({ singleOrganization: single })
		const to = { query: { next: next } } as unknown as RouteLocationNormalized

		await runInApp(() => pageMiddleware()(to, to))

		expect(navigateToMock.mock.calls).toEqual(
			redirects ? [["/", { replace: true }]] : [],
		)
	})

	it("confirms a deleted account on arrival", async ({ expect }) => {
		seedAuthConfig()
		arrangeRoute("?deletion=success")

		await mountSuspended(Signup)

		expect(raisedToasts()).toMatchObject([
			{
				type: "success",
				title: t(
					"settings.action-modals.account-deletion.success-message.account-deleted.title",
				),
				description: t(
					"settings.action-modals.account-deletion.success-message.account-deleted.description",
				),
			},
		])
	})

	it("shows the email/password form without a way back when it is the only enabled method", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["email-password"],
		})

		const wrapper = await mountSuspended(Signup)

		expect(wrapper.find('input[type="email"]').exists()).toBe(true)
		expect(wrapper.text()).not.toContain(
			t("onboarding.signup.email-password-form.back"),
		)
	})

	it("goes back to the methods from the email/password form", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["google", "email-password"],
		})
		const wrapper = await mountSuspended(Signup)
		await findButtonByText(
			wrapper,
			t("onboarding.signup.signup-email-password"),
		).trigger("click")

		await findButtonByText(
			wrapper,
			t("onboarding.signup.email-password-form.back"),
		).trigger("click")

		expect(wrapper.find('input[type="email"]').exists()).toBe(false)
		expect(wrapper.text()).toContain(t("onboarding.signup.signup-google"))
	})

	it("marks the first of three or more methods as primary", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["github", "slack", "email-password"] })

		const wrapper = await mountSuspended(Signup)

		expect(primaryButtonTexts(wrapper)).toEqual([
			t("onboarding.signup.signup-github"),
		])
	})

	it("marks no method as primary with only two to pick from", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google", "email-password"] })

		const wrapper = await mountSuspended(Signup)

		expect(primaryButtonTexts(wrapper)).toEqual([])
	})

	it("explains that no sign-up method is configured", async ({ expect }) => {
		seedAuthConfig({ methods: [] })

		const wrapper = await mountSuspended(Signup)

		expect(wrapper.text()).toContain(t("onboarding.signup.no-methods"))
	})

	it("claims no missing method while the auth config is still loading", async ({
		expect,
	}) => {
		const config = mockDeferredEndpoint(
			"GET",
			"http://test.local/auth-realtime/api/auth-config",
		)

		const wrapper = await mountSuspended(Signup)
		await config.reached

		expect.soft(wrapper.text()).not.toContain(t("onboarding.signup.no-methods"))
		expect.soft(wrapper.findAll("button")).toHaveLength(0)
	})

	it.for([
		{
			name: "links both conditions when both are configured",
			terms: "https://example.com/terms",
			privacy: "https://example.com/privacy",
			expected: ["https://example.com/terms", "https://example.com/privacy"],
		},
		{
			name: "links only the terms when no privacy policy is configured",
			terms: "https://example.com/terms",
			privacy: "",
			expected: ["https://example.com/terms"],
		},
		{
			name: "links only the privacy policy when no terms are configured",
			terms: "",
			privacy: "https://example.com/privacy",
			expected: ["https://example.com/privacy"],
		},
		{
			name: "shows no conditions when none are configured",
			terms: "",
			privacy: "",
			expected: [],
		},
	])("$name", async ({ terms, privacy, expected }, { expect }) => {
		seedAuthConfig()
		setConditionURLs(terms, privacy)

		const wrapper = await mountSuspended(Signup)

		expect(
			wrapper
				.findAll("a[rel='noopener noreferrer']")
				.map((a) => a.attributes("href")),
		).toEqual(expected)
	})

	it.for(["google", "github", "slack"] as const)(
		"signs up with %s and lands on the welcome page",
		async (provider, { expect }) => {
			seedAuthConfig({ methods: [provider] })
			const calls = mockSocialSignIn()
			const wrapper = await mountSuspended(Signup)

			await findButtonByText(
				wrapper,
				t(`onboarding.signup.signup-${provider}`),
			).trigger("click")
			await settleRequest(calls)

			expect.soft(calls[0]?.body).toEqual({
				provider: provider,
				callbackURL: "http://test.local/",
				newUserCallbackURL: "http://test.local/welcome",
			})
			expect.soft(toast.custom).not.toHaveBeenCalled()
		},
	)

	it("sends an invitee's social signup on to the invitation with no workspace slot left", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		seedSlots(0)
		arrangeRoute("?next=%2Faccept-invite%3Fid%3Dinvite-1")
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Signup)

		await findButtonByText(
			wrapper,
			t("onboarding.signup.signup-google"),
		).trigger("click")
		await settleRequest(calls)

		expect.soft(calls[0]?.query).toEqual({ next: "/accept-invite?id=invite-1" })
		expect.soft(calls[0]?.body).toEqual({
			provider: "google",
			callbackURL: "http://test.local/accept-invite?id=invite-1",
			newUserCallbackURL: "http://test.local/accept-invite?id=invite-1",
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("signs up with a social provider while workspace slots remain", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		seedSlots(2)
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Signup)

		await findButtonByText(
			wrapper,
			t("onboarding.signup.signup-google"),
		).trigger("click")
		await settleRequest(calls)

		expect.soft(calls).toHaveLength(1)
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("refuses a social signup once no workspace slot is left", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		seedSlots(0)
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Signup)

		await findButtonByText(
			wrapper,
			t("onboarding.signup.signup-google"),
		).trigger("click")

		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "info",
				title: t("onboarding.signup.errors.organization-limit-reached.title"),
				description: t(
					"onboarding.signup.errors.organization-limit-reached.description",
				),
			},
		])
		expect.soft(calls).toHaveLength(0)
	})

	it("reports a social signup the server refused", async ({ expect }) => {
		seedAuthConfig({ methods: ["google"] })
		mockAuthEndpoint("sign-in/social", () => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await mountSuspended(Signup)

		await findButtonByText(
			wrapper,
			t("onboarding.signup.signup-google"),
		).trigger("click")

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.signup.errors.signup-failed") },
			])
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.signup.signup-google"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it.for([
		{
			name: "sends a new email/password account to check its inbox",
			query: "",
			expected: "http://test.local/login?verified=true",
		},
		{
			name: "keeps an invitee's invitation in the emailed verification link",
			query: "?next=%252Faccept-invite%253Fid%253Dinvite-1",
			expected:
				"http://test.local/login?verified=true&next=%252Faccept-invite%253Fid%253Dinvite-1",
		},
	])("$name", async ({ query, expected }, { expect }) => {
		seedAuthConfig()
		arrangeRoute(query)
		const calls = mockEmailSignUp()
		const wrapper = await mountSuspended(Signup)

		await submitCredentials(wrapper)

		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(navigateToMock).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(calls).toHaveLength(1)
		expect.soft(calls[0]?.body).toEqual({
			email: "ada@example.com",
			password: PASSWORD,
			name: "ada",
			callbackURL: expected,
		})
		expect.soft(navigateToMock).toHaveBeenCalledWith({
			path: "/verify-email",
			query: { new: "ada@example.com", sent: "true" },
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("refuses an email/password signup once no workspace slot is left", async ({
		expect,
	}) => {
		seedAuthConfig()
		seedSlots(0)
		const calls = mockEmailSignUp()
		const wrapper = await mountSuspended(Signup)

		await submitCredentials(wrapper)

		// vee-validate validates on its own scheduler before the handler
		// runs, and nothing signals when that is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "info",
				title: t("onboarding.signup.errors.organization-limit-reached.title"),
			},
		])
		expect.soft(calls).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("reports an email/password signup the server refused", async ({
		expect,
	}) => {
		seedAuthConfig()
		mockAuthEndpoint("sign-up/email", () => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await mountSuspended(Signup)

		await submitCredentials(wrapper)

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.signup.errors.signup-failed") },
			])
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.signup.email-password-form.continue"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it.for([
		{
			name: "signs a new account straight in to the root when the server sends no email",
			query: "",
			expected: "/",
		},
		{
			name: "signs a new account straight in to the decoded next page when the server sends no email",
			query: "?next=%252Fteam%252Fdoc",
			expected: "/team/doc",
		},
	])("$name", async ({ query, expected }, { expect }) => {
		seedAuthConfig({ emailEnabled: false })
		arrangeRoute(query)
		mockEmailSignUp()
		const wrapper = await mountSuspended(Signup)
		mockAuthEndpoint("get-session", () => SESSION, "GET")

		await submitCredentials(wrapper)

		// vee-validate's validation and better-auth's requests run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(navigateToMock).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		// better-auth reads the session itself after a signup too, so the
		// cache entry is what shows the page refetched it
		expect.soft(readQueryData(["auth", "session"])).toEqual({
			data: SESSION,
			error: null,
		})
		expect.soft(navigateToMock).toHaveBeenCalledWith(expected, {
			replace: true,
		})
	})
})

function arrangeRoute(query = "") {
	useRouteMock.mockReturnValue({
		query: Object.fromEntries(new URLSearchParams(query).entries()),
		hash: "",
		fullPath: `/signup${query}`,
	})
}

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "signup")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the signup page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

// unseeded, the stats query would go out to the network, and a missing
// answer reads as no slot left
function seedSlots(availableSlots: number | null) {
	seedQueryData(["organizations", "stats"], { availableSlots: availableSlots })
}

function setConditionURLs(terms: string, privacy: string) {
	runInApp(() => {
		const config = useRuntimeConfig()

		config.public.termsOfServiceURL = terms
		config.public.privacyPolicyURL = privacy
	})
}

// redirect: false keeps better-auth from following the provider url
function mockSocialSignIn() {
	return mockAuthEndpoint("sign-in/social", () => ({
		url: "https://accounts.example.com/authorize",
		redirect: false,
	}))
}

function mockEmailSignUp() {
	return mockAuthEndpoint("sign-up/email", () => ({
		token: null,
		user: { id: "user-1", email: "ada@example.com" },
	}))
}

// better-auth's request runs on its own scheduler, and the page signals
// nothing once it is done
function settleRequest(calls: unknown[]) {
	return vi.waitFor(() => {
		if (calls.length !== 1) {
			throw new Error(`expected one request, got ${calls.length}`)
		}
	}, WAIT_FOR_OPTIONS)
}

// the submit handler prevents the default itself, and happy-dom raises no
// submit event from a click on a submit button
async function submitCredentials(wrapper: VueWrapper) {
	await wrapper.get('input[type="email"]').setValue("ada@example.com")
	await wrapper.get('input[type="password"]').setValue(PASSWORD)
	await wrapper.get("form").trigger("submit")
}

function primaryButtonTexts(wrapper: VueWrapper) {
	return wrapper
		.findAll("button")
		.filter((b) => b.classes().includes("bg-primary"))
		.map((b) => b.text())
}
