import type { FetchError } from "ofetch"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useSlackAPI from "./useSlackAPI"

const CONNECTED_KEY = ["slack", "connected"] as const
const SETTINGS_KEY = ["slack", "user-link-settings"] as const

function makeSlackAPI() {
	return runInApp(() => useSlackAPI())
}

// creating the composable eagerly loads its queries once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic. The
// settings query also refreshes the connection status internally, and that
// refresh dedupes into the same eager status load.
describe("useSlackAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchSlackConnectionStatus", () => {
		it("fetches the connection status", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const api = makeSlackAPI()

			const result = await api.fetchSlackConnectionStatus.refresh()

			expect(result.data).toEqual({ connected: true, configured: true })
			expect(statusCalls).toHaveLength(1)
		})
	})

	describe("slackConfigured", () => {
		it("reports configured while the status is unknown", ({ expect }) => {
			const api = makeSlackAPI()

			expect(api.slackConfigured.value).toBe(true)
		})

		it("reports unconfigured once the status says so", async ({ expect }) => {
			mockEndpoint("GET", "/api/slack", () => ({
				connected: false,
				configured: false,
			}))
			const api = makeSlackAPI()

			await api.fetchSlackConnectionStatus.refresh()

			expect(api.slackConfigured.value).toBe(false)
		})
	})

	describe("fetchSlackInstallURL", () => {
		it("fetches the install url", async ({ expect }) => {
			const installCalls = mockEndpoint("GET", "/api/slack/install", () => ({
				url: "https://slack.test/i",
			}))
			const api = makeSlackAPI()

			const result = await api.fetchSlackInstallURL()

			expect(result).toEqual({ url: "https://slack.test/i" })
			expect(installCalls).toHaveLength(1)
		})
	})

	describe("connectSlack", () => {
		it("connects with the callback query parameters", async ({ expect }) => {
			const connectCalls = mockEndpoint("GET", "/api/slack/connect", () => ({}))
			const api = makeSlackAPI()

			await api.connectSlack.mutateAsync(new URLSearchParams({ code: "c1" }))

			expect(connectCalls).toHaveLength(1)
			expect(connectCalls[0]?.query).toEqual({ code: "c1" })
		})

		it("connects without query parameters", async ({ expect }) => {
			const connectCalls = mockEndpoint("GET", "/api/slack/connect", () => ({}))
			const api = makeSlackAPI()

			await api.connectSlack.mutateAsync(new URLSearchParams())

			expect(connectCalls).toHaveLength(1)
			expect(connectCalls[0]?.query).toEqual({})
		})
	})

	describe("linkSlackUser", () => {
		it("links the user with the callback query parameters", async ({
			expect,
		}) => {
			const linkCalls = mockEndpoint("GET", "/api/slack/users/link", () => ({
				linked: true,
			}))
			const api = makeSlackAPI()

			const result = await api.linkSlackUser.mutateAsync(
				new URLSearchParams({ code: "c1" }),
			)

			expect(result).toEqual({ linked: true })
			expect(linkCalls).toHaveLength(1)
			expect(linkCalls[0]?.query).toEqual({ code: "c1" })
		})

		it("links the user without query parameters", async ({ expect }) => {
			const linkCalls = mockEndpoint("GET", "/api/slack/users/link", () => ({
				linked: false,
			}))
			const api = makeSlackAPI()

			const result = await api.linkSlackUser.mutateAsync(new URLSearchParams())

			expect(result).toEqual({ linked: false })
			expect(linkCalls).toHaveLength(1)
			expect(linkCalls[0]?.query).toEqual({})
		})
	})

	describe("disconnectSlack", () => {
		it("disconnects and refreshes the connection status", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const deleteCalls = mockEndpoint("DELETE", "/api/slack", () => ({}))
			const api = makeSlackAPI()
			await api.fetchSlackConnectionStatus.refresh()

			await api.disconnectSlack.mutateAsync()

			expect(deleteCalls).toHaveLength(1)
			// the success invalidation refetches the active status query
			expect(statusCalls).toHaveLength(2)
			expect(api.fetchSlackConnectionStatus.data.value).toEqual({
				connected: true,
				configured: true,
			})
		})

		it("rolls back the optimistic disconnect when the request fails", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("DELETE", "/api/slack", () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeSlackAPI()
			await api.fetchSlackConnectionStatus.refresh()

			await expect(api.disconnectSlack.mutateAsync()).rejects.toThrow()

			expect(statusCalls).toHaveLength(1)
			expect(api.fetchSlackConnectionStatus.data.value).toEqual({
				connected: true,
				configured: true,
			})
		})

		it("skips the rollback when the cache changed after the optimistic disconnect", async ({
			expect,
		}) => {
			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("DELETE", "/api/slack", () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeSlackAPI()
			await api.fetchSlackConnectionStatus.refresh()

			const pending = api.disconnectSlack.mutateAsync()
			await deleteReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(api.fetchSlackConnectionStatus.data.value).toEqual({
				connected: false,
				configured: true,
			})
			seedQueryData(CONNECTED_KEY, {
				connected: true,
				configured: false,
			})
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(api.fetchSlackConnectionStatus.data.value).toEqual({
				connected: true,
				configured: false,
			})
		})
	})

	describe("fetchSlackUserLinkSettings", () => {
		it("returns null when slack is not configured", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: false,
				configured: false,
			}))
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: true,
			}))
			const api = makeSlackAPI()

			const result = await api.fetchSlackUserLinkSettings.refresh()

			expect(result.data).toBeNull()
			expect(usersCalls).toHaveLength(0)
			expect(statusCalls).toHaveLength(1)
		})

		it("fetches the settings when slack is configured", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: true,
			}))
			const api = makeSlackAPI()

			const result = await api.fetchSlackUserLinkSettings.refresh()

			expect(result.data).toEqual({ notifications: true })
			expect(usersCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})

		it("returns null when the user has no slack link", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => {
				throw createError({ statusCode: 404 })
			})
			const api = makeSlackAPI()

			const result = await api.fetchSlackUserLinkSettings.refresh()

			expect(result.data).toBeNull()
			expect(usersCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})

		it("errors when the settings request fails", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const successCalls = mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: true,
			}))
			const api = makeSlackAPI()
			// a failing query cannot dedupe the eager creation load into the
			// explicit act, so the entry is warmed with a success first and
			// the failure is a single forced refetch through a later-wins
			// handler registration
			await api.fetchSlackUserLinkSettings.refresh()
			const failedCalls = mockEndpoint("GET", "/api/slack/users", () => {
				throw createError({ statusCode: 500 })
			})

			await api.fetchSlackUserLinkSettings.refetch()

			const error = api.fetchSlackUserLinkSettings.error
				.value as FetchError | null
			expect(error?.statusCode).toBe(500)
			// a non-404 failure keeps the previously fetched settings, unlike
			// the 404 branch that resolves them to null
			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: true,
			})
			expect(successCalls).toHaveLength(1)
			// ofetch retries a failed GET once by default on a 500 response
			expect(failedCalls).toHaveLength(2)
			expect(statusCalls).toHaveLength(1)
		})
	})

	describe("updateSlackUserLinkSettings", () => {
		it("optimistically applies the settings and refetches them after success", async ({
			expect,
		}) => {
			let resolvePut: (value: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			let serverSettings = { notifications: false }
			const usersCalls = mockEndpoint(
				"GET",
				"/api/slack/users",
				() => serverSettings,
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/slack/users/settings",
				(call) => {
					serverSettings = call.body as { notifications: boolean }
					putReached()

					return new Promise((resolve) => {
						resolvePut = resolve
					})
				},
			)
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			const pending = api.updateSlackUserLinkSettings.mutateAsync({
				notifications: true,
			})
			await putReachedSignal

			// the optimistic update lands before the request resolves
			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: true,
			})
			resolvePut(serverSettings)

			await pending
			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: true,
			})
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ notifications: true })
			// the success invalidation refetches the active settings query;
			// the connection status stays fresh and is not refetched
			expect(usersCalls).toHaveLength(2)
			expect(statusCalls).toHaveLength(1)
		})

		it("rolls back the optimistic settings when the request fails", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: false,
			}))
			const putCalls = mockEndpoint("PUT", "/api/slack/users/settings", () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			await expect(
				api.updateSlackUserLinkSettings.mutateAsync({ notifications: true }),
			).rejects.toThrow()

			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: false,
			})
			expect(putCalls).toHaveLength(1)
			expect(usersCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: false,
			}))
			mockEndpoint("PUT", "/api/slack/users/settings", () => {
				putReached()

				return new Promise((_resolve, reject) => {
					rejectPut = reject
				})
			})
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			const pending = api.updateSlackUserLinkSettings.mutateAsync({
				notifications: true,
			})
			await putReachedSignal

			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: true,
			})
			// divergent data written after the optimistic update must survive
			// the failure; null is the only cache value distinguishable from
			// both the optimistic and the original settings
			seedQueryData(SETTINGS_KEY, null)
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(api.fetchSlackUserLinkSettings.data.value).toBeNull()
		})
	})

	describe("unlinkSlackUser", () => {
		it("optimistically clears the settings and refetches them after success", async ({
			expect,
		}) => {
			let resolveDelete: (value: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			let linked = true
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => {
				if (!linked) {
					throw createError({ statusCode: 404 })
				}

				return { notifications: true }
			})
			const deleteCalls = mockEndpoint("DELETE", "/api/slack/users", () => {
				linked = false
				deleteReached()

				return new Promise((resolve) => {
					resolveDelete = resolve
				})
			})
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			const pending = api.unlinkSlackUser.mutateAsync()
			await deleteReachedSignal

			// the optimistic clear lands before the request resolves
			expect(api.fetchSlackUserLinkSettings.data.value).toBeNull()
			resolveDelete({})

			await pending
			// the success invalidation refetches the active settings query,
			// which now resolves the unlinked 404 into null
			expect(api.fetchSlackUserLinkSettings.data.value).toBeNull()
			expect(deleteCalls).toHaveLength(1)
			expect(usersCalls).toHaveLength(2)
			expect(statusCalls).toHaveLength(1)
		})

		it("rolls back the cleared settings when the request fails", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			const usersCalls = mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: true,
			}))
			const deleteCalls = mockEndpoint("DELETE", "/api/slack/users", () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			await expect(api.unlinkSlackUser.mutateAsync()).rejects.toThrow()

			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: true,
			})
			expect(deleteCalls).toHaveLength(1)
			expect(usersCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})

		it("skips the rollback when the cache changed after the optimistic clear", async ({
			expect,
		}) => {
			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			mockEndpoint("GET", "/api/slack", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("GET", "/api/slack/users", () => ({
				notifications: true,
			}))
			mockEndpoint("DELETE", "/api/slack/users", () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeSlackAPI()
			await api.fetchSlackUserLinkSettings.refresh()

			const pending = api.unlinkSlackUser.mutateAsync()
			await deleteReachedSignal

			expect(api.fetchSlackUserLinkSettings.data.value).toBeNull()
			// divergent data written after the optimistic clear must survive
			// the failure
			seedQueryData(SETTINGS_KEY, { notifications: false })
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(api.fetchSlackUserLinkSettings.data.value).toEqual({
				notifications: false,
			})
		})
	})
})
