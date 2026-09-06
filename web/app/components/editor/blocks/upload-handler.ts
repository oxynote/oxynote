import type { Editor } from "@tiptap/core"
import { FileHandler } from "@tiptap/extension-file-handler"
import { nanoid } from "nanoid"
import { showToastMessage } from "~/components/toast"
import { FILE_BLOCK_NAME, IMAGE_BLOCK_NAME } from "./node-names"

export interface UploadFileHandlerOptions {
	documentId?: string | null | undefined
}

// the image types the image block renders and the server admits as
// images; anything else dropped or pasted becomes a file attachment
const IMAGE_MIME_TYPES = new Set(["image/png", "image/jpeg", "image/webp"])

// one handler routes every dropped or pasted file by its type: tiptap
// registers the handler under its extension name, so two FileHandler
// instances would collide
export function createUploadFileHandler(options: UploadFileHandlerOptions) {
	return FileHandler.configure({
		onPaste: (editor, files) => {
			if (!isRootInsertPosition(editor) || !options.documentId) {
				return false
			}

			for (const file of files) {
				insertBlockWithUpload(editor, file, options.documentId, undefined)
			}

			return true
		},
		onDrop: (editor, files, pos) => {
			if (!isRootInsertPosition(editor, pos) || !options.documentId) {
				return false
			}

			for (const file of files) {
				insertBlockWithUpload(editor, file, options.documentId, pos)
			}

			return true
		},
	})
}

function isRootInsertPosition(editor: Editor, pos?: number): boolean {
	const insertPos = pos ?? editor.state.selection.anchor
	const $pos = editor.state.doc.resolve(insertPos)

	return $pos.depth <= 2
}

function insertBlockWithUpload(
	editor: Editor,
	file: File,
	documentId: string,
	pos?: number,
) {
	const isImage = IMAGE_MIME_TYPES.has(file.type)
	const nodeName = isImage ? IMAGE_BLOCK_NAME : FILE_BLOCK_NAME
	const blockId = nanoid()
	const { state, view } = editor
	const { schema } = state

	const nodeType = schema.nodes[nodeName]
	if (!nodeType) {
		return
	}

	const insertPos = pos ?? state.selection.anchor
	const $pos = state.doc.resolve(insertPos)

	// insert the block after the current one. A depth of 0 means the
	// position already sits between top-level blocks, where there is no
	// enclosing block to insert after.
	const blockPos = $pos.depth === 0 ? insertPos : $pos.after()

	const tr = state.tr
	const attrs: Record<string, unknown> = { uid: blockId, uploading: true }

	if (!isImage) {
		attrs.name = file.name
		attrs.size = file.size
	}

	tr.insert(blockPos, nodeType.create(attrs))

	view.dispatch(tr)

	uploadFile(documentId, blockId, isImage, file)
		.then((uploaded) => {
			const src = buildDocumentFileSrc(documentId, blockId, uploaded.name)

			updateAttrsByUid(
				editor,
				nodeName,
				blockId,
				isImage
					? { src, uploading: false }
					: {
							src,
							name: uploaded.name,
							size: uploaded.size,
							contentType: uploaded.contentType,
							uploading: false,
						},
			)
		})
		.catch((error: unknown) => {
			console.error(
				isImage ? "Failed to upload image:" : "Failed to upload file:",
				error,
			)
			updateAttrsByUid(editor, nodeName, blockId, { uploading: false })
			showToastMessage(
				"error",
				$t(
					isImage
						? "editor.image.errors.upload-failed"
						: "editor.file.errors.upload-failed",
				),
			)
		})
}

async function uploadFile(
	documentId: string,
	blockId: string,
	isImage: boolean,
	file: File,
): Promise<UploadedDocumentFile> {
	const { uploadDocumentFile } = useDocumentFileAPI()

	return await uploadDocumentFile.mutateAsync({
		documentId,
		id: blockId,
		loc: DocumentFileLocation.Document,
		kind: isImage ? DocumentFileKind.Image : DocumentFileKind.File,
		file,
	})
}

function updateAttrsByUid(
	editor: Editor,
	nodeName: string,
	uid: string,
	attrs: Record<string, unknown>,
): void {
	const { doc } = editor.state

	// -1 sentinel instead of null: typescript does not track assignments made
	// inside the descendants callback, so a null check here would be reported
	// as always-true and the closure below would need non-null assertions.
	let nodePos = -1

	doc.descendants((node, pos) => {
		if (node.type.name === nodeName && node.attrs.uid === uid) {
			nodePos = pos
			return false
		}

		return true
	})

	if (nodePos === -1) {
		return
	}

	editor.commands.command(({ tr }) => {
		const node = tr.doc.nodeAt(nodePos)
		if (!node) {
			return false
		}

		tr.setNodeMarkup(nodePos, undefined, { ...node.attrs, ...attrs })
		return true
	})
}
