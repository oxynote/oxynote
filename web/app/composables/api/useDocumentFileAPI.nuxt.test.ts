import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { getQuery, setResponseHeader, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	runInApp,
	trackEndpointDisposal,
} from "./test-helpers"
import useDocumentFileAPI, { buildDocumentFileSrc } from "./useDocumentFileAPI"

function makeDocumentFileAPI() {
	return runInApp(() => useDocumentFileAPI())
}

interface RecordedUpload {
	query: Record<string, unknown>
	fieldNames: string[]
}

// registers an upload handler on the test-time h3 app that the real api
// clients route through, recording the query parameters and multipart
// field names of each call for accounting. The test transport hands the
// fetch body to the handler unserialized (node-mock-http assigns it
// straight to req.body), so the field names are read off the original
// FormData rather than parsed from a wire format. The handler answers
// with the given location header, or with no header at all when location
// is omitted.
function mockUploadEndpoint(url: string, location?: string): RecordedUpload[] {
	const calls: RecordedUpload[] = []

	const dispose = registerEndpoint(url, {
		method: "POST",
		handler: (event: H3Event) => {
			const body = (event.node.req as { body?: unknown }).body

			calls.push({
				query: getQuery(event),
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
describe("useDocumentFileAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("buildDocumentFileSrc", () => {
		it("builds the file url from the document and block ids", ({ expect }) => {
			const src = runInApp(() => buildDocumentFileSrc("doc-1", "block-1"))

			// the test runtime config leaves coreAPIBaseHttpURL empty, so
			// the url is origin-relative
			expect(src).toBe("/api/documents/doc-1/files/block-1")
		})
	})

	describe("uploadDocumentFile", () => {
		it("uploads the file and returns the stored location", async ({
			expect,
		}) => {
			const uploadCalls = mockUploadEndpoint(
				"/api/documents/doc-1/files",
				"/files/document-file-1",
			)
			const api = makeDocumentFileAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			const location = await api.uploadDocumentFile.mutateAsync({
				documentId: "doc-1",
				id: "block 1/a",
				loc: DocumentFileLocation.Document,
				file,
			})

			expect(location).toBe("/files/document-file-1")
			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.query).toEqual({
				id: "block 1/a",
				location: "document",
			})
			expect(uploadCalls[0]?.fieldNames).toEqual(["file"])
		})

		it("rejects when the response has no location header", async ({
			expect,
		}) => {
			const uploadCalls = mockUploadEndpoint("/api/documents/doc-1/files")
			const api = makeDocumentFileAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			await expect(
				api.uploadDocumentFile.mutateAsync({
					documentId: "doc-1",
					id: "block-1",
					loc: DocumentFileLocation.Comment,
					file,
				}),
			).rejects.toThrow("missing location header")

			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.query).toEqual({
				id: "block-1",
				location: "comment",
			})
			expect(uploadCalls[0]?.fieldNames).toEqual(["file"])
		})
	})
})
