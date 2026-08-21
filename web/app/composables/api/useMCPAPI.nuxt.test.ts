import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useMCPAPI from "./useMCPAPI"

function makeMCPAPI() {
	return runInApp(() => useMCPAPI())
}

const stubConsent = {
	id: "consent1",
	clientId: "client1",
	scopes: ["documents:read", "documents:write"],
	createdAt: "2026-08-21T10:00:00.000Z",
}

describe("useMCPAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchConsents", () => {
		it("fetches the user's authorized clients", async ({ expect }) => {
			const consentCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth/oauth2/get-consents",
				() => [stubConsent],
			)
			const api = makeMCPAPI()

			const result = await api.fetchConsents.refresh()

			expect(result.data).toEqual([stubConsent])
			expect(consentCalls).toHaveLength(1)
		})
	})

	describe("fetchPublicClient", () => {
		it("fetches a client's public fields", async ({ expect }) => {
			const clientCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth/oauth2/public-client",
				() => ({ client_id: "client1", client_name: "Claude Code" }),
			)
			const api = makeMCPAPI()

			const result = await api.fetchPublicClient("client1")

			expect(result).toEqual({
				client_id: "client1",
				client_name: "Claude Code",
			})
			expect(clientCalls).toHaveLength(1)
		})
	})

	describe("revokeConsent", () => {
		it("deletes the consent and refreshes the list", async ({ expect }) => {
			const deleteCalls = mockEndpoint(
				"POST",
				"http://test.local/auth-realtime/api/auth/oauth2/delete-consent",
				() => ({}),
			)
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth/oauth2/get-consents",
				() => [],
			)
			const api = makeMCPAPI()

			await api.revokeConsent.mutateAsync("consent1")

			expect(deleteCalls).toHaveLength(1)
			expect(deleteCalls[0]?.body).toEqual({ id: "consent1" })
		})
	})

	describe("decideConsent", () => {
		it("posts the decision with the page's oauth query", async ({
			expect,
		}) => {
			const consentCalls = mockEndpoint(
				"POST",
				"http://test.local/auth-realtime/api/auth/oauth2/consent",
				() => ({ redirect: true, url: "https://client.test/cb?code=abc" }),
			)
			const api = makeMCPAPI()

			const result = await api.decideConsent(true, "?client_id=client1")

			expect(result).toEqual({
				redirect: true,
				url: "https://client.test/cb?code=abc",
			})
			expect(consentCalls).toHaveLength(1)
			expect(consentCalls[0]?.body).toEqual({
				accept: true,
				oauth_query: "?client_id=client1",
			})
		})
	})
})
