import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useAuthAPI from "./useAuthAPI"

function makeAuthAPI() {
	return runInApp(() => useAuthAPI())
}

// creating the composable eagerly loads its query once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic
describe("useAuthAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchAuthConfig", () => {
		it("fetches the auth config", async ({ expect }) => {
			const configCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({ methods: ["email-password", "github"] }),
			)
			const api = makeAuthAPI()

			const result = await api.fetchAuthConfig.refresh()

			expect(result.data).toEqual({ methods: ["email-password", "github"] })
			expect(configCalls).toHaveLength(1)
		})
	})
})
