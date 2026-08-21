import type { Editor as TiptapEditor, JSONContent } from "@tiptap/core"
import { Editor, getSchema } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode, NodeType } from "@tiptap/pm/model"
import { beforeEach, describe, it, vi } from "vitest"
import { DocumentFileLocation } from "~/utils/api/document"
import { IMAGE_BLOCK_NAME } from "../node-names"
import UniqueID from "../../tiptap-utils/unique-id"
import { createImageFileHandler, ImageBlock } from "."
import type { ImageFileHandlerOptions } from "."
import {
	nodeType,
	paragraph,
	parseAttributes,
} from "~/components/editor/test-helpers"

const uploadDocumentFile = vi.fn()
const showToastMessage = vi.fn()

vi.mock("~/components/toast", () => ({
	showToastMessage: (...args: unknown[]) => {
		showToastMessage(...args)
	},
}))

// the composable is pulled in through a nuxt auto-import, so the module
// itself is the only seam: the real one reaches for the nuxt app
vi.mock("~/composables/api/useDocumentFileAPI", () => ({
	default: () => ({ uploadDocumentFile: { mutateAsync: uploadDocumentFile } }),
	buildDocumentFileSrc: (documentId: string, blockId: string) =>
		`/files/${documentId}/${blockId}`,
}))

const extensions = [
	Document,
	Text,
	Paragraph,
	Blockquote,
	ImageBlock,
	UniqueID.configure({
		types: [Paragraph.name, Blockquote.name, ImageBlock.name],
		attributeName: "uid",
	}),
]

const schema = getSchema(extensions)

function imageType(): NodeType {
	return nodeType(schema, IMAGE_BLOCK_NAME)
}

// the file handler options are typed as returning void, but the paste
// and drop callbacks answer whether they handled the event
interface FileCallbacks {
	allowedMimeTypes: string[]
	onPaste: (editor: TiptapEditor, files: File[]) => boolean
	onDrop: (editor: TiptapEditor, files: File[], pos: number) => boolean
}

function fileCallbacks(options: ImageFileHandlerOptions): FileCallbacks {
	return createImageFileHandler(options).options as unknown as FileCallbacks
}

function makeEditor(content: JSONContent[], withImages = true): Editor {
	return new Editor({
		extensions: withImages
			? extensions
			: [Document, Text, Paragraph, Blockquote],
		content: { type: "doc", content },
	})
}

function pngFile(name: string): File {
	return new File(["binary"], name, { type: "image/png" })
}

// compresses the document into one "type" or "type:src" entry per
// top-level node, plus the pending-upload flag for images
function shape(doc: PMNode): string[] {
	const entries: string[] = []

	doc.forEach((child) => {
		if (child.type.name !== IMAGE_BLOCK_NAME) {
			entries.push(child.type.name)
			return
		}

		entries.push(
			`${child.type.name}:${String(child.attrs.src)}:${String(child.attrs.uploading)}`,
		)
	})

	return entries
}

function imageUid(doc: PMNode): string {
	let uid = ""

	doc.descendants((node) => {
		if (node.type.name === IMAGE_BLOCK_NAME) {
			uid = node.attrs.uid as string
			return false
		}

		return true
	})

	if (!uid) {
		throw new Error("no image block in the test document")
	}

	return uid
}

// the upload chain is pure microtasks, so one macrotask boundary is
// enough to settle it — no timers involved
function flushUpload(): Promise<void> {
	return new Promise((resolve) => setImmediate(resolve))
}

describe("ImageBlock", () => {
	it("defines an unselectable, undraggable atomic block", ({ expect }) => {
		expect(imageType().spec).toMatchObject({
			group: "block",
			atom: true,
			draggable: false,
			selectable: false,
		})
	})

	it("matches only image block markers when parsing html", ({ expect }) => {
		expect(imageType().spec.parseDOM?.[0]?.tag).toBe(
			`img[data-type="image-block"]`,
		)
	})

	it("defaults every attribute to null except the upload flag", ({
		expect,
	}) => {
		expect(imageType().create().attrs).toEqual({
			uid: null,
			src: null,
			alt: null,
			title: null,
			width: null,
			uploading: false,
		})
	})

	it("renders the image attributes onto an img element", ({ expect }) => {
		const node = imageType().create({
			src: "/files/d1/b1",
			alt: "a photo",
			title: "Photo",
			width: 320,
		})

		expect(imageType().spec.toDOM?.(node)).toEqual([
			"img",
			{
				src: "/files/d1/b1",
				alt: "a photo",
				title: "Photo",
				width: 320,
				"data-type": "image-block",
			},
		])
	})

	it("keeps the transient upload flag out of the html", ({ expect }) => {
		const node = imageType().create({ src: "/files/d1/b1", uploading: true })

		expect(imageType().spec.toDOM?.(node)).toEqual([
			"img",
			{
				src: "/files/d1/b1",
				alt: null,
				title: null,
				"data-type": "image-block",
			},
		])
	})

	// the width is the only attribute with a renderer of its own; the
	// others pass through as null and the serializer drops them
	it("omits the width when it is unset", ({ expect }) => {
		const node = imageType().create({ src: "/files/d1/b1" })

		expect(imageType().spec.toDOM?.(node)).toEqual([
			"img",
			{
				src: "/files/d1/b1",
				alt: null,
				title: null,
				"data-type": "image-block",
			},
		])
	})

	it("parses the width attribute into a number", ({ expect }) => {
		expect(
			parseAttributes(imageType(), {
				src: "/files/d1/b1",
				alt: "a",
				width: "320",
			}),
		).toEqual({ src: "/files/d1/b1", alt: "a", width: 320 })
	})

	// tiptap drops null results from an attribute parser, so the node
	// falls back to the attribute defaults
	it("parses an img without a width into no width attribute", ({ expect }) => {
		expect(parseAttributes(imageType(), { src: "/files/d1/b1" })).toEqual({
			src: "/files/d1/b1",
		})
	})

	it("round-trips a node through render and parse", ({ expect }) => {
		const node = imageType().create({
			src: "/files/d1/b1",
			alt: "a photo",
			title: "Photo",
			width: 320,
		})
		const rendered = imageType().spec.toDOM?.(node) as [
			string,
			Record<string, string>,
		]

		expect(parseAttributes(imageType(), rendered[1])).toEqual({
			src: "/files/d1/b1",
			alt: "a photo",
			title: "Photo",
			width: 320,
		})
	})
})

