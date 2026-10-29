import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import AcceptInvite from "./accept-invite.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	raisedToasts,
	seedAuthSession,
	settleActionSubmit,
	settleMutations,
	t,
} from "../components/test-helpers"

const INVITE_QUERY = {
	id: "invitation-1",
	email: "invitee@example.com",
	inviter: "Ada",
	orgName: "Acme",
	orgId: "org-1",
}
const INVITE_PATH = "/accept-invite?id=invitation-1&email=invitee%40example.com"

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

// the vue-sonner, useRoute and navigateTo module mocks and the query cache
// are app-wide singletons every mount in the file shares, and accepting is
// driven by the global fake timers
describe("<accept-invite>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useRouteMock.mockReset()
		navigateToMock.mockReset()
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	it.for([
		{ name: "sends a visitor without an invitation id home", missing: "id" },
		{ name: "sends a visitor without an email home", missing: "email" },
		{ name: "sends a visitor without an inviter home", missing: "inviter" },
		{
			name: "sends a visitor without a workspace name home",
			missing: "orgName",
		},
		{ name: "sends a visitor without a workspace id home", missing: "orgId" },
	])("$name", async ({ missing }, { expect }) => {
		const query: Record<string, string> = { ...INVITE_QUERY }
		delete query[missing]
		const to = { query: query } as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/")
	})

	it("sends a visitor with a repeated invitation id home", async ({
		expect,
	}) => {
		const to = {
			query: { ...INVITE_QUERY, id: ["invitation-1", "invitation-2"] },
		} as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/")
	})

	it("lets a visitor with a complete invitation through", async ({
		expect,
	}) => {
		const to = { query: INVITE_QUERY } as unknown as RouteLocationNormalized

		const result = await pageMiddleware()(to, to)

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it("names the inviter, the workspace and the invited address", async ({
		expect,
	}) => {
		const wrapper = await mountInvite()

		expect(wrapper.text()).toContain(
			t("onboarding.accept-invite.title", { name: "Ada", org: "Acme" }),
		)
		expect(wrapper.text()).toContain(
			t("onboarding.accept-invite.description", {
				email: "invitee@example.com",
			}),
		)
	})

	describe("when signed out", { concurrent: false }, () => {
		it("offers a signup and no way to log out", async ({ expect }) => {
			const wrapper = await mountInvite()

			expect(wrapper.findAll("button").map((b) => b.text())).toEqual([
				t("onboarding.accept-invite.signup-button"),
			])
			expect(wrapper.text()).not.toContain(
				t("onboarding.accept-invite.login-info.logged-in-as"),
			)
		})

		it("sends the visitor to signup with a way back to the invitation", async ({
			expect,
		}) => {
			const accepts = mockAuthEndpoint(
				"organization/accept-invitation",
				() => ({
					invitation: {},
				}),
			)
			const wrapper = await mountInvite()

			await findButtonByText(
				wrapper,
				t("onboarding.accept-invite.signup-button"),
			).trigger("click")

			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({
				name: "signup",
				query: { next: encodeURIComponent(INVITE_PATH) },
			})
			expect(accepts).toHaveLength(0)
		})
	})

	describe("when signed in", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthSession({ email: "invitee@example.com" })
		})

		it("shows who is signed in next to a way to log out", async ({
			expect,
		}) => {
			const wrapper = await mountInvite()

			expect(wrapper.text()).toContain(
				t("onboarding.accept-invite.login-info.logged-in-as"),
			)
			expect(wrapper.text()).toContain("invitee@example.com")
			expect(wrapper.findAll("button").map((b) => b.text())).toEqual([
				t("onboarding.accept-invite.login-info.logout"),
				t("onboarding.accept-invite.accept-button"),
			])
		})

		it("joins the workspace and opens the app", async ({ expect }) => {
			const { accepts, activations, signOuts } = mockAcceptance()
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(accepts).toHaveLength(1)
			expect(accepts[0]?.body).toEqual({ invitationId: "invitation-1" })
			expect(activations).toHaveLength(1)
			expect(activations[0]?.body).toEqual({ organizationId: "org-1" })
			expect.soft(signOuts).toHaveLength(0)
			expect.soft(toast.custom).not.toHaveBeenCalled()
			expect.soft(navigateToMock).toHaveBeenCalledExactlyOnceWith("/")
		})

		it("spins and disables the accept button while accepting", async ({
			expect,
		}) => {
			mockAcceptance()
			const wrapper = await mountInvite()
			vi.useFakeTimers()

			await findButtonByText(
				wrapper,
				t("onboarding.accept-invite.accept-button"),
			).trigger("click")

			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.accept-button"),
				).attributes("disabled"),
			).toBeDefined()
			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.login-info.logout"),
				).classes(),
			).toContain("pointer-events-none")
		})

		it("warns and leaves for onboarding when the invitation is for another address", async ({
			expect,
		}) => {
			seedAuthSession({ email: "someone-else@example.com" })
			const { accepts } = mockAcceptance()
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(raisedToasts()).toMatchObject([
				{
					type: "warning",
					title: t("onboarding.accept-invite.warn.email-mismatch.title"),
					description: t(
						"onboarding.accept-invite.warn.email-mismatch.description",
					),
				},
			])
			expect(accepts).toHaveLength(0)
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({
				name: "onboarding",
			})
		})

		it("explains a full workspace and lets the user try again", async ({
			expect,
		}) => {
			const { activations } = mockAcceptance({
				accept: (_call, event) => {
					setResponseStatus(event, 403)

					return {
						code: "ORGANIZATION_MEMBERSHIP_LIMIT_REACHED",
						message: "Organization membership limit reached",
					}
				},
			})
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(raisedToasts()).toMatchObject([
				{
					type: "info",
					title: t(
						"onboarding.accept-invite.errors.member-limit-reached.title",
					),
				},
			])
			expect(activations).toHaveLength(0)
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.accept-button"),
				).attributes("disabled"),
			).toBeUndefined()
		})

		it("reports a refused acceptance and lets the user try again", async ({
			expect,
		}) => {
			const { activations } = mockAcceptance({
				accept: () => {
					throw createError({ statusCode: 500 })
				},
			})
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					title: t("onboarding.accept-invite.errors.accept-failed"),
				},
			])
			expect(activations).toHaveLength(0)
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.accept-button"),
				).attributes("disabled"),
			).toBeUndefined()
		})

		it("signs the user out to pick the workspace up on the next login when it cannot be activated", async ({
			expect,
		}) => {
			const { accepts, signOuts } = mockAcceptance({
				activate: () => {
					throw createError({ statusCode: 500 })
				},
			})
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(accepts).toHaveLength(1)
			expect(signOuts).toHaveLength(1)
			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					title: t(
						"onboarding.accept-invite.success.accepted-invite-log-out.title",
					),
					description: t(
						"onboarding.accept-invite.success.accepted-invite-log-out.description",
					),
				},
			])
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({ name: "login" })
		})

		it("stays quiet about a failed sign-out after an acceptance it could not activate", async ({
			expect,
		}) => {
			const { signOuts } = mockAcceptance({
				activate: () => {
					throw createError({ statusCode: 500 })
				},
				signOut: () => {
					throw createError({ statusCode: 500 })
				},
			})
			const wrapper = await mountInvite()

			await accept(wrapper)

			expect(signOuts).toHaveLength(1)
			expect(raisedToasts().map((toast) => toast.type)).toEqual(["success"])
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.login-info.logout"),
				).attributes("disabled"),
			).toBeUndefined()
		})

		it("logs the user out to the login page", async ({ expect }) => {
			const { accepts, signOuts } = mockAcceptance()
			const wrapper = await mountInvite()

			await findButtonByText(
				wrapper,
				t("onboarding.accept-invite.login-info.logout"),
			).trigger("click")
			await settleMutations()

			expect(signOuts).toHaveLength(1)
			expect(accepts).toHaveLength(0)
			expect(toast.custom).not.toHaveBeenCalled()
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({ name: "login" })
		})

		it("reports a failed logout and lets the user retry", async ({
			expect,
		}) => {
			const { signOuts } = mockAcceptance({
				signOut: () => {
					throw createError({ statusCode: 500 })
				},
			})
			const wrapper = await mountInvite()

			await findButtonByText(
				wrapper,
				t("onboarding.accept-invite.login-info.logout"),
			).trigger("click")
			await settleMutations()

			expect(signOuts).toHaveLength(1)
			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					title: t("onboarding.accept-invite.errors.signout-failed"),
				},
			])
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(
				findButtonByText(
					wrapper,
					t("onboarding.accept-invite.login-info.logout"),
				).attributes("disabled"),
			).toBeUndefined()
		})
	})
})

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "accept-invite")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the accept-invite page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

