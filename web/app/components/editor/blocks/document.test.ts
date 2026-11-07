import { getSchema } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { describe, it } from "vitest"
import { ContentDocument } from "./document"
import { BLOCK_GRID_NAME } from "./node-names"
import {
	BlockGridStub,
	MetricBlockStub,
	nodeType,
} from "~/components/editor/test-helpers"

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	Blockquote,
	BlockGridStub,
	MetricBlockStub,
])

describe("ContentDocument", () => {
	it("takes a grid at the root", ({ expect }) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME)

		expect(schema.topNodeType.contentMatch.matchType(grid)).not.toBeNull()
	})

	it("takes ordinary blocks at the root", ({ expect }) => {
		const paragraph = nodeType(schema, Paragraph.name)

		expect(schema.topNodeType.contentMatch.matchType(paragraph)).not.toBeNull()
	})

	it("keeps grids out of any other block", ({ expect }) => {
		const grid = nodeType(schema, BLOCK_GRID_NAME)
		const blockquote = nodeType(schema, Blockquote.name)

		expect(blockquote.contentMatch.matchType(grid)).toBeNull()
	})
})