// the upload composable and the toast module are file-level module
// mocks, so their call accounting cannot be isolated across
// interleaving tests
describe("createImageFileHandler", { concurrent: false }, () => {
	beforeEach(() => {
		uploadDocumentFile.mockReset()
		uploadDocumentFile.mockResolvedValue(undefined)
		showToastMessage.mockReset()
		vi.stubGlobal("$t", (path: string) => path)
	})

	it("accepts only the supported image mime types", ({ expect }) => {
		expect(fileCallbacks({ documentId: "d1" }).allowedMimeTypes).toEqual([
			"image/png",
			"image/jpeg",
			"image/gif",
			"image/webp",
		])
	})

	describe("onPaste", () => {
		it("inserts an uploading image after the current block", ({ expect }) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])
			const file = pngFile("a.png")
			editor.commands.setTextSelection(2)

			const handled = fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				file,
			])

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"imageBlock:null:true",
				"paragraph",
			])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(1)
			expect(uploadDocumentFile).toHaveBeenCalledWith({
				documentId: "d1",
				id: imageUid(editor.state.doc),
				loc: DocumentFileLocation.Document,
				file,
			})
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it("points the image at the uploaded file once the upload finishes", async ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")])
			const uid = imageUid(editor.state.doc)

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				`imageBlock:/files/d1/${uid}:false`,
			])
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it("clears the upload flag and reports the failure", async ({ expect }) => {
			const error = new Error("boom")
			uploadDocumentFile.mockRejectedValue(error)
			const logged = vi.spyOn(console, "error").mockImplementation(() => {
				return undefined
			})
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")])

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"imageBlock:null:false",
			])
			expect(logged).toHaveBeenCalledWith("Failed to upload image:", error)
			expect(showToastMessage).toHaveBeenCalledTimes(1)
			expect(showToastMessage).toHaveBeenCalledWith(
				"error",
				"editor.image.errors.upload-failed",
			)
		})

		it("inserts one image per pasted file", ({ expect }) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)

			fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				pngFile("a.png"),
				pngFile("b.png"),
			])

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"imageBlock:null:true",
				"imageBlock:null:true",
			])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(2)
		})

		it.for([
			{ name: "ignores a paste without a document id", documentId: undefined },
			{ name: "ignores a paste with a null document id", documentId: null },
		])("$name", ({ documentId }, { expect }) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)

			expect(
				fileCallbacks({ documentId }).onPaste(editor, [pngFile("a.png")]),
			).toBe(false)
			expect(shape(editor.state.doc)).toEqual(["paragraph"])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})

		it("ignores a paste nested deeper than the root level", ({ expect }) => {
			const editor = makeEditor([
				{
					type: "blockquote",
					content: [{ type: "blockquote", content: [paragraph("deep")] }],
				},
			])
			editor.commands.setTextSelection(4)

			expect(
				fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")]),
			).toBe(false)
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})

		it("inserts nothing when the schema has no image block", ({ expect }) => {
			const editor = makeEditor([paragraph("one")], false)
			editor.commands.setTextSelection(2)

			expect(
				fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")]),
			).toBe(true)
			expect(shape(editor.state.doc)).toEqual(["paragraph"])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})

		it("leaves the document alone when the image is gone before the upload finishes", async ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")])
			editor.commands.setContent({
				type: "doc",
				content: [paragraph("replaced")],
			})

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual(["paragraph"])
			expect(editor.state.doc.textContent).toBe("replaced")
		})
	})

	describe("onDrop", () => {
		it("inserts the image after the block holding the drop position", ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])

			const handled = fileCallbacks({ documentId: "d1" }).onDrop(
				editor,
				[pngFile("a.png")],
				7,
			)

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"paragraph",
				"imageBlock:null:true",
			])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(1)
		})

		it("inserts the image at a drop position between two blocks", ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])

			const handled = fileCallbacks({ documentId: "d1" }).onDrop(
				editor,
				[pngFile("a.png")],
				5,
			)

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"imageBlock:null:true",
				"paragraph",
			])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(1)
		})

		it("ignores a drop without a document id", ({ expect }) => {
			const editor = makeEditor([paragraph("one")])

			expect(fileCallbacks({}).onDrop(editor, [pngFile("a.png")], 2)).toBe(
				false,
			)
			expect(shape(editor.state.doc)).toEqual(["paragraph"])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})

		it("ignores a drop nested deeper than the root level", ({ expect }) => {
			const editor = makeEditor([
				{
					type: "blockquote",
					content: [{ type: "blockquote", content: [paragraph("deep")] }],
				},
			])

			expect(
				fileCallbacks({ documentId: "d1" }).onDrop(
					editor,
					[pngFile("a.png")],
					4,
				),
			).toBe(false)
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})
	})
})
