import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useCapabilitiesAPI from "./useCapabilitiesAPI"

function makeCapabilitiesAPI() {
	return runInApp(() => useCapabilitiesAPI())
}

function stubCapabilities(overrides: Partial<Capabilities> = {}): Capabilities {
	return {
		github: true,
		slack: true,
		changeDetection: true,
		search: true,
		aiAssistant: { status: AssistantStatus.Active, model: "claude-opus-5" },
		...overrides,
	}
}

// the composable is created once per test and its query loaded eagerly;
// refresh() joins that in-flight load rather than forcing a second request
describe("useCapabilitiesAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchCapabilities", () => {
		it("fetches the deployment's capabilities", async ({ expect }) => {
			const capabilities = stubCapabilities()
			const calls = mockEndpoint("GET", "/api/capabilities", () => capabilities)
			const api = makeCapabilitiesAPI()

			const result = await api.fetchCapabilities.refresh()

			expect(result.data).toEqual(capabilities)
			expect(calls).toHaveLength(1)
		})
	})

	describe("service flags", () => {
		it("maps each flag to its own field", async ({ expect }) => {
			mockEndpoint("GET", "/api/capabilities", () =>
				stubCapabilities({
					github: true,
					slack: false,
					changeDetection: true,
					search: false,
				}),
			)
			const api = makeCapabilitiesAPI()

			await api.fetchCapabilities.refresh()

			expect(api.isGithubEnabled.value).toBe(true)
			expect(api.isSlackEnabled.value).toBe(false)
			expect(api.isChangeDetectionEnabled.value).toBe(true)
			expect(api.isSearchEnabled.value).toBe(false)
		})

		it("reports every service enabled before the request resolves", ({
			expect,
		}) => {
			const api = makeCapabilitiesAPI()

			expect(api.isGithubEnabled.value).toBe(true)
			expect(api.isSlackEnabled.value).toBe(true)
			expect(api.isChangeDetectionEnabled.value).toBe(true)
			expect(api.isSearchEnabled.value).toBe(true)
			expect(api.isAssistantEnabled.value).toBe(true)
		})

		it("reports every service enabled when the request fails", async ({
			expect,
		}) => {
			mockEndpoint("GET", "/api/capabilities", (_call, event) => {
				setResponseStatus(event, 500)

				return { error: "boom" }
			})
			const api = makeCapabilitiesAPI()

			await api.fetchCapabilities.refresh()

			expect(api.fetchCapabilities.error.value).not.toBeNull()
			expect(api.isGithubEnabled.value).toBe(true)
			expect(api.isSlackEnabled.value).toBe(true)
			expect(api.isChangeDetectionEnabled.value).toBe(true)
			expect(api.isSearchEnabled.value).toBe(true)
			expect(api.isAssistantEnabled.value).toBe(true)
		})
	})

	describe("isAssistantEnabled", () => {
		const cc: Record<string, { status: AssistantStatus; enabled: boolean }> = {
			"runs a full-strength model": {
				status: AssistantStatus.Active,
				enabled: true,
			},
			"runs a mid-tier model with limitations": {
				status: AssistantStatus.ActiveButWeak,
				enabled: true,
			},
			"is disabled because the model is too weak": {
				status: AssistantStatus.InactiveTooWeak,
				enabled: false,
			},
			"is disabled because no provider is configured": {
				status: AssistantStatus.Inactive,
				enabled: false,
			},
		}

		Object.entries(cc).forEach(([name, c]) => {
			it(`reports ${c.enabled ? "enabled" : "disabled"} when the assistant ${name}`, async ({
				expect,
			}) => {
				mockEndpoint("GET", "/api/capabilities", () =>
					stubCapabilities({
						aiAssistant: { status: c.status, model: "some-model" },
					}),
				)
				const api = makeCapabilitiesAPI()

				await api.fetchCapabilities.refresh()

				expect(api.isAssistantEnabled.value).toBe(c.enabled)
			})
		})
	})

	describe("assistantStatus / assistantModel", () => {
		it("passes the status and configured model through", async ({ expect }) => {
			mockEndpoint("GET", "/api/capabilities", () =>
				stubCapabilities({
					aiAssistant: {
						status: AssistantStatus.ActiveButWeak,
						model: "gpt-4o-mini",
					},
				}),
			)
			const api = makeCapabilitiesAPI()

			await api.fetchCapabilities.refresh()

			expect(api.assistantStatus.value).toBe(AssistantStatus.ActiveButWeak)
			expect(api.assistantModel.value).toBe("gpt-4o-mini")
		})

		it("reports no status and an empty model while unknown", ({ expect }) => {
			const api = makeCapabilitiesAPI()

			expect(api.assistantStatus.value).toBeNull()
			expect(api.assistantModel.value).toBe("")
		})
	})
})
