import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import McpSection from "./McpSection.vue"
import { findButtonByText, settleMutations, t } from "../test-helpers"

// $authRealtimeAPIClient carries an absolute base url, so its requests
// match the absolute registration — not the bare path a better-auth call
// would be matched on.
const AUTH_REALTIME_BASE = "http://test.local/auth-realtime/api/auth"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

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

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares
describe("<McpSection>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
	})

	afterEach(disposeMockEndpoints)

	it("shows the server url the client has to be pointed at", async ({
		expect,
	}) => {
		seedConsents([])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain(t("settings.mcp.endpoint-label"))
		expect(wrapper.find("code").text()).toContain("/api/mcp")
	})

	it("says nothing is authorized yet when the list is empty", async ({
		expect,
	}) => {
		seedConsents([])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain(
			t("settings.mcp.clients-label", { count: 0 }),
		)
		expect(wrapper.text()).toContain(t("settings.mcp.no-clients"))
		expect(wrapper.find("table").exists()).toBe(false)
	})

	it("counts the authorized clients in the heading", async ({ expect }) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent(), makeConsent({ id: "consent2" })])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain(
			t("settings.mcp.clients-label", { count: 2 }),
		)
		expect(wrapper.findAll("tbody tr")).toHaveLength(2)
	})

	it("names the client and the scopes it was granted", async ({ expect }) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent()])

		const wrapper = await mountSuspended(McpSection)
		await settleMutations()

		expect(wrapper.text()).toContain("Claude Code")
		expect(wrapper.text()).toContain(
			`${t("settings.mcp.scopes.documents-read")}, ${t("settings.mcp.scopes.documents-write")}`,
		)
	})

	it("falls back to a placeholder when the client name cannot be read", async ({
		expect,
	}) => {
		// an h3 error fails the request without dumping a stack to stderr
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/public-client`, () => {
			throw createError({ statusCode: 500 })
		})
		seedConsents([makeConsent()])

		const wrapper = await mountSuspended(McpSection)
		await settleMutations()

		expect(wrapper.text()).toContain(t("settings.mcp.unknown-client"))
	})

	it("falls back to a placeholder when the client has no name", async ({
		expect,
	}) => {
		// the lookup succeeds but the client registered without a name, so
		// there is nothing to show but the fallback
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/public-client`, () => ({
			client_id: "client1",
		}))
		seedConsents([makeConsent()])

		const wrapper = await mountSuspended(McpSection)
		await settleMutations()

		expect(wrapper.text()).toContain(t("settings.mcp.unknown-client"))
	})

	it("shows an unrecognised scope verbatim rather than dropping it", async ({
		expect,
	}) => {
		mockClientName("Claude Code")
		seedConsents([makeConsent({ scopes: ["documents:read", "wibble"] })])

		const wrapper = await mountSuspended(McpSection)

		expect(wrapper.text()).toContain(
			`${t("settings.mcp.scopes.documents-read")}, wibble`,
		)
	})

	it("puts the server url on the clipboard", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)

		seedConsents([])

		const wrapper = await mountSuspended(McpSection)
		await findButtonByText(
			wrapper,
			t("settings.mcp.copy-button-screen-reader-hint"),
		).trigger("click")
		await settleMutations()

		expect(writeText).toHaveBeenCalledTimes(1)
		expect(writeText).toHaveBeenCalledWith(expect.stringContaining("/api/mcp"))
		expect(toast.custom).toHaveBeenCalledTimes(1)
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
		await findButtonByText(wrapper, t("settings.mcp.revoke")).trigger("click")
		await settleMutations()

		expect(revocations).toHaveLength(1)
		expect(revocations[0]?.body).toEqual({ id: "consent1" })
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("reports a revoke the server refused", async ({ expect }) => {
		mockClientName("Claude Code")

		const revocations = mockEndpoint(
			"POST",
			`${AUTH_REALTIME_BASE}/oauth2/delete-consent`,
			() => {
				throw createError({ statusCode: 500 })
			},
		)

		seedConsents([makeConsent()])

		// the mutation invalidates the list whether it succeeded or not, so
		// the refetch needs an answer either way
		mockEndpoint("GET", `${AUTH_REALTIME_BASE}/oauth2/get-consents`, () => [
			makeConsent(),
		])

		const wrapper = await mountSuspended(McpSection)
		await findButtonByText(wrapper, t("settings.mcp.revoke")).trigger("click")
		await settleMutations()
		await settleMutations()

		expect(revocations).toHaveLength(1)

		// the row stays: nothing was revoked, so nothing should look like it
		expect(wrapper.findAll("tbody tr")).toHaveLength(1)
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})
})
