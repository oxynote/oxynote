import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import McpSection from "./McpSection.vue"
import { findButtonByText, settleMutations } from "../test-helpers"

// $authRealtimeAPIClient carries an absolute base url, so its requests
// match the absolute registration — not the bare path a better-auth call
// would be matched on.
const AUTH_REALTIME_BASE = "http://test.local/auth-realtime/api/auth"

function makeConsent(overrides: Partial<OAuthConsent> = {}): OAuthConsent {
	return {
		id: "consent1",
		clientId: "client1",
		scopes: ["documents:read", "documents:write"],
		createdAt: "2026-01-02T00:00:00Z",
		...overrides,
	}
}

function seedConsents(consents: OAuthConsent[]) {
	seedQueryData(["mcp", "consents"], consents)
}

// the client name lookup fires from a watcher on every mount, so it is
// registered before the component goes up
function mockClientName(clientName: string | undefined) {
	return mockEndpoint(
		"GET",
		`${AUTH_REALTIME_BASE}/oauth2/public-client`,
		() => ({
			client_id: "client1",
			client_name: clientName,
		}),
	)
}

// the query cache is app-wide
describe("<McpSection>", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	it("shows the server url the client has to be pointed at", async ({
		expect,
	}) => {
		seedConsents([])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain("Server URL")
		expect(wrapper.find("code").text()).toContain("/api/mcp")
	})

	it("says nothing is authorized yet when the list is empty", async ({
		expect,
	}) => {
		seedConsents([])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain("Authorized clients (0)")
		expect(wrapper.text()).toContain("No clients authorized yet")
		expect(wrapper.find("table").exists()).toBe(false)
	})

	it("counts the authorized clients in the heading", async ({ expect }) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent(), makeConsent({ id: "consent2" })])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain("Authorized clients (2)")
		expect(wrapper.findAll("tbody tr")).toHaveLength(2)
	})

	it("names the client and the scopes it was granted", async ({ expect }) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent()])

		const wrapper = await mountSuspended(McpSection)
		await settleMutations()

		expect(wrapper.text()).toContain("Claude Code")
		expect(wrapper.text()).toContain("Read, Write")
	})

	it("falls back to a placeholder when the client name cannot be read", async ({
		expect,
	}) => {
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/public-client`, () => {
			throw new Error("gone")
		})
		seedConsents([makeConsent()])

		const wrapper = await mountSuspended(McpSection)
		await settleMutations()

		expect(wrapper.text()).toContain("Unknown client")
	})

	it("shows an unrecognised scope verbatim rather than dropping it", async ({
		expect,
	}) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent({ scopes: ["documents:read", "wibble"] })])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain("Read, wibble")
	})

	it("revokes the client whose row was acted on", async ({ expect }) => {
		mockClientName("Claude Code")

		const revocations = mockEndpoint(
			"POST",
			`${AUTH_REALTIME_BASE}/oauth2/delete-consent`,
			() => ({}),
		)

		seedConsents([makeConsent()])
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/get-consents`, () => [])

		const wrapper = await mountSuspended(McpSection)
		await findButtonByText(wrapper, "Revoke").trigger("click")
		await settleMutations()

		expect(revocations).toHaveLength(1)
		expect(revocations[0]?.body).toEqual({ id: "consent1" })
	})
})
