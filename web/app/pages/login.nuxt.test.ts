import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
} from "~/composables/api/test-helpers"
import Login from "./login.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	raisedToasts,
	seedAuthConfig,
	t,
	WAIT_FOR_OPTIONS,
} from "../components/test-helpers"

const OAUTH_QUERY =
	"?client_id=client1&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb&sig=abc"
const OAUTH_CONTINUE_URL =
	"http://test.local/auth-realtime/api/auth/oauth2/authorize?client_id=client1&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcb&sig=abc"

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

// the query cache and the vue-sonner, useRoute and navigateTo module mocks
// are app-wide singletons every mount in the file shares
describe("<login>", { concurrent: false }, () => {
	// the page polls for the desktop redirect until it unmounts
	enableAutoUnmount(afterEach)

	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		useRouteMock.mockReset()
		arrangeRoute()
	})

	afterEach(disposeMockEndpoints)

	it("shows the default admin credentials above the email field while they still sign in", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["google", "email-password"],
			singleOrganization: true,
			defaultAdmin: {
				email: "admin@example.com",
				password: "oxynote-admin-1234",
			},
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.findAll("li").map((li) => li.text())).toEqual([
			"admin@example.com",
			"oxynote-admin-1234",
		])
		expect(wrapper.findAll("code").map((c) => c.element.textContent)).toEqual([
			"admin@example.com",
			"oxynote-admin-1234",
		])

		const form = wrapper.get("form").element
		const box = form.firstElementChild
		expect(box?.textContent).toContain("oxynote-admin-1234")
		expect(
			box?.nextElementSibling?.querySelector("input[type='email']"),
		).not.toBeNull()
	})

	it("shows no credentials once the default admin has changed the password", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["google", "email-password"],
			singleOrganization: true,
			defaultAdmin: { email: "admin@example.com", password: null },
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.text()).not.toContain("admin@example.com")
	})

	it("shows no credentials outside single-workspace mode", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["google", "email-password"],
			singleOrganization: false,
			defaultAdmin: {
				email: "admin@example.com",
				password: "oxynote-admin-1234",
			},
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.text()).not.toContain("admin@example.com")
	})

	it("shows the email/password form immediately when it is the only enabled method", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["email-password"],
		})

		const wrapper = await mountSuspended(Login)

		expect(wrapper.find("form").exists()).toBe(true)
		expect(wrapper.find('input[type="email"]').exists()).toBe(true)
		expect(wrapper.find('input[type="password"]').exists()).toBe(true)
	})

	it("opens the password reset form when email/password is the only enabled method", async ({
		expect,
	}) => {
		seedAuthConfig({
			methods: ["email-password"],
		})
		const wrapper = await mountSuspended(Login)

		await findButtonByText(
			wrapper,
			t("onboarding.login.forgot-password.placeholders.reset"),
		).trigger("click")

		expect(wrapper.find('input[type="password"]').exists()).toBe(false)
		expect(
			wrapper
				.find(
					`input[placeholder="${t("onboarding.login.password-reset-form.email-placeholder")}"]`,
				)
				.exists(),
		).toBe(true)
	})

	it.for([
		{ query: "?verified=true", key: "onboarding.login.email-verified" },
		{ query: "?reset=true", key: "onboarding.login.password-reset-success" },
	])(
		"confirms the finished step on arrival from $query",
		async ({ query, key }, { expect }) => {
			seedAuthConfig()
			arrangeRoute(query)

			await mountSuspended(Login)

			expect(raisedToasts()).toMatchObject([{ type: "success", title: t(key) }])
		},
	)

	it.for(["google", "github", "slack"] as const)(
		"signs in with %s and returns to the root",
		async (provider, { expect }) => {
			seedAuthConfig({ methods: [provider] })
			const calls = mockSocialSignIn()
			const wrapper = await mountSuspended(Login)

			await findButtonByText(
				wrapper,
				t(`onboarding.login.login-${provider}`),
			).trigger("click")
			await settleSignIn(calls)

			expect.soft(calls[0]?.body).toEqual({
				provider: provider,
				callbackURL: "http://test.local/",
				newUserCallbackURL: "http://test.local/welcome",
			})
			expect.soft(toast.custom).not.toHaveBeenCalled()
		},
	)

	it("sends a social sign-in back to the next page, hash included", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		arrangeRoute("?next=%2Fteam%2Fdoc", "#intro")
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Login)

		await findButtonByText(wrapper, t("onboarding.login.login-google")).trigger(
			"click",
		)
		await settleSignIn(calls)

		expect.soft(calls[0]?.query).toEqual({ next: "/team/doc" })
		expect.soft(calls[0]?.body).toEqual({
			provider: "google",
			callbackURL: "http://test.local/team/doc#intro",
			newUserCallbackURL: "http://test.local/team/doc#intro",
		})
	})

	it("sends a social sign-in on to the interrupted authorization", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		arrangeRoute(OAUTH_QUERY)
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Login)

		await findButtonByText(wrapper, t("onboarding.login.login-google")).trigger(
			"click",
		)
		await settleSignIn(calls)

		expect.soft(calls[0]?.body).toEqual({
			provider: "google",
			callbackURL: OAUTH_CONTINUE_URL,
			newUserCallbackURL: "http://test.local/welcome",
		})
	})

	it("leaves a repeated query parameter out of the continued authorization", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google"] })
		useRouteMock.mockReturnValue({
			query: {
				...Object.fromEntries(new URLSearchParams(OAUTH_QUERY).entries()),
				resource: ["a", "b"],
			},
			hash: "",
			fullPath: "/login",
		})
		const calls = mockSocialSignIn()
		const wrapper = await mountSuspended(Login)

		await findButtonByText(wrapper, t("onboarding.login.login-google")).trigger(
			"click",
		)
		await settleSignIn(calls)

		expect(calls[0]?.body).toMatchObject({ callbackURL: OAUTH_CONTINUE_URL })
	})

	it("reports a social sign-in the server refused", async ({ expect }) => {
		seedAuthConfig({ methods: ["google"] })
		mockAuthEndpoint("sign-in/social", () => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await mountSuspended(Login)

		await findButtonByText(wrapper, t("onboarding.login.login-google")).trigger(
			"click",
		)

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.login.errors.login-failed") },
			])
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.login.login-google"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it.for([
		{
			methods: ["google", "github", "slack", "email-password"],
			expected: "onboarding.login.login-google",
		},
		{
			methods: ["github", "slack", "email-password"],
			expected: "onboarding.login.login-github",
		},
	] as const)(
		"marks the first of $methods.length methods as primary",
		async ({ methods, expected }, { expect }) => {
			seedAuthConfig({ methods: [...methods] })

			const wrapper = await mountSuspended(Login)

			expect(primaryButtonTexts(wrapper)).toEqual([t(expected)])
		},
	)

	it("marks no method as primary with only two to pick from", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google", "email-password"] })

		const wrapper = await mountSuspended(Login)

		expect(primaryButtonTexts(wrapper)).toEqual([])
	})

	it("explains that no login method is configured", async ({ expect }) => {
		seedAuthConfig({ methods: [] })

		const wrapper = await mountSuspended(Login)

		expect(wrapper.text()).toContain(t("onboarding.login.no-methods"))
		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("claims no missing method while the auth config is still loading", async ({
		expect,
	}) => {
		const config = mockDeferredEndpoint(
			"GET",
			"http://test.local/auth-realtime/api/auth-config",
		)

		const wrapper = await mountSuspended(Login)
		await config.reached

		expect.soft(wrapper.text()).not.toContain(t("onboarding.login.no-methods"))
		expect.soft(wrapper.findAll("button")).toHaveLength(0)
	})

	it("goes back to the methods from the email/password form", async ({
		expect,
	}) => {
		seedAuthConfig({ methods: ["google", "email-password"] })
		const wrapper = await openEmailPasswordForm()

		await findButtonByText(
			wrapper,
			t("onboarding.login.email-password-form.back"),
		).trigger("click")

		expect(wrapper.find('input[type="email"]').exists()).toBe(false)
		expect(wrapper.text()).toContain(t("onboarding.login.login-google"))
	})

	it("lets better-auth follow its own redirect after an email/password sign-in", async ({
		expect,
	}) => {
		seedAuthConfig()
		const location = stubLocationHref()
		const calls = mockAuthEndpoint("sign-in/email", (call) => ({
			redirect: true,
			url: (call.body as { callbackURL: string }).callbackURL,
			token: "token-1",
			user: { id: "user-1", email: "ada@example.com" },
		}))
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)
		await settleSignIn(calls)

		expect.soft(calls[0]?.body).toEqual({
			email: "ada@example.com",
			password: "hunter2",
			callbackURL: "http://test.local/login?verified=true",
		})
		expect
			.soft(location)
			.toHaveBeenCalledExactlyOnceWith("http://test.local/login?verified=true")
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("carries the next page into the email/password callback", async ({
		expect,
	}) => {
		seedAuthConfig()
		arrangeRoute("?next=%2Fteam%2Fdoc", "#intro")
		const calls = mockAuthEndpoint("sign-in/email", () => ({
			redirect: false,
			token: "token-1",
			user: { id: "user-1", email: "ada@example.com" },
		}))
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)
		await settleSignIn(calls)

		expect(calls[0]?.body).toMatchObject({
			callbackURL:
				"http://test.local/login?verified=true&next=%2Fteam%2Fdoc%2523intro",
		})
	})

	it("continues the interrupted authorization after an email/password sign-in", async ({
		expect,
	}) => {
		seedAuthConfig()
		arrangeRoute(OAUTH_QUERY)
		const location = stubLocationHref()
		const calls = mockAuthEndpoint("sign-in/email", () => ({
			redirect: false,
			token: "token-1",
			user: { id: "user-1", email: "ada@example.com" },
		}))
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)

		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(location).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(calls).toHaveLength(1)
		expect.soft(location).toHaveBeenCalledWith(OAUTH_CONTINUE_URL)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("sends an unverified address to check its inbox", async ({ expect }) => {
		seedAuthConfig()
		mockAuthEndpoint("sign-in/email", (_call, event) => {
			setResponseStatus(event, 403)

			return { code: "EMAIL_NOT_VERIFIED", message: "Email not verified" }
		})
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)

		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(navigateToMock).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(navigateToMock).toHaveBeenCalledWith({
			path: "/verify-email",
			query: { new: "ada@example.com", sent: "true" },
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("reports wrong credentials", async ({ expect }) => {
		seedAuthConfig()
		mockAuthEndpoint("sign-in/email", (_call, event) => {
			setResponseStatus(event, 401)

			return {
				code: "INVALID_EMAIL_OR_PASSWORD",
				message: "Invalid email or password",
			}
		})
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)

		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "error",
				title: t("onboarding.login.errors.invalid-credentials"),
			},
		])
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("reports an email/password sign-in the server refused", async ({
		expect,
	}) => {
		seedAuthConfig()
		mockAuthEndpoint("sign-in/email", () => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await mountSuspended(Login)

		await submitCredentials(wrapper)

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.login.errors.login-failed") },
			])
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.login.email-password-form.continue"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it("asks for a reset link for the entered address", async ({ expect }) => {
		seedAuthConfig()
		const calls = mockAuthEndpoint("request-password-reset", () => ({
			status: true,
		}))
		const wrapper = await openResetForm()

		await submitResetRequest(wrapper, "ada@example.com")

		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("onboarding.login.password-reset-sent", {
					email: "ada@example.com",
				}),
			)
		}, WAIT_FOR_OPTIONS)
		expect.soft(calls).toHaveLength(1)
		expect.soft(calls[0]?.body).toEqual({
			email: "ada@example.com",
			redirectTo: "http://test.local/reset-password",
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("asks for no reset link for a malformed address", async ({ expect }) => {
		seedAuthConfig()
		const calls = mockAuthEndpoint("request-password-reset", () => ({
			status: true,
		}))
		const wrapper = await openResetForm()

		await submitResetRequest(wrapper, "not-an-address")

		// vee-validate renders the message on its own scheduler, with no
		// signal the page surfaces
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("onboarding.login.errors.email-invalid"),
			)
		}, WAIT_FOR_OPTIONS)
		expect.soft(calls).toHaveLength(0)
	})

	it("reports a reset request the server refused", async ({ expect }) => {
		seedAuthConfig()
		mockAuthEndpoint("request-password-reset", () => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await openResetForm()

		await submitResetRequest(wrapper, "ada@example.com")

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "error",
				title: t("onboarding.login.errors.password-reset-failed"),
			},
		])
		expect.soft(wrapper.find('input[type="email"]').exists()).toBe(true)
	})

	it("goes back to the email/password form from the reset request", async ({
		expect,
	}) => {
		seedAuthConfig()
		const wrapper = await openResetForm()

		await findButtonByText(
			wrapper,
			t("onboarding.login.password-reset-form.back"),
		).trigger("click")

		expect(wrapper.find('input[type="password"]').exists()).toBe(true)
	})

	it("goes back to the email/password form once the reset link is sent", async ({
		expect,
	}) => {
		seedAuthConfig()
		mockAuthEndpoint("request-password-reset", () => ({ status: true }))
		const wrapper = await openResetForm()
		await submitResetRequest(wrapper, "ada@example.com")
		// vee-validate's validation and better-auth's request run on their
		// own schedulers, and the page signals the end of neither
		await vi.waitFor(() => {
			expect(wrapper.find("form").exists()).toBe(false)
		}, WAIT_FOR_OPTIONS)

		await findButtonByText(
			wrapper,
			t("onboarding.login.password-reset-form.back"),
		).trigger("click")

		expect(wrapper.find('input[type="password"]').exists()).toBe(true)
	})

	it("offers no password reset when the server sends no email", async ({
		expect,
	}) => {
		seedAuthConfig({ emailEnabled: false })

		const wrapper = await mountSuspended(Login)

		expect(wrapper.text()).not.toContain(
			t("onboarding.login.forgot-password.placeholders.reset"),
		)
	})

	it("points a visitor without an account to a workspace member in single-workspace mode", async ({
		expect,
	}) => {
		seedAuthConfig({ singleOrganization: true })

		const wrapper = await mountSuspended(Login)

		expect(wrapper.text()).toContain(
			t("onboarding.login.no-account.invitation-only"),
		)
		expect(wrapper.text()).not.toContain(
			t("onboarding.login.no-account.placeholders.sign-up"),
		)
	})

	it("offers signup to an invitee in single-workspace mode", async ({
		expect,
	}) => {
		seedAuthConfig({ singleOrganization: true })
		arrangeRoute("?next=%2Faccept-invite%3Fid%3Dinvite-1")

		const wrapper = await mountSuspended(Login)

		expect(wrapper.text()).not.toContain(
			t("onboarding.login.no-account.invitation-only"),
		)
		expect(signupLink(wrapper).attributes("href")).toBe(
			"/signup?next=/accept-invite?id=invite-1",
		)
	})
})

