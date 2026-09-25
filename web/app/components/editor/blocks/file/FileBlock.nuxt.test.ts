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

// where core serves the stored attachment; the test runtime config leaves
// the api base empty, so the address is origin-relative
const STORED_SRC = `/api/documents/${DOCUMENT_ID}/files/file-1-notes.zip`

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
		src: STORED_SRC,
		name: "notes.zip",
		size: 2_516_582,
		contentType: "application/zip",
		...attrs,
	}
}

// the span wrapping the kind icon
function iconBadge(wrapper: VueWrapper): Element {
	const badge = wrapper.get("a .iconify").element.parentElement

	if (!badge) {
		throw new Error("no icon badge")
	}

	return badge
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
			expected: "mingcute:pdf-fill",
		},
		{
			name: "clip.mp4",
			contentType: "video/mp4",
			expected: "mingcute:video-fill",
		},
		{
			name: "notes.zip",
			contentType: "application/zip",
			expected: "mingcute:file-zip-fill",
		},
		{
			name: "main.go",
			contentType: "text/plain",
			expected: "mingcute:file-code-fill",
		},
		{
			name: "budget.ods",
			contentType: "application/vnd.oasis.opendocument.spreadsheet",
			expected: "mingcute:xls-fill",
		},
		{
			name: "thing.bin",
			contentType: "application/octet-stream",
			expected: "mingcute:file-fill",
		},
	])(
		"shows the $expected icon for $name",
		async ({ name, contentType, expected }, { expect }) => {
			const wrapper = await mountFile(storedZip({ name, contentType }))

			expect(wrapper.get("a .iconify").classes()).toContain(`i-${expected}`)
		},
	)

	// the two modes are pure css, which happy-dom loads none of, so the
	// custom properties the variants read are the only trace a test can
	// follow
	it("tints the icon badge with the kind's selectable colour", async ({
		expect,
	}) => {
		const wrapper = await mountFile(storedZip())
		const style = iconBadge(wrapper).getAttribute("style") ?? ""

		expect(style).toContain(
			"--kind-bg: color-mix(in srgb, var(--selectable-color-4) 13%, transparent)",
		)
		expect(style).toContain(
			"--kind-fg: color-mix(in srgb, var(--selectable-color-4) 80%, black)",
		)
		expect(style).toContain(
			"--kind-dark-bg: color-mix(in srgb, var(--selectable-color-4) 18%, transparent)",
		)
		expect(style).toContain(
			"--kind-dark-fg: color-mix(in srgb, var(--selectable-color-4) 60%, white)",
		)
	})

	it("leaves the icon badge neutral for an unknown kind", async ({
		expect,
	}) => {
		const wrapper = await mountFile(
			storedZip({ name: "thing.bin", contentType: "application/octet-stream" }),
		)
		const badge = iconBadge(wrapper)

		expect(badge.getAttribute("style")).toBeNull()
		expect(badge.classList.contains("bg-muted")).toBe(true)
	})

	it.for([
		{ name: "a PDF", contentType: "application/pdf" },
		{ name: "a video", contentType: "video/mp4" },
		{ name: "an audio file", contentType: "audio/mpeg" },
		{ name: "an image", contentType: "image/png" },
		{ name: "a plain text file", contentType: "text/plain; charset=utf-8" },
	])("opens $name in a new tab", async ({ contentType }, { expect }) => {
		const wrapper = await mountFile(storedZip({ contentType }))

		const card = wrapper.get("a")

		expect(card.attributes("href")).toBe(STORED_SRC)
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

		expect(card.attributes("href")).toBe(STORED_SRC)
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
		{
			name: "an attachment path on another host",
			src: `https://files.example.net/api/documents/${DOCUMENT_ID}/files/file-1-q3.exe`,
		},
		{
			name: "a path climbing out of the attachment",
			src: `/api/documents/${DOCUMENT_ID}/files/../../../users/me`,
		},
	])("treats $name as no file", async ({ src }, { expect }) => {
		const wrapper = await mountFile(storedZip({ src }))

		expect(wrapper.find("a").exists()).toBe(false)
		expect(wrapper.find("input[type='file']").exists()).toBe(true)
	})

	it("keeps the card a link in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFile(storedZip())

		expect(wrapper.get("a").attributes("href")).toBe(STORED_SRC)
	})

	it.for([
		{ status: "added", expected: "diff-added" },
		{ status: "removed", expected: "diff-removed" },
	])(
		"marks a $status file with its diff overlay",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountFile(storedZip({ diffStatus: status }))

			expect(wrapper.get(".diff-overlay").classes()).toContain(expected)
			expect(wrapper.get("a").classes()).toContain("diff-overlay-anchor")
		},
	)

	it("counts a modified file's changes instead of tinting it", async ({
		expect,
	}) => {
		const wrapper = await mountFile(
			storedZip({
				diffStatus: "modified",
				oldNode: { attrs: storedZip({ uid: "file-1", size: 1 }) },
			}),
		)

		expect(wrapper.find(".diff-overlay").exists()).toBe(false)
		expect(wrapper.text()).toContain(
			t("editor.diff-change-marker.label", { removed: 1, added: 1 }),
		)
	})

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
		// the upload carries an id of its own, never the block's uid, so a
		// replacement leaves the previous object in place
		const fileId = String(calls[0]?.query.id)
		expect(fileId).toMatch(/^[A-Za-z0-9_-]{21}$/)
		expect(calls[0]?.query).toEqual({
			id: fileId,
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
				new RegExp(`/api/documents/${DOCUMENT_ID}/files/${fileId}-notes.zip$`),
			),
			name: "notes.zip",
			size: 6,
			contentType: "application/zip",
			uploading: false,
		})
		expect(toast.custom).toHaveBeenCalledTimes(0)
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
