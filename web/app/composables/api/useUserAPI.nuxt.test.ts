import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { createError, readFormData, setResponseHeader, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
	trackEndpointDisposal,
} from "./test-helpers"
import useUserAPI from "./useUserAPI"

const SPOTLIGHTS_KEY = ["users", "spotlights"]

function makeUserAPI() {
	return runInApp(() => useUserAPI())
}

interface RecordedUpload {
	fieldNames: string[]
}

// registers an upload handler on the test-time h3 app that the real api
// clients route through, recording the multipart field names of each call
// for accounting. The test transport hands the fetch body to the handler
// unserialized (node-mock-http assigns it straight to req.body), so the
// field names are read off the original FormData rather than parsed from
// a wire format. The handler answers with the given location header, or
// with no header at all when location is omitted.
function mockUploadEndpoint(url: string, location?: string): RecordedUpload[] {
	const calls: RecordedUpload[] = []

	const dispose = registerEndpoint(url, {
		method: "PUT",
		handler: async (event: H3Event) => {
			const body = await readFormData(event)

			calls.push({
				fieldNames: [...body.keys()],
			})

			if (location !== undefined) {
				setResponseHeader(event, "location", location)
			}

			return null
		},
	})

	trackEndpointDisposal(dispose)

	return calls
}

// the tests share the app-wide query cache and the test-time endpoint
// registry, so they cannot interleave
describe("useUserAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useFetchUserSpotlights", () => {
		it("fetches the user's spotlight states", async ({ expect }) => {
			const calls = mockEndpoint("GET", "/api/users/spotlights", () => ({
				star: SpotlightState.Shown,
			}))
			const api = makeUserAPI()

			const result = await runInApp(() =>
				api.useFetchUserSpotlights(),
			).refresh()

			expect(result.data).toEqual({ star: SpotlightState.Shown })
			expect(calls).toHaveLength(1)
		})
	})

	describe("updateUserSpotlights", () => {
		it("shows the new state while the write is in flight", async ({
			expect,
		}) => {
			seedQueryData(SPOTLIGHTS_KEY, { star: SpotlightState.Shown })
			const put = mockDeferredEndpoint("PUT", "/api/users/spotlights")
			const reads = mockEndpoint("GET", "/api/users/spotlights", () => ({
				star: SpotlightState.Dismissed,
			}))
			const api = makeUserAPI()
			// the refetch reaches only a query someone still reads
			runInApp(() => api.useFetchUserSpotlights())

			const pending = api.updateUserSpotlights.mutateAsync({
				star: SpotlightState.Dismissed,
			})
			await put.reached

			expect(readQueryData(SPOTLIGHTS_KEY)).toEqual({
				star: SpotlightState.Dismissed,
			})

			put.resolve(null)
			await pending

			expect.soft(put.calls).toHaveLength(1)
			expect
				.soft(put.calls[0]?.body)
				.toEqual({ star: SpotlightState.Dismissed })
			// the answer core keeps replaces the optimistic one
			expect.soft(reads).toHaveLength(1)
		})

		it("puts the previous state back when the write fails", async ({
			expect,
		}) => {
			seedQueryData(SPOTLIGHTS_KEY, { star: SpotlightState.Shown })
			const put = mockEndpoint("PUT", "/api/users/spotlights", () => {
				throw createError({ statusCode: 500 })
			})
			const reads = mockEndpoint("GET", "/api/users/spotlights", () => ({
				star: SpotlightState.Shown,
			}))
			const api = makeUserAPI()

			await expect(
				api.updateUserSpotlights.mutateAsync({
					star: SpotlightState.Dismissed,
				}),
			).rejects.toThrow()

			expect.soft(readQueryData(SPOTLIGHTS_KEY)).toEqual({
				star: SpotlightState.Shown,
			})
			expect.soft(put).toHaveLength(1)
			expect.soft(reads).toHaveLength(0)
		})
		it("keeps a newer state when the write fails", async ({ expect }) => {
			seedQueryData(SPOTLIGHTS_KEY, { star: SpotlightState.Shown })
			const put = mockDeferredEndpoint("PUT", "/api/users/spotlights")
			const api = makeUserAPI()

			const pending = api.updateUserSpotlights.mutateAsync({
				star: SpotlightState.Dismissed,
			})
			await put.reached
			seedQueryData(SPOTLIGHTS_KEY, { star: null })
			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()

			expect.soft(readQueryData(SPOTLIGHTS_KEY)).toEqual({ star: null })
			expect.soft(put.calls).toHaveLength(1)
		})
	})

	describe("uploadUserImage", () => {
		it("uploads the image and returns the stored location", async ({
			expect,
		}) => {
			const uploadCalls = mockUploadEndpoint(
				"/api/users/image",
				"/files/user-image-1",
			)
			const api = makeUserAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			const location = await api.uploadUserImage.mutateAsync(file)

			expect(location).toBe("/files/user-image-1")
			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.fieldNames).toEqual(["image"])
		})

		it("rejects when the response has no location header", async ({
			expect,
		}) => {
			const uploadCalls = mockUploadEndpoint("/api/users/image")
			const api = makeUserAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			await expect(api.uploadUserImage.mutateAsync(file)).rejects.toThrow(
				"missing location header",
			)

			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.fieldNames).toEqual(["image"])
		})
	})
})
