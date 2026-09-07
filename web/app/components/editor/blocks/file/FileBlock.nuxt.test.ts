import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { setResponseHeader, setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import FileBlock from "./FileBlock.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"
import {
	disposeMockEndpoints,
	makeXid,
	matchingString,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { settleMutations, t, WAIT_FOR_OPTIONS } from "~/components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: {
		custom: vi.fn(),
		dismiss: vi.fn(),
	},
}))

const DOCUMENT_ID = makeXid("doc")

const useRouteMock = vi.hoisted(() => vi.fn())
mockNuxtImport("useRoute", () => useRouteMock)

function onDocumentPage() {
	useRouteMock.mockReturnValue({
		params: { documentSlug: `page-${DOCUMENT_ID}` },
	})
}

function mountFile(
	attrs: Record<string, unknown> = {},
	updateAttributes: (attrs: Record<string, unknown>) => void = () => undefined,
) {
	return mountNodeView(FileBlock, {
		node: makeNode({ uid: "file-1", ...attrs }),
		updateAttributes: updateAttributes,
	})
}

// an uploaded attachment as the node stores it
function storedZip(attrs: Record<string, unknown> = {}) {
	return {
		src: "https://cdn.test/notes.zip",
		name: "notes.zip",
		size: 2_516_582,
		contentType: "application/zip",
		...attrs,
	}
}

function fileInput(wrapper: VueWrapper): HTMLInputElement {
	return wrapper.get("input[type='file']").element as HTMLInputElement
}

async function pickFile(wrapper: VueWrapper, file?: File) {
	const input = wrapper.get("input[type='file']")
	Object.defineProperty(input.element, "files", {
		value: file ? [file] : [],
		configurable: true,
	})

	await input.trigger("change")
	await settleMutations()
}

function zipFile() {
	return new File(["zipped"], "notes.zip", { type: "application/zip" })
}

// what the server answers a successful upload with: the name and size
// it recorded and the type it detected, which need not match the file
function uploadedZip() {
	return {
		id: "file-1",
		name: "notes.zip",
		size: 6,
		contentType: "application/zip",
	}
}