async function mountInvite() {
	useRouteMock.mockReturnValue({ query: INVITE_QUERY, fullPath: INVITE_PATH })

	return await mountSuspended(AcceptInvite)
}

// every better-auth call an acceptance can make. Activating the workspace
// refetches the session and the organization, and the organization has to
// come back as the invited one for the session to count as updated.
function mockAcceptance(
	respond: {
		accept?: Parameters<typeof mockAuthEndpoint>[1]
		activate?: Parameters<typeof mockAuthEndpoint>[1]
		signOut?: Parameters<typeof mockAuthEndpoint>[1]
	} = {},
) {
	mockAuthEndpoint(
		"get-session",
		() => ({
			session: { id: "session-1" },
			user: { email: "invitee@example.com" },
		}),
		"GET",
	)
	mockAuthEndpoint(
		"organization/get-full-organization",
		() => ({ id: "org-1" }),
		"GET",
	)

	return {
		accepts: mockAuthEndpoint(
			"organization/accept-invitation",
			respond.accept ?? (() => ({ invitation: {}, member: {} })),
		),
		activations: mockAuthEndpoint(
			"organization/set-active",
			respond.activate ?? (() => ({ id: "org-1" })),
		),
		signOuts: mockAuthEndpoint(
			"sign-out",
			respond.signOut ?? (() => ({ success: true })),
		),
	}
}

// the accept button holds its request for delay(300) so the spinner shows.
// Fake timers go in after the mount, since installing them earlier freezes
// the nuxt app's own async setup.
async function accept(wrapper: Awaited<ReturnType<typeof mountInvite>>) {
	vi.useFakeTimers()
	await findButtonByText(
		wrapper,
		t("onboarding.accept-invite.accept-button"),
	).trigger("click")
	await settleActionSubmit()
}