function arrangeRoute(query = "", hash = "") {
	useRouteMock.mockReturnValue({
		query: Object.fromEntries(new URLSearchParams(query).entries()),
		hash: hash,
		fullPath: `/login${query}${hash}`,
	})
}

async function openEmailPasswordForm() {
	const wrapper = await mountSuspended(Login)

	await findButtonByText(
		wrapper,
		t("onboarding.login.login-email-password"),
	).trigger("click")

	return wrapper
}

async function openResetForm() {
	const wrapper = await mountSuspended(Login)

	await findButtonByText(
		wrapper,
		t("onboarding.login.forgot-password.placeholders.reset"),
	).trigger("click")

	return wrapper
}

// redirect: false keeps better-auth from following the provider url
function mockSocialSignIn() {
	return mockAuthEndpoint("sign-in/social", () => ({
		url: "https://accounts.example.com/authorize",
		redirect: false,
	}))
}

// vee-validate's validation and better-auth's request run on their own
// schedulers, and the page signals the end of neither
function settleSignIn(calls: unknown[]) {
	return vi.waitFor(() => {
		if (calls.length !== 1) {
			throw new Error(`expected one sign-in call, got ${calls.length}`)
		}
	}, WAIT_FOR_OPTIONS)
}

// the email/password submit handler prevents the default itself, and
// happy-dom raises no submit event from a click on a submit button
async function submitCredentials(wrapper: VueWrapper) {
	await wrapper.get('input[type="email"]').setValue("ada@example.com")
	await wrapper.get('input[type="password"]').setValue("hunter2")
	await wrapper.get("form").trigger("submit")
}

async function submitResetRequest(wrapper: VueWrapper, email: string) {
	await wrapper.get('input[type="email"]').setValue(email)
	await wrapper.get("form").trigger("submit")
}

// a full navigation would leave the test page, so the href setter is
// replaced with a recorder
function stubLocationHref() {
	return vi
		.spyOn(window.location, "href", "set")
		.mockImplementation(() => undefined)
}

function primaryButtonTexts(wrapper: VueWrapper) {
	return wrapper
		.findAll("button")
		.filter((b) => b.classes().includes("bg-primary"))
		.map((b) => b.text())
}

function signupLink(wrapper: VueWrapper) {
	const link = wrapper
		.findAll("a")
		.find((a) =>
			a.text().includes(t("onboarding.login.no-account.placeholders.sign-up")),
		)
	if (!link) {
		throw new Error("no signup link")
	}

	return link
}
