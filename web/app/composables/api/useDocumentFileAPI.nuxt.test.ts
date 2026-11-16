import { registerEndpoint } from "@nuxt/test-utils/runtime"
import { getQuery, readFormData, type H3Event } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	runInApp,
	trackEndpointDisposal,
} from "./test-helpers"
import useDocumentFileAPI, {
	buildDocumentFileSrc,
	isDocumentFileSrc,
} from "./useDocumentFileAPI"

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
		handler: async (event: H3Event) => {
			const form = await readFormData(event)

			calls.push({
				query: getQuery(event),
				fieldNames: [...form.keys()],
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

	describe("isDocumentFileSrc", () => {
		const BASE = "https://app.example.com/core"

		it.for([
			{
				name: "accepts an attachment under the api base",
				input: `${BASE}/api/documents/doc-1/files/block-1-my%20notes.zip`,
				base: BASE,
				expected: true,
			},
			{
				name: "accepts a relative attachment under an empty base",
				input: "/api/documents/doc-1/files/block-1-notes.zip",
				base: "",
				expected: true,
			},
			{
				name: "rejects an attachment path on another host",
				input:
					"https://files.example.net/core/api/documents/doc-1/files/q3.exe",
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a host that only starts like the base host",
				input:
					"https://app.example.com.evil.net/core/api/documents/doc-1/files/q3.exe",
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a relative attachment under an absolute base",
				input: "/api/documents/doc-1/files/block-1-notes.zip",
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a protocol-relative address under an empty base",
				input: "//files.example.net/api/documents/doc-1/files/q3.exe",
				base: "",
				expected: false,
			},
			{
				name: "rejects another api path",
				input: `${BASE}/api/documents/doc-1`,
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a path climbing out of the attachment",
				input: `${BASE}/api/documents/doc-1/files/../../../users/me`,
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a path climbing out with backslashes",
				input: `${BASE}/api/documents/doc-1/files/..\\..\\..\\users\\me`,
				base: BASE,
				expected: false,
			},
			{
				name: "rejects a nested path below the attachment",
				input: `${BASE}/api/documents/doc-1/files/block-1/notes.zip`,
				base: BASE,
				expected: false,
			},
			{
				name: "rejects an empty address",
				input: "",
				base: "",
				expected: false,
			},
		])("$name", ({ input, base, expected }, { expect }) => {
			expect(isDocumentFileSrc(input, base)).toBe(expected)
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

	describe("downloadDocumentFile", () => {
		// $host is normally provided by the host.client plugin in desktop
		// builds; assign it directly so each test controls the bridge
		function withHost(host: unknown) {
			const nuxtApp = useNuxtApp() as unknown as { $host?: unknown }
			nuxtApp.$host = host
		}

		afterEach(() => {
			withHost(undefined)
		})

		it("hands the download and its progress to the desktop bridge", async ({
			expect,
		}) => {
			const download = vi.fn(
				(
					_url: string,
					_name: string,
					onProgress: (received: number, total: number) => void,
				) => {
					onProgress(3, 6)

					return Promise.resolve("completed")
				},
			)
			withHost({ files: { download } })
			const onProgress = vi.fn()
			const api = makeDocumentFileAPI()

			const result = await api.downloadDocumentFile.mutateAsync({
				src: "/api/documents/doc-1/files/block-1-notes.zip",
				name: "notes.zip",
				onProgress,
			})

			expect(result).toBe("completed")
			expect
				.soft(download)
				.toHaveBeenCalledExactlyOnceWith(
					"/api/documents/doc-1/files/block-1-notes.zip",
					"notes.zip",
					onProgress,
				)
			expect.soft(onProgress).toHaveBeenCalledExactlyOnceWith(3, 6)
		})

		it("passes on a dismissed save dialog", async ({ expect }) => {
			const download = vi.fn(() => Promise.resolve("cancelled"))
			withHost({ files: { download } })
			const api = makeDocumentFileAPI()

			await expect(
				api.downloadDocumentFile.mutateAsync({
					src: "/api/documents/doc-1/files/block-1-notes.zip",
					name: "notes.zip",
					onProgress: vi.fn(),
				}),
			).resolves.toBe("cancelled")

			expect(download).toHaveBeenCalledTimes(1)
		})

		it("rejects when the desktop bridge fails", async ({ expect }) => {
			const failure = new Error("disk full")
			const download = vi.fn(() => Promise.reject(failure))
			withHost({ files: { download } })
			const api = makeDocumentFileAPI()

			await expect(
				api.downloadDocumentFile.mutateAsync({
					src: "/api/documents/doc-1/files/block-1-notes.zip",
					name: "notes.zip",
					onProgress: vi.fn(),
				}),
			).rejects.toBe(failure)

			expect(download).toHaveBeenCalledTimes(1)
		})

		it("rejects outside the desktop app", async ({ expect }) => {
			const api = makeDocumentFileAPI()

			await expect(
				api.downloadDocumentFile.mutateAsync({
					src: "/api/documents/doc-1/files/block-1-notes.zip",
					name: "notes.zip",
					onProgress: vi.fn(),
				}),
			).rejects.toThrow("desktop host bridge missing")
		})
	})
})
