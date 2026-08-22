import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { setResponseHeader, setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ImageBlock from "./ImageBlock.vue"
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

function mountImage(
	attrs: Record<string, unknown> = {},
	updateAttributes: (attrs: Record<string, unknown>) => void = () => undefined,
) {
	return mountNodeView(ImageBlock, {
		node: makeNode({ uid: "image-1", ...attrs }),
		updateAttributes: updateAttributes,
	})
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

function pngFile() {
	return new File(["binary"], "shot.png", { type: "image/png" })
}

// the resize maths starts from the rendered image box, which happy-dom
// reports as zero-sized — every resize test states the size it drags from
function sizeImage(wrapper: VueWrapper, width: number, height: number) {
	vi.spyOn(wrapper.get("img").element, "getBoundingClientRect").mockReturnValue(
		{ width: width, height: height } as DOMRect,
	)
}

async function startDrag(wrapper: VueWrapper) {
	await wrapper
		.get("[aria-label='Resize image']")
		.trigger("mousedown", { clientX: 100, clientY: 200 })
}

function dragBy(dx: number, dy: number) {
	window.dispatchEvent(
		new MouseEvent("mousemove", { clientX: 100 + dx, clientY: 200 + dy }),
	)
}

// the route mock, the editable flag, the editor store and the mocked
// toast module are all shared by the whole file, so these tests cannot
// interleave
describe("<ImageBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		vi.mocked(toast.custom).mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		onDocumentPage()
	})

	afterEach(disposeMockEndpoints)

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountImage()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("image-1")
		expect(root.attributes("data-uid")).toBe("image-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountImage({
			nodeCommentId: "comment-1",
			diffStatus: "modified",
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("invites the reader to upload while the block is empty", async ({
		expect,
	}) => {
		const wrapper = await mountImage()

		expect(wrapper.text()).toBe(t("editor.image.description"))
		expect(wrapper.find("img").exists()).toBe(false)
	})

	it("reports an empty block as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountImage()

		expect(wrapper.text()).toBe(t("editor.image.empty"))
	})

	it("reports progress while an upload is running", async ({ expect }) => {
		const wrapper = await mountImage({ uploading: true })

		expect(wrapper.text()).toBe(t("editor.image.uploading"))
	})

	it("reports upload as unavailable off a document page", async ({
		expect,
	}) => {
		useRouteMock.mockReturnValue({ params: {} })

		const wrapper = await mountImage()

		expect(wrapper.text()).toBe(t("editor.image.upload-unavailable"))
	})

	it("reports upload as unavailable for an unrecognisable document slug", async ({
		expect,
	}) => {
		useRouteMock.mockReturnValue({ params: { documentSlug: "not-a-doc" } })

		const wrapper = await mountImage()

		expect(wrapper.text()).toBe(t("editor.image.upload-unavailable"))
	})

	it("shows the uploaded image with its alt text and title", async ({
		expect,
	}) => {
		const wrapper = await mountImage({
			src: "https://cdn.test/a.png",
			alt: "A diagram",
			title: "Architecture",
		})

		const image = wrapper.get("img")

		expect(image.attributes("src")).toBe("https://cdn.test/a.png")
		expect(image.attributes("alt")).toBe("A diagram")
		expect(image.attributes("title")).toBe("Architecture")
	})

	it("leaves the alt text and title empty when the node has none", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })

		const image = wrapper.get("img")

		expect(image.attributes("alt")).toBe("")
		expect(image.attributes("title")).toBe("")
	})

	it("sizes the image to the width the node stores", async ({ expect }) => {
		const wrapper = await mountImage({
			src: "https://cdn.test/a.png",
			width: 240,
		})

		expect(wrapper.get("img").attributes("style")).toContain("width: 240px")
	})

	it("reads a stored width written as a string", async ({ expect }) => {
		const wrapper = await mountImage({
			src: "https://cdn.test/a.png",
			width: "240",
		})

		expect(wrapper.get("img").attributes("style")).toContain("width: 240px")
	})

	it("falls back to a thumbnail width when the node stores none", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })

		expect(wrapper.get("img").attributes("style")).toBeUndefined()
		expect(wrapper.get("img").classes()).toContain("w-32")
	})

	it("ignores a stored width that is not a number", async ({ expect }) => {
		const wrapper = await mountImage({
			src: "https://cdn.test/a.png",
			width: {},
		})

		expect(wrapper.get("img").classes()).toContain("w-32")
	})

	it.for([
		{ status: "added", expected: "diff-added" },
		{ status: "removed", expected: "diff-removed" },
		{ status: "modified", expected: "diff-modified" },
	])(
		"marks a $status image with its diff overlay",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountImage({
				src: "https://cdn.test/a.png",
				diffStatus: status,
			})

			expect(wrapper.get(".diff-overlay").classes()).toContain(expected)
		},
	)

	it("shows no diff overlay on an unchanged image", async ({ expect }) => {
		const wrapper = await mountImage({
			src: "https://cdn.test/a.png",
			diffStatus: "unchanged",
		})

		expect(wrapper.find(".diff-overlay").exists()).toBe(false)
	})

	it("offers the resize handle on an image while editing", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })

		expect(wrapper.find("[aria-label='Resize image']").exists()).toBe(true)
	})

	it("hides the resize handle in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })

		expect(wrapper.find("[aria-label='Resize image']").exists()).toBe(false)
	})

	it("scales the image while the handle is dragged", async ({ expect }) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })
		sizeImage(wrapper, 200, 100)

		await startDrag(wrapper)
		dragBy(100, 0)
		await nextTick()

		expect(wrapper.get("img").attributes("style")).toContain("width: 300px")
	})

	it("scales by whichever axis the pointer moved further along", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })
		sizeImage(wrapper, 200, 100)

		await startDrag(wrapper)
		dragBy(20, 100)
		await nextTick()

		expect(wrapper.get("img").attributes("style")).toContain("width: 400px")
	})

	it("keeps the image at its minimum width when dragged past it", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ src: "https://cdn.test/a.png" })
		sizeImage(wrapper, 200, 100)

		await startDrag(wrapper)
		dragBy(-500, -500)
		await nextTick()

		expect(wrapper.get("img").attributes("style")).toContain("width: 128px")
	})

	it("stores the new width once the drag ends", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountImage(
			{ src: "https://cdn.test/a.png" },
			updateAttributes,
		)
		sizeImage(wrapper, 200, 100)

		await startDrag(wrapper)
		dragBy(100, 0)
		window.dispatchEvent(new MouseEvent("mouseup"))
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({ width: 300 })
	})

	it("stores nothing when the handle is released without a drag", async ({
		expect,
	}) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountImage(
			{ src: "https://cdn.test/a.png" },
			updateAttributes,
		)
		sizeImage(wrapper, 200, 100)

		await startDrag(wrapper)
		window.dispatchEvent(new MouseEvent("mouseup"))
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("does not resize an image it cannot measure", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountImage(
			{ src: "https://cdn.test/a.png" },
			updateAttributes,
		)

		await startDrag(wrapper)
		dragBy(100, 0)
		window.dispatchEvent(new MouseEvent("mouseup"))
		await nextTick()

		expect(wrapper.get("img").attributes("style")).toBeUndefined()
		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stops resizing once the block is unmounted", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountImage(
			{ src: "https://cdn.test/a.png" },
			updateAttributes,
		)
		sizeImage(wrapper, 200, 100)
		await startDrag(wrapper)

		wrapper.unmount()
		dragBy(100, 0)
		window.dispatchEvent(new MouseEvent("mouseup"))

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("opens the file picker from the empty state", async ({ expect }) => {
		const wrapper = await mountImage()
		// a real click on the hidden input bubbles back to the container
		// and would re-enter the handler, so the spy stands in for it
		const click = vi
			.spyOn(fileInput(wrapper), "click")
			.mockImplementation(() => undefined)

		await wrapper.get("[data-node-view-wrapper] > div").trigger("click")

		expect(click).toHaveBeenCalledTimes(1)
	})

	it("keeps the file picker shut in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountImage()
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > div").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("keeps the file picker shut while an upload is running", async ({
		expect,
	}) => {
		const wrapper = await mountImage({ uploading: true })
		const click = vi.spyOn(fileInput(wrapper), "click")

		await wrapper.get("[data-node-view-wrapper] > div").trigger("click")

		expect(click).toHaveBeenCalledTimes(0)
	})

	it("uploads a picked image and points the block at it", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/files`,
			(_call, event) => {
				setResponseHeader(event, "location", "https://cdn.test/stored.png")

				return null
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountImage({}, updateAttributes)

		await pickFile(wrapper, pngFile())

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.query).toEqual({ id: "image-1", location: "document" })
		expect(updateAttributes).toHaveBeenCalledTimes(2)
		expect(updateAttributes).toHaveBeenNthCalledWith(1, { uploading: true })
		expect(updateAttributes).toHaveBeenNthCalledWith(2, {
			src: matchingString(
				new RegExp(`/api/documents/${DOCUMENT_ID}/files/image-1$`),
			),
			uid: "image-1",
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
				setResponseHeader(event, "location", "https://cdn.test/stored.png")

				return null
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountImage({ uid: null }, updateAttributes)

		await pickFile(wrapper, pngFile())

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
		const wrapper = await mountImage({}, updateAttributes)

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
		const wrapper = await mountImage({}, updateAttributes)

		await pickFile(wrapper, pngFile())

		expect(calls).toHaveLength(0)
		expect(updateAttributes).toHaveBeenCalledTimes(0)
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("warns and clears the upload flag when the upload fails", async ({
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
		const wrapper = await mountImage({}, updateAttributes)

		await pickFile(wrapper, pngFile())

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(updateAttributes).toHaveBeenNthCalledWith(2, { uploading: false })
		expect(consoleError).toHaveBeenCalledTimes(1)
	})

	it("leaves a rejected file type to the storage error handler", async ({
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

				return { code: "storage.invalid_content_type" }
			},
		)
		const updateAttributes = vi.fn()
		const wrapper = await mountImage({}, updateAttributes)

		await pickFile(wrapper, pngFile())

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(updateAttributes).toHaveBeenNthCalledWith(2, { uploading: false })
		expect(consoleError).toHaveBeenCalledTimes(0)
	})

	it("warns when the upload response carries no location", async ({
		expect,
	}) => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {
			return undefined
		})
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/files`, () => null)
		const updateAttributes = vi.fn()
		const wrapper = await mountImage({}, updateAttributes)

		await pickFile(wrapper, pngFile())

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(consoleError).toHaveBeenCalledTimes(1)
	})
})