// the route mock, the editable flag, the editor store and the mocked
// toast module are all shared by the whole file, so these tests cannot
// interleave
describe("<FileBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		vi.mocked(toast.custom).mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		onDocumentPage()
	})

	afterEach(disposeMockEndpoints)

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountFile()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("file-1")
		expect(root.attributes("data-uid")).toBe("file-1")
		expect(root.attributes("data-type")).toBe("fileBlock")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountFile({
			nodeCommentId: "comment-1",
			diffStatus: "modified",
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("invites the reader to pick a file while the block is empty", async ({
		expect,
	}) => {
		const wrapper = await mountFile()

		expect(wrapper.text()).toBe(t("editor.file.description"))
		expect(wrapper.find("a").exists()).toBe(false)
	})

	it("reports an empty block as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFile()

		expect(wrapper.text()).toBe(t("editor.file.empty"))
	})

	it("reports an empty block as empty while a review diff is active", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountFile()

		expect(wrapper.text()).toBe(t("editor.file.empty"))
	})

	it("reports progress while an upload is running", async ({ expect }) => {
		const wrapper = await mountFile({ uploading: true })

		expect(wrapper.text()).toBe(t("editor.file.uploading"))
	})

	it("reports upload as unavailable off a document page", async ({
		expect,
	}) => {
		useRouteMock.mockReturnValue({ params: {} })

		const wrapper = await mountFile()

		expect(wrapper.text()).toBe(t("editor.file.upload-unavailable"))
	})

	it("reports upload as unavailable for an unrecognisable document slug", async ({
		expect,
	}) => {
		useRouteMock.mockReturnValue({ params: { documentSlug: "not-a-doc" } })

		const wrapper = await mountFile()

		expect(wrapper.text()).toBe(t("editor.file.upload-unavailable"))
	})

	it("shows the uploaded file as a card with its name and size", async ({
		expect,
	}) => {
		const wrapper = await mountFile(storedZip())

		const card = wrapper.get("a")

		expect(card.text()).toContain("notes.zip")
		expect(card.text()).toContain("2.4 MB")
		expect(wrapper.find("input[type='file']").exists()).toBe(false)
	})

	it("renders the card from the node's attributes without a request", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/files/file-1`,
			() => null,
		)

		const wrapper = await mountFile(
			storedZip({ src: `/api/documents/${DOCUMENT_ID}/files/file-1` }),
		)

		expect(wrapper.get("a").text()).toContain("notes.zip")
		expect(calls).toHaveLength(0)
	})

	it("leaves the size out when the node stores none", async ({ expect }) => {
		const wrapper = await mountFile(storedZip({ size: null }))

		expect(wrapper.get("a").text().trim()).toBe("notes.zip")
	})

	it.for([
		{
			name: "report.pdf",
			contentType: "application/pdf",
			expected: "lucide:file-text",
		},
		{
			name: "clip.mp4",
			contentType: "video/mp4",
			expected: "lucide:file-video-camera",
		},
		{
			name: "notes.zip",
			contentType: "application/zip",
			expected: "lucide:file-archive",
		},
		{
			name: "main.go",
			contentType: "text/plain",
			expected: "lucide:file-code",
		},
		{
			name: "thing.bin",
			contentType: "application/octet-stream",
			expected: "lucide:file",
		},
	])(
		"shows the $expected icon for $name",
		async ({ name, contentType, expected }, { expect }) => {
			const wrapper = await mountFile(storedZip({ name, contentType }))

			expect(wrapper.get("a .iconify").classes()).toContain(`i-${expected}`)
		},
	)

	it.for([
		{ name: "a PDF", contentType: "application/pdf" },
		{ name: "a video", contentType: "video/mp4" },
		{ name: "an audio file", contentType: "audio/mpeg" },
		{ name: "an image", contentType: "image/png" },
		{ name: "a plain text file", contentType: "text/plain; charset=utf-8" },
	])("opens $name in a new tab", async ({ contentType }, { expect }) => {
		const wrapper = await mountFile(storedZip({ contentType }))

		const card = wrapper.get("a")

		expect(card.attributes("href")).toBe("https://cdn.test/notes.zip")
		expect(card.attributes("target")).toBe("_blank")
		expect(card.attributes("rel")).toBe("noopener")
	})

	it.for([
		{ name: "an archive", contentType: "application/zip" },
		{ name: "an HTML file", contentType: "text/html" },
		{ name: "an SVG", contentType: "image/svg+xml" },
		{ name: "a file of unknown type", contentType: null },
	])("links $name as a plain download", async ({ contentType }, { expect }) => {
		const wrapper = await mountFile(storedZip({ contentType }))

		const card = wrapper.get("a")

		expect(card.attributes("href")).toBe("https://cdn.test/notes.zip")
		expect(card.attributes("target")).toBeUndefined()
		expect(card.attributes("rel")).toBe("noopener")
	})

	it.for([
		{ name: "a javascript: address", src: "javascript:alert(1)" },
		{
			name: "a data: address",
			src: "data:text/html,<script>alert(1)</script>",
		},
		{ name: "an unparsable address", src: "http://[" },
	])("treats $name as no file", async ({ src }, { expect }) => {
		const wrapper = await mountFile(storedZip({ src }))

		expect(wrapper.find("a").exists()).toBe(false)
		expect(wrapper.find("input[type='file']").exists()).toBe(true)
	})

	it("keeps the card a link in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFile(storedZip())

		expect(wrapper.get("a").attributes("href")).toBe(
			"https://cdn.test/notes.zip",
		)
	})

	it.for([
		{ status: "added", expected: "diff-added" },
		{ status: "removed", expected: "diff-removed" },
		{ status: "modified", expected: "diff-modified" },
	])(
		"marks a $status file with its diff overlay",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountFile(storedZip({ diffStatus: status }))

			expect(wrapper.get(".diff-overlay").classes()).toContain(expected)
			expect(wrapper.get("a").classes()).toContain("diff-overlay-anchor")
		},
	)

	it("shows no diff overlay on an unchanged file", async ({ expect }) => {
		const wrapper = await mountFile(storedZip({ diffStatus: "unchanged" }))

		expect(wrapper.find(".diff-overlay").exists()).toBe(false)
	})

	it("opens the file picker from the empty state without a type restriction", async ({
		expect,
	}) => {
		const wrapper = await mountFile()
		// a real click on the hidden input bubbles back to the container
		// and would re-enter the handler, so the spy stands in for it
		const click = vi
			.spyOn(fileInput(wrapper), "click")
			.mockImplementation(() => undefined)

		await wrapper.get("[data-node-view-wrapper] > button").trigger("click")

		expect(click).toHaveBeenCalledTimes(1)
		expect(fileInput(wrapper).getAttribute("accept")).toBeNull()
	})

	it("keeps the file picker shut in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountFile()
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > button").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("keeps the file picker shut while a review diff is active", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		const wrapper = await mountFile()
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > button").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("keeps the file picker shut off a document page", async ({ expect }) => {
		useRouteMock.mockReturnValue({ params: {} })
		const wrapper = await mountFile()
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > button").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("keeps the file picker shut while an upload is running", async ({
		expect,
	}) => {
		const wrapper = await mountFile({ uploading: true })
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > button").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("uploads a picked file and turns the block into its card", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			(_call, event) => {
				setResponseHeader(event, "location", "https://cdn.test/stored.zip")

				return uploadedZip()
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({}, updateAttributes)

		await pickFile(wrapper, zipFile())

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.query).toEqual({
			id: "file-1",
			location: "document",
			kind: "file",
		})
		expect(updateAttributes).toHaveBeenCalledTimes(2)
		expect(updateAttributes).toHaveBeenNthCalledWith(1, {
			uploading: true,
			name: "notes.zip",
			size: 6,
		})
		expect(updateAttributes).toHaveBeenNthCalledWith(2, {
			src: matchingString(
				new RegExp(`/api/documents/${DOCUMENT_ID}/files/file-1-notes.zip$`),
			),
			uid: "file-1",
			name: "notes.zip",
			size: 6,
			contentType: "application/zip",
			uploading: false,
		})
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("uploads under a generated id when the node has none", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			(_call, event) => {
				setResponseHeader(event, "location", "https://cdn.test/stored.zip")

				return uploadedZip()
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({ uid: null }, updateAttributes)

		await pickFile(wrapper, zipFile())

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.query.id).toEqual(expect.any(String))
		expect(calls[0]?.query.id).not.toBe("")
	})

	it("does nothing when the file dialog is dismissed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			() => null,
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({}, updateAttributes)

		await pickFile(wrapper)

		expect(calls).toHaveLength(0)
		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("warns that upload is unavailable off a document page", async ({
		expect,
	}) => {
		useRouteMock.mockReturnValue({ params: {} })
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			() => null,
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({}, updateAttributes)

		await pickFile(wrapper, zipFile())

		expect(calls).toHaveLength(0)
		expect(updateAttributes).toHaveBeenCalledTimes(0)
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("returns the block to empty and warns when the upload fails", async ({
		expect,
	}) => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {
			return undefined
		})
		mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			(_call, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({}, updateAttributes)

		await pickFile(wrapper, zipFile())

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(updateAttributes).toHaveBeenNthCalledWith(2, {
			uploading: false,
			name: null,
			size: null,
			contentType: null,
		})
		expect(consoleError).toHaveBeenCalledTimes(1)
	})

	it("leaves a file over the size limit to the storage error handler", async ({
		expect,
	}) => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {
			return undefined
		})
		mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			(_call, event) => {
				setResponseStatus(event, 400)

				return { code: "storage.size_limit_exceeded" }
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountFile({}, updateAttributes)

		await pickFile(wrapper, zipFile())

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(updateAttributes).toHaveBeenNthCalledWith(2, {
			uploading: false,
			name: null,
			size: null,
			contentType: null,
		})
		expect(consoleError).toHaveBeenCalledTimes(0)
	})
})
