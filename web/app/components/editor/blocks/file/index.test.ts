import { getSchema } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { NodeType } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { FILE_BLOCK_NAME } from "../node-names"
import { FileBlock } from "."
import { nodeType, parseAttributes } from "~/components/editor/test-helpers"

const schema = getSchema([Document, Text, Paragraph, FileBlock])

function fileType(): NodeType {
	return nodeType(schema, FILE_BLOCK_NAME)
}

describe("FileBlock", () => {
	it("defines an unselectable, undraggable atomic block", ({ expect }) => {
		expect(fileType().spec).toMatchObject({
			group: "block",
			atom: true,
			draggable: false,
			selectable: false,
		})
	})

	it("matches only file block markers when parsing html", ({ expect }) => {
		expect(fileType().spec.parseDOM?.[0]?.tag).toBe(`a[data-type="file-block"]`)
	})

	it("defaults every attribute to null except the upload flag", ({
		expect,
	}) => {
		expect(fileType().create().attrs).toEqual({
			src: null,
			name: null,
			size: null,
			contentType: null,
			uploading: false,
		})
	})

	it("renders the file attributes onto an anchor", ({ expect }) => {
		const node = fileType().create({
			src: "/files/d1/b1",
			name: "notes.zip",
			size: 2048,
			contentType: "application/zip",
		})

		expect(fileType().spec.toDOM?.(node)).toEqual([
			"a",
			{
				href: "/files/d1/b1",
				"data-name": "notes.zip",
				"data-size": 2048,
				"data-content-type": "application/zip",
				"data-type": "file-block",
			},
		])
	})

	it("keeps the transient upload flag and unset attributes out of the html", ({
		expect,
	}) => {
		const node = fileType().create({ src: "/files/d1/b1", uploading: true })

		expect(fileType().spec.toDOM?.(node)).toEqual([
			"a",
			{
				href: "/files/d1/b1",
				"data-type": "file-block",
			},
		])
	})

	it("parses the attributes back off an anchor", ({ expect }) => {
		expect(
			parseAttributes(fileType(), {
				href: "/files/d1/b1",
				"data-name": "notes.zip",
				"data-size": "2048",
				"data-content-type": "application/zip",
			}),
		).toEqual({
			src: "/files/d1/b1",
			name: "notes.zip",
			size: 2048,
			contentType: "application/zip",
		})
	})

	// tiptap drops null results from an attribute parser, so the node
	// falls back to the attribute defaults
	it("parses an anchor without data attributes into none", ({ expect }) => {
		expect(parseAttributes(fileType(), { href: "/files/d1/b1" })).toEqual({
			src: "/files/d1/b1",
		})
	})

	it("round-trips a node through render and parse", ({ expect }) => {
		const node = fileType().create({
			src: "/files/d1/b1",
			name: "notes.zip",
			size: 2048,
			contentType: "application/zip",
		})
		const rendered = fileType().spec.toDOM?.(node) as [
			string,
			Record<string, string>,
		]

		expect(parseAttributes(fileType(), rendered[1])).toEqual({
			src: "/files/d1/b1",
			name: "notes.zip",
			size: 2048,
			contentType: "application/zip",
		})
	})
})
