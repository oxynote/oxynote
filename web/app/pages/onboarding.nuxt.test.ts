import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { setResponseStatus, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	type RecordedCall,
} from "~/composables/api/test-helpers"
import Onboarding from "./onboarding.vue"
import {
	at,
	findButtonByText,
	mockAuthEndpoint,
	raisedToasts,
	seedAuthConfig,
	seedAuthSession,
	settleMutations,
	t,
} from "../components/test-helpers"

const USER = { id: "user-1", email: "ada@example.com" }
const ORGANIZATION = { id: "org-1", name: "Acme Corp", slug: "acme-corp" }
const FIRST_DOCUMENT = {
	id: makeXid("doc1"),
	documentName: "Getting Started",
	icon: "",
	protected: false,
}

type Respond = (call: RecordedCall, event: H3Event) => unknown

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

// the query cache, the vue-sonner and navigateTo module mocks are app-wide
// singletons every mount in the file shares, and the submit flow is driven
// by the global fake timers
describe("<onboarding>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		seedAuthSession(USER)
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	it("shows the address the visitor is signed in with", async ({ expect }) => {
		const wrapper = await mountOnboarding()

		expect(wrapper.text()).toContain(
			t("onboarding.welcome.login-info.logged-in-as"),
		)
		expect(wrapper.text()).toContain("ada@example.com")
	})

	it("leaves the signed-in address blank without a session", async ({
		expect,
	}) => {
		seedAuthSession(null)

		const wrapper = await mountOnboarding()

		expect(wrapper.text()).not.toContain("ada@example.com")
	})

	it("signs the visitor out and sends them to the login page", async ({
		expect,
	}) => {
		const signOuts = mockSignOut(() => ({ success: true }))
		const wrapper = await mountOnboarding()

		await logOut(wrapper)

		expect.soft(signOuts).toHaveLength(1)
		expect.soft(navigateToMock).toHaveBeenCalledTimes(1)
		expect.soft(navigateToMock).toHaveBeenCalledWith({ name: "login" })
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("warns and stays put when signing out fails", async ({ expect }) => {
		const signOuts = mockSignOut(() => {
			throw createError({ statusCode: 500 })
		})
		const wrapper = await mountOnboarding()

		await logOut(wrapper)

		expect.soft(signOuts).toHaveLength(1)
		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.welcome.errors.signout-failed") },
			])
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.welcome.login-info.logout"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it.for([
		{
			name: "rejects a workspace name with symbols in it",
			input: { workspaceName: "Acme Corp!", workspaceSlug: "acme-corp" },
			expected: "onboarding.welcome.errors.workspace-name-regex",
		},
		{
			name: "rejects a workspace url with spaces in it",
			input: { workspaceName: "Acme Corp", workspaceSlug: "acme corp" },
			expected: "onboarding.welcome.errors.workspace-url-regex",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const creation = mockWorkspaceCreation()
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, input.workspaceName, input.workspaceSlug)

		expect.soft(wrapper.text()).toContain(t(expected))
		expect.soft(creation.slugChecks).toHaveLength(0)
		expect.soft(creation.creates).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("shows a taken workspace url next to the field", async ({ expect }) => {
		const creation = mockWorkspaceCreation({
			checkSlug: (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "SLUG_IS_TAKEN", message: "taken" }
			},
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect
			.soft(wrapper.text())
			.toContain(t("onboarding.welcome.errors.workspace-url-taken"))
		expect.soft(creation.slugChecks).toHaveLength(1)
		expect.soft(creation.creates).toHaveLength(0)
		expect.soft(toast.custom).not.toHaveBeenCalled()
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("shows a workspace url the server refuses for any other reason as invalid", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation({
			checkSlug: () => {
				throw createError({ statusCode: 500 })
			},
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect
			.soft(wrapper.text())
			.toContain(t("onboarding.welcome.errors.workspace-url-invalid"))
		expect.soft(creation.slugChecks).toHaveLength(1)
		expect.soft(creation.creates).toHaveLength(0)
		expect.soft(toast.custom).not.toHaveBeenCalled()
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("explains the workspace limit when the server allows no new workspace", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation({
			create: (_call, event) => {
				setResponseStatus(event, 403)

				return {
					code: "YOU_ARE_NOT_ALLOWED_TO_CREATE_A_NEW_ORGANIZATION",
					message: "not allowed",
				}
			},
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "info",
				title: t("onboarding.signup.errors.organization-limit-reached.title"),
			},
		])
		expect.soft(creation.creates).toHaveLength(1)
		expect.soft(creation.activations).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("warns when the workspace cannot be created", async ({ expect }) => {
		const creation = mockWorkspaceCreation({
			create: () => {
				throw createError({ statusCode: 500 })
			},
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect
			.soft(raisedToasts())
			.toMatchObject([
				{ type: "error", title: t("onboarding.welcome.errors.create-failed") },
			])
		expect.soft(creation.creates).toHaveLength(1)
		expect.soft(creation.activations).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.welcome.create-workspace"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})

	it("warns when the new workspace cannot be made the active one", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation({
			setActive: () => {
				throw createError({ statusCode: 500 })
			},
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "error",
				title: t("onboarding.welcome.errors.activation-failed"),
			},
		])
		expect.soft(creation.activations).toHaveLength(1)
		expect.soft(creation.trees).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("creates and activates the workspace under the chosen name and url", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation()
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect.soft(creation.slugChecks).toHaveLength(1)
		expect.soft(creation.slugChecks[0]?.body).toEqual({ slug: "acme-corp" })
		expect.soft(creation.creates).toHaveLength(1)
		expect.soft(creation.creates[0]?.body).toMatchObject({
			name: "Acme Corp",
			slug: "acme-corp",
		})
		expect.soft(creation.activations).toHaveLength(1)
		expect.soft(creation.activations[0]?.body).toMatchObject({
			organizationId: "org-1",
			organizationSlug: "acme-corp",
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("opens the workspace's first page in place of onboarding", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation()
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		// the tree query fetches once when the page creates it, then once per
		// refetch in the retry loop
		expect.soft(creation.trees).toHaveLength(2)
		expect.soft(navigateToMock).toHaveBeenCalledTimes(1)
		expect
			.soft(navigateToMock)
			.toHaveBeenCalledWith(`/Acme-Corp/Getting-Started-${FIRST_DOCUMENT.id}`, {
				replace: true,
			})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("waits for the first page when the tree is still empty", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation({
			tree: (call, calls) => (calls.indexOf(call) < 3 ? [] : [FIRST_DOCUMENT]),
		})
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp")

		expect.soft(creation.trees).toHaveLength(4)
		expect.soft(navigateToMock).toHaveBeenCalledTimes(1)
		expect
			.soft(navigateToMock)
			.toHaveBeenCalledWith(`/Acme-Corp/Getting-Started-${FIRST_DOCUMENT.id}`, {
				replace: true,
			})
		expect.soft(toast.custom).not.toHaveBeenCalled()
	})

	it("gives up and warns when the first page never appears", async ({
		expect,
	}) => {
		const creation = mockWorkspaceCreation({ tree: () => [] })
		const wrapper = await mountOnboarding()

		await submitWorkspace(wrapper, "Acme Corp", "acme-corp", 60)

		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "error",
				title: t("onboarding.welcome.errors.first-document-load-failed"),
			},
		])
		expect.soft(creation.trees).toHaveLength(51)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})
})

// fake timers go in once the page is up. Installed before the mount they
// leave the nuxt app's own async setup frozen.
async function mountOnboarding() {
	const wrapper = await mountSuspended(Onboarding)
	vi.useFakeTimers()

	return wrapper
}

// each round lets the pending requests answer and then runs out one
// delay(500) between tree fetches
async function submitWorkspace(
	wrapper: VueWrapper,
	workspaceName: string,
	workspaceSlug: string,
	rounds = 10,
) {
	const inputs = wrapper.findAll("input[type='text']")
	await at(inputs, 0).setValue(workspaceName)
	await at(inputs, 1).setValue(workspaceSlug)
	await wrapper.get("form").trigger("submit")

	for (let round = 0; round < rounds; round++) {
		await settleMutations()
		await vi.advanceTimersByTimeAsync(500)
	}

	await settleMutations()
}

async function logOut(wrapper: VueWrapper) {
	await findButtonByText(
		wrapper,
		t("onboarding.welcome.login-info.logout"),
	).trigger("click")
	await settleMutations()
}

// signing out refetches the auth config, which has to be answered too
function mockSignOut(respond: () => unknown) {
	seedAuthConfig()
	mockEndpoint(
		"GET",
		"http://test.local/auth-realtime/api/auth-config",
		() => ({
			methods: ["email-password"],
			emailEnabled: true,
			singleOrganization: false,
			maxOrganizationMembers: 5,
			defaultAdmin: null,
		}),
	)

	return mockAuthEndpoint("sign-out", respond)
}

// answers every request the submit makes. Each step succeeds unless the
// test passes its own responder.
function mockWorkspaceCreation(
	overrides: {
		checkSlug?: Respond
		create?: Respond
		setActive?: Respond
		tree?: (call: RecordedCall, calls: RecordedCall[]) => unknown
	} = {},
) {
	const slugChecks = mockAuthEndpoint(
		"organization/check-slug",
		overrides.checkSlug ?? (() => ({ status: true })),
	)
	const creates = mockAuthEndpoint(
		"organization/create",
		overrides.create ?? (() => ORGANIZATION),
	)
	const activations = mockAuthEndpoint(
		"organization/set-active",
		overrides.setActive ?? (() => ORGANIZATION),
	)

	mockAuthEndpoint(
		"get-session",
		() => ({ session: { id: "session-1" }, user: USER }),
		"GET",
	)
	mockAuthEndpoint(
		"organization/get-full-organization",
		() => ORGANIZATION,
		"GET",
	)

	const respondTree = overrides.tree ?? (() => [FIRST_DOCUMENT])
	const trees: RecordedCall[] = mockEndpoint(
		"GET",
		"/api/documents/tree",
		(call) => respondTree(call, trees),
	)

	return { slugChecks, creates, activations, trees }
}
