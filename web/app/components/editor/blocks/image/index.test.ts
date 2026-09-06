import { getSchema } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { NodeType } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { IMAGE_BLOCK_NAME } from "../node-names"
import UniqueID from "../../tiptap-utils/unique-id"
import { ImageBlock } from "."
import { nodeType, parseAttributes } from "~/components/editor/test-helpers"

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
