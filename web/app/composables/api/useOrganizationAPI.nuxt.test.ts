import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { setResponseHeader, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
	trackEndpointDisposal,
} from "./test-helpers"
import useOrganizationAPI from "./useOrganizationAPI"

function makeOrganizationAPI() {
	return runInApp(() => useOrganizationAPI())
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
		handler: (event: H3Event) => {
			const body = (event.node.req as { body?: unknown }).body

			calls.push({
				fieldNames: body instanceof FormData ? [...body.keys()] : [],
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

// creating the composable eagerly loads its query once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic
describe("useOrganizationAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("uploadOrganizationLogo", () => {
		it("uploads the logo and returns the stored location", async ({
			expect,
		}) => {
			const statsCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/organizations/stats",
				() => ({ availableSlots: 3 }),
			)
			const uploadCalls = mockUploadEndpoint(
				"/api/organizations/logo",
				"/files/organization-logo-1",
			)
			const api = makeOrganizationAPI()
			// settle the eager stats load so its single call is accounted for
			await api.fetchOrganizationStats.refresh()
			const file = new File(["x"], "x.png", { type: "image/png" })

			const location = await api.uploadOrganizationLogo.mutateAsync(file)

			expect(location).toBe("/files/organization-logo-1")
			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.fieldNames).toEqual(["logo"])
			expect(statsCalls).toHaveLength(1)
		})

		it("rejects when the response has no location header", async ({
			expect,
		}) => {
			const statsCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/organizations/stats",
				() => ({ availableSlots: 3 }),
			)
			const uploadCalls = mockUploadEndpoint("/api/organizations/logo")
			const api = makeOrganizationAPI()
			// settle the eager stats load so its single call is accounted for
			await api.fetchOrganizationStats.refresh()
			const file = new File(["x"], "x.png", { type: "image/png" })

			await expect(
				api.uploadOrganizationLogo.mutateAsync(file),
			).rejects.toThrow("missing location header")

			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.fieldNames).toEqual(["logo"])
			expect(statsCalls).toHaveLength(1)
		})
	})

	describe("fetchOrganizationStats", () => {
		it("fetches the organization stats", async ({ expect }) => {
			const statsCalls = mockEndpoint(
				"GET",
				"http://test.local/auth-realtime/api/organizations/stats",
				() => ({ availableSlots: 3 }),
			)
			const api = makeOrganizationAPI()

			const result = await api.fetchOrganizationStats.refresh()

			expect(result.data).toEqual({ availableSlots: 3 })
			expect(statsCalls).toHaveLength(1)
		})
	})
})
