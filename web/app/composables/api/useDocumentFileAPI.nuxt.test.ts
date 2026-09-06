import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { getQuery, type H3Event } from "h3"
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

// the body the server answers an upload with
const UPLOADED = {
	id: "block-1",
	name: "notes.zip",
	size: 2048,
	contentType: "application/zip",
}

// registers an upload handler on the test-time h3 app that the real api
// clients route through, recording the query parameters and multipart
// field names of each call for accounting. The test transport hands the
// fetch body to the handler unserialized (node-mock-http assigns it
// straight to req.body), so the field names are read off the original
// FormData rather than parsed from a wire format. The handler answers
// with the recorded file as its body, or with no body when body is null.
function mockUploadEndpoint(
	url: string,
	body: typeof UPLOADED | null = UPLOADED,
): RecordedUpload[] {
	const calls: RecordedUpload[] = []

	const dispose = registerEndpoint(url, {
		method: "POST",
		handler: (event: H3Event) => {
			const req = (event.node.req as { body?: unknown }).body

			calls.push({
				query: getQuery(event),
				fieldNames: req instanceof FormData ? [...req.keys()] : [],
			})

			return body
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
		it("builds the file url from the document id, block id and escaped name", ({
			expect,
		}) => {
			const src = runInApp(() =>
				buildDocumentFileSrc("doc-1", "block-1", "my notes.zip"),
			)

			// the test runtime config leaves coreAPIBaseHttpURL empty, so
			// the url is origin-relative
			expect(src).toBe("/api/documents/doc-1/files/block-1-my%20notes.zip")
		})
	})

	describe("uploadDocumentFile", () => {
		it("uploads the file and returns what the server recorded", async ({
			expect,
		}) => {
			const uploadCalls = mockUploadEndpoint("/api/documents/doc-1/files")
			const api = makeDocumentFileAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			const uploaded = await api.uploadDocumentFile.mutateAsync({
				documentId: "doc-1",
				id: "block 1/a",
				loc: DocumentFileLocation.Document,
				kind: DocumentFileKind.Image,
				file,
			})

			expect(uploaded).toEqual({
				name: "notes.zip",
				size: 2048,
				contentType: "application/zip",
			})
			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.query).toEqual({
				id: "block 1/a",
				location: "document",
				kind: "image",
			})
			expect(uploadCalls[0]?.fieldNames).toEqual(["file"])
		})

		it("sends the file kind the caller names", async ({ expect }) => {
			const uploadCalls = mockUploadEndpoint("/api/documents/doc-1/files")
			const api = makeDocumentFileAPI()
			const file = new File(["x"], "x.zip", { type: "application/zip" })

			await api.uploadDocumentFile.mutateAsync({
				documentId: "doc-1",
				id: "block-1",
				loc: DocumentFileLocation.Document,
				kind: DocumentFileKind.File,
				file,
			})

			expect(uploadCalls).toHaveLength(1)
			expect(uploadCalls[0]?.query).toEqual({
				id: "block-1",
				location: "document",
				kind: "file",
			})
		})

		it("rejects when the response has no body", async ({ expect }) => {
			const uploadCalls = mockUploadEndpoint("/api/documents/doc-1/files", null)
			const api = makeDocumentFileAPI()
			const file = new File(["x"], "x.png", { type: "image/png" })

			await expect(
				api.uploadDocumentFile.mutateAsync({
					documentId: "doc-1",
					id: "block-1",
					loc: DocumentFileLocation.Document,
					kind: DocumentFileKind.File,
					file,
				}),
			).rejects.toThrow("missing upload response body")

			expect(uploadCalls).toHaveLength(1)
		})
	})
})
