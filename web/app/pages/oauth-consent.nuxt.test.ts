import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import OAuthConsent from "./oauth-consent.vue"
import {
	findButtonByText,
	settleMutations,
	t,
} from "../components/test-helpers"

// $authRealtimeAPIClient carries an absolute base url, so its requests
// match the absolute registration rather than a bare path
const AUTH_REALTIME_BASE = "http://test.local/auth-realtime/api/auth"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock } = vi.hoisted(() => ({ useRouteMock: vi.fn() }))

mockNuxtImport("useRoute", () => useRouteMock)

function mockClientName(clientName: string) {
	return mockEndpoint(
		"GET",
		`${AUTH_REALTIME_BASE}/oauth2/public-client`,
		() => ({ client_id: "client1", client_name: clientName }),
	)
}

function mockDecision(respond: () => unknown) {
	return mockEndpoint("POST", `${AUTH_REALTIME_BASE}/oauth2/consent`, respond)
}

// the page reads its client and scopes off the query string
async function mountConsent(
	query = "?client_id=client1&scope=documents%3Aread",
) {
	useRouteMock.mockReturnValue({
		query: Object.fromEntries(new URLSearchParams(query).entries()),
	})

	const wrapper = await mountSuspended(OAuthConsent)
	await settleMutations()

	return wrapper
}

// the vue-sonner and useRoute module mocks are app-wide singletons every
// mount in the file shares
describe("<oauth-consent>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useRouteMock.mockReset()
	})

	afterEach(disposeMockEndpoints)

	it("names the client asking for access", async ({ expect }) => {
		mockClientName("Claude Code")

		const wrapper = await mountConsent()

		expect(wrapper.text()).toContain("Claude Code")
	})

	it("falls back to the client id when the name cannot be read", async ({
		expect,
	}) => {
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/public-client`, () => {
			throw createError({ statusCode: 500 })
		})

		const wrapper = await mountConsent()

		expect(wrapper.text()).toContain("client1")
	})

	it("lists the scopes the client is asking for", async ({ expect }) => {
		mockClientName("Claude Code")

		const wrapper = await mountConsent(
			"?client_id=client1&scope=documents%3Aread%20documents%3Awrite",
		)

		expect(wrapper.text()).toContain(t("oauth.consent.scopes-title"))
		expect(wrapper.text()).toContain(t("oauth.consent.scopes.documents-read"))
		expect(wrapper.text()).toContain(t("oauth.consent.scopes.documents-write"))
	})

	it("names the data-sources scope", async ({ expect }) => {
		const wrapper = await mountConsent(
			"?client_id=client1&scope=data-sources%3Aread",
		)

		expect(wrapper.text()).toContain(
			t("oauth.consent.scopes.data-sources-read"),
		)
	})

	it("shows an unrecognised scope verbatim rather than dropping it", async ({
		expect,
	}) => {
		mockClientName("Claude Code")

		const wrapper = await mountConsent("?client_id=client1&scope=wibble")

		expect(wrapper.text()).toContain("wibble")
	})

	it("offers no scope list when the request asks for none", async ({
		expect,
	}) => {
		mockClientName("Claude Code")

		const wrapper = await mountConsent("?client_id=client1")

		expect(wrapper.text()).not.toContain(t("oauth.consent.scopes-title"))
	})

	it("sends the user on to the redirect the server answers with", async ({
		expect,
	}) => {
		mockClientName("Claude Code")

		const decisions = mockDecision(() => ({
			url: "https://client.test/callback?code=abc",
		}))
		const wrapper = await mountConsent()

		await findButtonByText(wrapper, t("oauth.consent.approve")).trigger("click")
		await settleMutations()

		expect(decisions).toHaveLength(1)
		expect(decisions[0]?.body).toMatchObject({ accept: true })
	})

	it("carries a denial to the server too", async ({ expect }) => {
		mockClientName("Claude Code")

		const decisions = mockDecision(() => ({
			url: "https://client.test/callback?error=access_denied",
		}))
		const wrapper = await mountConsent()

		await findButtonByText(wrapper, t("oauth.consent.deny")).trigger("click")
		await settleMutations()

		expect(decisions).toHaveLength(1)
		expect(decisions[0]?.body).toMatchObject({ accept: false })
	})

	it("reports a decision the server answers without a redirect", async ({
		expect,
	}) => {
		mockClientName("Claude Code")
		mockDecision(() => ({}))

		const wrapper = await mountConsent()

		await findButtonByText(wrapper, t("oauth.consent.approve")).trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)

		// the buttons come back so the user can try the other answer
		expect(
			findButtonByText(wrapper, t("oauth.consent.approve")).attributes(
				"disabled",
			),
		).toBeUndefined()
	})

	it("reports a decision the server refused", async ({ expect }) => {
		mockClientName("Claude Code")
		mockDecision(() => {
			throw createError({ statusCode: 500 })
		})

		const wrapper = await mountConsent()

		await findButtonByText(wrapper, t("oauth.consent.approve")).trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(
			findButtonByText(wrapper, t("oauth.consent.deny")).attributes("disabled"),
		).toBeUndefined()
	})
})
