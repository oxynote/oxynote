import type { Editor as TiptapEditor, JSONContent } from "@tiptap/core"
import { Editor, getSchema } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import { beforeEach, describe, it, vi } from "vitest"
import { DocumentFileKind, DocumentFileLocation } from "~/utils/api/document"
import { FILE_BLOCK_NAME, IMAGE_BLOCK_NAME } from "./node-names"
import UniqueID from "../tiptap-utils/unique-id"
import { ImageBlock } from "./image"
import { FileBlock } from "./file"
import { createUploadFileHandler } from "./upload-handler"
import type { UploadFileHandlerOptions } from "./upload-handler"
import { paragraph } from "~/components/editor/test-helpers"

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
	buildDocumentFileSrc: (documentId: string, blockId: string, name: string) =>
		`/files/${documentId}/${blockId}-${name}`,
}))

const extensions = [
	Document,
	Text,
	Paragraph,
	Blockquote,
	ImageBlock,
	FileBlock,
	UniqueID.configure({
		types: [Paragraph.name, Blockquote.name, ImageBlock.name, FileBlock.name],
		attributeName: "uid",
	}),
]

// the schema is built once so tiptap's per-extension warnings fire once
getSchema(extensions)

// the file handler options are typed as returning void, but the paste
// and drop callbacks answer whether they handled the event
interface FileCallbacks {
	allowedMimeTypes?: string[]
	onPaste: (editor: TiptapEditor, files: File[]) => boolean
	onDrop: (editor: TiptapEditor, files: File[], pos: number) => boolean
}

function fileCallbacks(options: UploadFileHandlerOptions): FileCallbacks {
	return createUploadFileHandler(options).options as unknown as FileCallbacks
}

function makeEditor(content: JSONContent[], withBlocks = true): Editor {
	return new Editor({
		extensions: withBlocks
			? extensions
			: [Document, Text, Paragraph, Blockquote],
		content: { type: "doc", content },
	})
}

function pngFile(name: string): File {
	return new File(["binary"], name, { type: "image/png" })
}

function zipFile(name: string): File {
	return new File(["zipped"], name, { type: "application/zip" })
}

// what the server answers a successful upload with
function uploaded(name: string) {
	return { name, size: 6, contentType: "application/zip" }
}

// compresses the document into one entry per top-level node: the type
// name, then for the upload blocks the src and the pending-upload flag
function shape(doc: PMNode): string[] {
	const entries: string[] = []

	doc.forEach((child) => {
		if (
			child.type.name !== IMAGE_BLOCK_NAME &&
			child.type.name !== FILE_BLOCK_NAME
		) {
			entries.push(child.type.name)
			return
		}

		entries.push(
			`${child.type.name}:${String(child.attrs.src)}:${String(child.attrs.uploading)}`,
		)
	})

	return entries
}

// typescript does not track assignments made inside the descendants
// callback, so the match is collected rather than assigned
function blockAttrs(doc: PMNode, typeName: string): Record<string, unknown> {
	const found: Record<string, unknown>[] = []

	doc.descendants((node) => {
		if (node.type.name === typeName) {
			found.push(node.attrs)
			return false
		}

		return true
	})

	const attrs = found[0]

	if (!attrs) {
		throw new Error(`no ${typeName} in the test document`)
	}

	return attrs
}

function blockUid(doc: PMNode, typeName: string): string {
	return blockAttrs(doc, typeName).uid as string
}

// the upload chain is pure microtasks, so one macrotask boundary is
// enough to settle it — no timers involved
function flushUpload(): Promise<void> {
	return new Promise((resolve) => setImmediate(resolve))
}

