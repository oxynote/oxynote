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

	describe("isEmailEnabled", () => {
		it("assumes email is delivered until the config arrives", async ({
			expect,
		}) => {
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({ methods: ["email-password"], emailEnabled: false }),
			)
			const api = makeAuthAPI()

			expect(api.isEmailEnabled.value).toBe(true)

			// drains the eager load so it cannot land in the next test
			await api.fetchAuthConfig.refresh()
		})

		it("follows the server's answer once it arrives", async ({ expect }) => {
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({ methods: ["email-password"], emailEnabled: false }),
			)
			const api = makeAuthAPI()

			await api.fetchAuthConfig.refresh()

			expect(api.isEmailEnabled.value).toBe(false)
		})
	})
	describe("organization limits", () => {
		it("hides the default admin outside single-workspace mode", async ({
			expect,
		}) => {
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({
					singleOrganization: false,
					defaultAdmin: { email: "admin@example.com", password: null },
				}),
			)
			const api = makeAuthAPI()

			await api.fetchAuthConfig.refresh()

			expect(api.defaultAdmin.value).toBeNull()
		})

		it("assumes open signup and no member limit until the config arrives", async ({
			expect,
		}) => {
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({ singleOrganization: true, maxOrganizationMembers: 5 }),
			)
			const api = makeAuthAPI()

			expect(api.isSingleOrganization.value).toBe(false)
			expect(api.maxOrganizationMembers.value).toBeNull()

			// drains the eager load so it cannot land in the next test
			await api.fetchAuthConfig.refresh()
		})

		it("follows the server's answer once it arrives", async ({ expect }) => {
			mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/auth-config",
				() => ({
					singleOrganization: true,
					maxOrganizationMembers: 5,
					defaultAdmin: { email: "admin@example.com", password: null },
				}),
			)
			const api = makeAuthAPI()

			await api.fetchAuthConfig.refresh()

			expect(api.isSingleOrganization.value).toBe(true)
			expect(api.maxOrganizationMembers.value).toBe(5)
			expect(api.defaultAdmin.value).toEqual({
				email: "admin@example.com",
				password: null,
			})
		})
	})
})
