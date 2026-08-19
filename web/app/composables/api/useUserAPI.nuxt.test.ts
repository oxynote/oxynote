import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { setResponseHeader, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	runInApp,
	trackEndpointDisposal,
} from "./test-helpers"
import useUserAPI from "./useUserAPI"

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

// the tests share the app-wide query cache and the test-time endpoint
// registry, so they cannot interleave
describe("useUserAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

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