// the upload composable and the toast module are file-level module
// mocks, so their call accounting cannot be isolated across
// interleaving tests
describe("createUploadFileHandler", { concurrent: false }, () => {
	beforeEach(() => {
		uploadDocumentFile.mockReset()
		uploadDocumentFile.mockResolvedValue(uploaded("a.zip"))
		showToastMessage.mockReset()
		vi.stubGlobal("$t", (path: string) => path)
	})

	it("accepts every file type", ({ expect }) => {
		expect(fileCallbacks({ documentId: "d1" }).allowedMimeTypes).toBeUndefined()
	})

	describe("onPaste", () => {
		it("inserts an uploading image after the current block for an image", ({
			expect,
		}) => {
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
				id: blockUid(editor.state.doc, IMAGE_BLOCK_NAME),
				loc: DocumentFileLocation.Document,
				kind: DocumentFileKind.Image,
				file,
			})
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it("inserts an uploading file block after the current block for a non-image", ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])
			const file = zipFile("notes.zip")
			editor.commands.setTextSelection(2)

			const handled = fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				file,
			])

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"fileBlock:null:true",
				"paragraph",
			])
			// the card shows the file's own name and size while the upload runs
			expect(blockAttrs(editor.state.doc, FILE_BLOCK_NAME)).toMatchObject({
				name: "notes.zip",
				size: 6,
				contentType: null,
			})
			expect(uploadDocumentFile).toHaveBeenCalledTimes(1)
			expect(uploadDocumentFile).toHaveBeenCalledWith({
				documentId: "d1",
				id: blockUid(editor.state.doc, FILE_BLOCK_NAME),
				loc: DocumentFileLocation.Document,
				kind: DocumentFileKind.File,
				file,
			})
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it.for([
			{ name: "png", type: "image/png", expected: IMAGE_BLOCK_NAME },
			{ name: "jpeg", type: "image/jpeg", expected: IMAGE_BLOCK_NAME },
			{ name: "gif", type: "image/gif", expected: FILE_BLOCK_NAME },
			{ name: "webp", type: "image/webp", expected: IMAGE_BLOCK_NAME },
			{ name: "svg", type: "image/svg+xml", expected: FILE_BLOCK_NAME },
			{ name: "pdf", type: "application/pdf", expected: FILE_BLOCK_NAME },
			{ name: "untyped", type: "", expected: FILE_BLOCK_NAME },
		])(
			"routes a $name file to the $expected",
			({ type, expected }, { expect }) => {
				const editor = makeEditor([paragraph("one")])
				editor.commands.setTextSelection(2)

				fileCallbacks({ documentId: "d1" }).onPaste(editor, [
					new File(["x"], "thing", { type }),
				])

				expect(shape(editor.state.doc)).toEqual([
					"paragraph",
					`${expected}:null:true`,
				])
			},
		)

		it("points the image at the uploaded file once the upload finishes", async ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [pngFile("a.png")])
			const uid = blockUid(editor.state.doc, IMAGE_BLOCK_NAME)

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				`imageBlock:/files/d1/${uid}-a.zip:false`,
			])
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it("records what the server stored on the file block once the upload finishes", async ({
			expect,
		}) => {
			uploadDocumentFile.mockResolvedValue({
				name: "renamed.zip",
				size: 2048,
				contentType: "application/zip",
			})
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				zipFile("notes.zip"),
			])
			const uid = blockUid(editor.state.doc, FILE_BLOCK_NAME)

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				`fileBlock:/files/d1/${uid}-renamed.zip:false`,
			])
			expect(blockAttrs(editor.state.doc, FILE_BLOCK_NAME)).toMatchObject({
				name: "renamed.zip",
				size: 2048,
				contentType: "application/zip",
			})
			expect(showToastMessage).toHaveBeenCalledTimes(0)
		})

		it("clears the upload flag and reports a failed image upload", async ({
			expect,
		}) => {
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

		it("clears the upload flag and reports a failed file upload", async ({
			expect,
		}) => {
			const error = new Error("boom")
			uploadDocumentFile.mockRejectedValue(error)
			const logged = vi.spyOn(console, "error").mockImplementation(() => {
				return undefined
			})
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				zipFile("notes.zip"),
			])

			await flushUpload()

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"fileBlock:null:false",
			])
			expect(logged).toHaveBeenCalledWith("Failed to upload file:", error)
			expect(showToastMessage).toHaveBeenCalledTimes(1)
			expect(showToastMessage).toHaveBeenCalledWith(
				"error",
				"editor.file.errors.upload-failed",
			)
		})

		// every file lands right after the block the selection sits in, so
		// the later file ends up above the earlier one
		it("inserts one block per pasted file", ({ expect }) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)

			fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				pngFile("a.png"),
				zipFile("b.zip"),
			])

			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"fileBlock:null:true",
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

		it("inserts nothing when the schema has no upload blocks", ({ expect }) => {
			const editor = makeEditor([paragraph("one")], false)
			editor.commands.setTextSelection(2)

			expect(
				fileCallbacks({ documentId: "d1" }).onPaste(editor, [
					pngFile("a.png"),
					zipFile("b.zip"),
				]),
			).toBe(true)
			expect(shape(editor.state.doc)).toEqual(["paragraph"])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})

		it("leaves the document alone when the block is gone before the upload finishes", async ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one")])
			editor.commands.setTextSelection(2)
			fileCallbacks({ documentId: "d1" }).onPaste(editor, [
				zipFile("notes.zip"),
			])
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

		it("inserts a file block after the block holding the drop position", ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])

			const handled = fileCallbacks({ documentId: "d1" }).onDrop(
				editor,
				[zipFile("notes.zip")],
				2,
			)

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"fileBlock:null:true",
				"paragraph",
			])
			expect(uploadDocumentFile).toHaveBeenCalledTimes(1)
			expect(uploadDocumentFile).toHaveBeenCalledWith(
				expect.objectContaining({ kind: DocumentFileKind.File }),
			)
		})

		it("inserts the block at a drop position between two blocks", ({
			expect,
		}) => {
			const editor = makeEditor([paragraph("one"), paragraph("two")])

			const handled = fileCallbacks({ documentId: "d1" }).onDrop(
				editor,
				[zipFile("notes.zip")],
				5,
			)

			expect(handled).toBe(true)
			expect(shape(editor.state.doc)).toEqual([
				"paragraph",
				"fileBlock:null:true",
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
					[zipFile("notes.zip")],
					4,
				),
			).toBe(false)
			expect(uploadDocumentFile).toHaveBeenCalledTimes(0)
		})
	})
})
