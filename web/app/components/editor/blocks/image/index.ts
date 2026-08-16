import { Node, mergeAttributes, type Editor } from "@tiptap/core"
import { FileHandler } from "@tiptap/extension-file-handler"
import { VueNodeViewRenderer } from "@tiptap/vue-3"
import { nanoid } from "nanoid"
import ImageBlockComponent from "./ImageBlock.vue"
import { showToastMessage } from "~/components/toast"
import { IMAGE_BLOCK_NAME } from "../node-names"

export interface ImageFileHandlerOptions {
	documentId?: string | null | undefined
}

export const ImageBlock = Node.create({
	name: IMAGE_BLOCK_NAME,
	group: "block",
	atom: true,
	draggable: false,
	selectable: false,
	addAttributes() {
		return {
			src: {
				default: null,
			},
			alt: {
				default: null,
			},
			title: {
				default: null,
			},
			width: {
				default: null,
				parseHTML: (element) => {
					const width = element.getAttribute("width")
					return width ? Number.parseInt(width, 10) : null
				},
				renderHTML: (attrs) => {
					if (!attrs.width) {
						return {}
					}

					return { width: attrs.width as number }
				},
			},
			uploading: {
				default: false,
				rendered: false,
			},
		}
	},
	parseHTML() {
		return [{ tag: `img[data-type="image-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"img",
			mergeAttributes(HTMLAttributes, {
				"data-type": "image-block",
			}),
		]
	},
	addNodeView() {
		// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		return VueNodeViewRenderer(ImageBlockComponent)
	},
})

async function uploadFile(
	documentId: string,
	blockId: string,
	file: File,
): Promise<string> {
	const { uploadDocumentFile } = useDocumentFileAPI()

	return await uploadDocumentFile.mutateAsync({
		documentId,
		id: blockId,
		loc: DocumentFileLocation.Document,
		file,
	})
}

export function createImageFileHandler(options: ImageFileHandlerOptions) {
	return FileHandler.configure({
		allowedMimeTypes: ["image/png", "image/jpeg", "image/gif", "image/webp"],
		onPaste: (editor, files) => {
			if (!isRootInsertPosition(editor) || !options.documentId) {
				return false
			}

			for (const file of files) {
				insertImageWithUpload(editor, file, options.documentId, undefined)
			}

			return true
		},
		onDrop: (editor, files, pos) => {
			if (!isRootInsertPosition(editor, pos) || !options.documentId) {
				return false
			}

			for (const file of files) {
				insertImageWithUpload(editor, file, options.documentId, pos)
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

function insertImageWithUpload(
	editor: Editor,
	file: File,
	documentId: string,
	pos?: number,
) {
	const blockId = nanoid()
	const { state, view } = editor
	const { schema } = state

	const imageNode = schema.nodes[ImageBlock.name]
	if (!imageNode) {
		return
	}

	const insertPos = pos ?? state.selection.anchor
	const $pos = state.doc.resolve(insertPos)

	// Insert image after the current block
	const paragraphEnd = $pos.after()

	const tr = state.tr
	const image = imageNode.create({ uid: blockId, uploading: true })
	tr.insert(paragraphEnd, image)

	view.dispatch(tr)

	uploadFile(documentId, blockId, file)
		.then(() => {
			const src = buildDocumentFileSrc(documentId, blockId)
			updateImageAttrsByUid(editor, blockId, { src, uploading: false })
		})
		.catch((error: unknown) => {
			console.error("Failed to upload image:", error)
			updateImageAttrsByUid(editor, blockId, { uploading: false })
			showToastMessage("error", $t("editor.image.errors.upload-failed"))
		})
}

function updateImageAttrsByUid(
	editor: Editor,
	uid: string,
	attrs: Record<string, unknown>,
): void {
	const { doc } = editor.state

	// -1 sentinel instead of null: typescript does not track assignments made
	// inside the descendants callback, so a null check here would be reported
	// as always-true and the closure below would need non-null assertions.
	let nodePos = -1

	doc.descendants((node, pos) => {
		if (node.type.name === ImageBlock.name && node.attrs.uid === uid) {
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
