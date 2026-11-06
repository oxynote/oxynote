import Document from "@tiptap/extension-document"
import { ROOT_ONLY_BLOCK_GROUP } from "./node-names"

// the root of a document's content. Only it takes the root-only blocks,
// such as grids.
export const ContentDocument = Document.extend({
	content: `(block | ${ROOT_ONLY_BLOCK_GROUP})+`,
})
