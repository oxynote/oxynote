import type { Content, Extensions } from "@tiptap/core"
import { Editor, getSchema } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Text from "@tiptap/extension-text"
import { describe, it, vi } from "vitest"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "./blocks/node-names"
import { COMMENT_MARK_NAME } from "./mark-names"
import { contentExtensionsWithIDs, editorProseClass } from "./schema-extensions"
import {
	markType,
	MetricBlockStub,
	MetricGridStub,
	nodeType,
	parseAttributes,
} from "./test-helpers"

function makeExtensions(): Extensions {
	return contentExtensionsWithIDs((path) => path, false, {
		onCommentActivated: vi.fn(),
	} as never)
}

function withMetricStubs(extensions: Extensions): Extensions {
	return extensions.map((extension) => {
		switch (extension.name) {
			case METRIC_GRID_NAME:
				return MetricGridStub
			case METRIC_BLOCK_NAME:
				return MetricBlockStub
			default:
				return extension
		}
	})
}

const schema = getSchema([Document, Text, ...withMetricStubs(makeExtensions())])

function extensionByName(name: string) {
	const extension = makeExtensions().find(
		(candidate) => candidate.name === name,
	)
	if (!extension) {
		throw new Error(`extension "${name}" is missing from the content set`)
	}

	return extension
}

// reproduces the keystroke prosemirror would deliver on typing, which
// is the only way input rules run without a browser. Every extension
// contributing input rules gets its own plugin, so the handlers are
// walked in plugin order until one claims the input — the editor's own
// someProp is off limits while the view is unmounted, and an unmounted
// editor's state carries no plugins either.
function typeText(editor: Editor, pos: number, text: string): boolean {
	for (const plugin of editor.extensionManager.plugins) {
		const handler = plugin.props.handleTextInput

		if (
			handler?.call(plugin, editor.view, pos, pos, text, () => editor.state.tr)
		) {
			return true
		}
	}

	return false
}

describe("contentExtensionsWithIDs", () => {
	it("appends the comment mark, the unique ids and the placeholder to the node extensions", ({
		expect,
	}) => {
		const names = makeExtensions().map((extension) => extension.name)

		expect(names.slice(-3)).toEqual([
			COMMENT_MARK_NAME,
			"uniqueID",
			"defaultContentPlaceholder",
		])
		expect(names).toEqual(expect.arrayContaining(["blockquote", "heading"]))
	})

	it("configures the comment mark with the given options", ({ expect }) => {
		const onCommentActivated = vi.fn()
		const extensions = contentExtensionsWithIDs((path) => path, false, {
			onCommentActivated,
		} as never)
		const commentMark = extensions.find(
			(extension) => extension.name === COMMENT_MARK_NAME,
		)

		expect(commentMark?.options).toMatchObject({ onCommentActivated })
	})

	it("registers the unique id extension for the node extensions only", ({
		expect,
	}) => {
		const extensions = makeExtensions()
		const uniqueID = extensionByName("uniqueID")
		const { types } = uniqueID.options as { types: string[] }

		expect(uniqueID.options).toMatchObject({ attributeName: "uid" })
		expect(types).toEqual(
			extensions
				.slice(0, -3)
				.filter((extension) => extension.type === "node")
				.map((extension) => extension.name),
		)
		expect(types).toEqual(expect.arrayContaining(["paragraph", "imageBlock"]))
		expect(types).not.toContain("bold")
		expect(types).not.toContain("link")
		expect(types).not.toContain("listKeymap")
	})

	it("generates a distinct id on every call", ({ expect }) => {
		const { generateID } = extensionByName("uniqueID").options as {
			generateID: () => string
		}

		const ids = [generateID(), generateID(), generateID()]

		expect(new Set(ids).size).toBe(3)
		expect(ids.every((id) => id.length === 21)).toBe(true)
	})

	describe("the resulting schema", () => {
		it.for([
			"paragraph",
			"heading",
			"blockquote",
			"horizontalRule",
			"bulletList",
			"orderedList",
			"listItem",
			"taskList",
			"taskItem",
			"calloutBlock",
			"imageBlock",
			"mermaidBlock",
			"figmaBlock",
			"splitDocumentation",
			"splitDocumentationLeftSide",
			"splitDocumentationRightSide",
			"titledCodeBlock",
			"codeBlock",
			"codeBlockTitle",
			"splitDocumentationParameterList",
		])("defines the %s node", (name, { expect }) => {
			expect(schema.nodes[name]).toBeDefined()
		})

		it.for([
			"bold",
			"code",
			"italic",
			"link",
			"strike",
			"underline",
			COMMENT_MARK_NAME,
		])("defines the %s mark", (name, { expect }) => {
			expect(schema.marks[name]).toBeDefined()
		})

		it("lets headings carry comment marks only", ({ expect }) => {
			expect(nodeType(schema, "heading").spec.marks).toBe(COMMENT_MARK_NAME)
			expect(nodeType(schema, "paragraph").spec.marks).toBeUndefined()
		})

		it("restricts headings to the first three levels", ({ expect }) => {
			expect(extensionByName("heading").options).toMatchObject({
				levels: [1, 2, 3],
			})
		})

		it("makes the link mark non-inclusive", ({ expect }) => {
			expect(markType(schema, "link").spec.inclusive).toBe(false)
		})

		it("renders links with the shared link classes", ({ expect }) => {
			const mark = markType(schema, "link").create({
				href: "https://example.com",
			})
			const rendered = markType(schema, "link").spec.toDOM?.(mark, false) as [
				string,
				Record<string, string>,
			]

			expect(rendered[0]).toBe("a")
			expect(rendered[1].class).toContain("text-primary")
			expect(rendered[1].href).toBe("https://example.com")
		})
	})

	describe("the uid attribute", () => {
		it.for(["paragraph", "heading", "calloutBlock", "imageBlock"])(
			"adds a uid attribute to the %s node",
			(name, { expect }) => {
				expect(nodeType(schema, name).spec.attrs?.uid).toEqual({
					default: null,
				})
			},
		)

		it.for(["bold", "code", "italic", "link", "strike", "underline"])(
			"leaves the %s mark without a uid attribute",
			(name, { expect }) => {
				expect(markType(schema, name).spec.attrs?.uid).toBeUndefined()
			},
		)

		it("renders the uid as both a data attribute and a dom id", ({
			expect,
		}) => {
			const node = nodeType(schema, "paragraph").create({ uid: "abc123" })
			const rendered = nodeType(schema, "paragraph").spec.toDOM?.(node) as [
				string,
				Record<string, string>,
			]

			expect(rendered[1]).toMatchObject({ "data-uid": "abc123", id: "abc123" })
		})

		it("renders no uid attributes when the node carries none", ({ expect }) => {
			const node = nodeType(schema, "paragraph").create()
			const rendered = nodeType(schema, "paragraph").spec.toDOM?.(node) as [
				string,
				Record<string, string>,
			]

			expect(rendered[1]).toEqual({})
		})

		it("parses the uid off the data attribute", ({ expect }) => {
			expect(
				parseAttributes(nodeType(schema, "paragraph"), {
					"data-uid": "abc123",
				}),
			).toEqual({ uid: "abc123" })
		})

		it("falls back to the dom id when there is no data attribute", ({
			expect,
		}) => {
			expect(
				parseAttributes(nodeType(schema, "paragraph"), { id: "abc123" }),
			).toEqual({
				uid: "abc123",
			})
		})

		it("prefers the data attribute over the dom id", ({ expect }) => {
			expect(
				parseAttributes(nodeType(schema, "paragraph"), {
					"data-uid": "abc123",
					id: "other",
				}),
			).toEqual({ uid: "abc123" })
		})
	})

	describe("the horizontal rule input rule", () => {
		function makeEditor(content: Content) {
			return new Editor({
				extensions: [Document, Text, ...withMetricStubs(makeExtensions())],
				content,
			})
		}

		it("turns three dashes at the document root into a horizontal rule", ({
			expect,
		}) => {
			const editor = makeEditor({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "--" }] },
				],
			})
			editor.commands.setTextSelection(3)

			const handled = typeText(editor, 3, "-")

			expect(handled).toBe(true)
			expect(editor.state.doc.firstChild?.type.name).toBe("horizontalRule")
		})

		it("leaves three dashes nested inside another node alone", ({ expect }) => {
			const editor = makeEditor({
				type: "doc",
				content: [
					{
						type: "blockquote",
						content: [
							{ type: "paragraph", content: [{ type: "text", text: "--" }] },
						],
					},
				],
			})
			editor.commands.setTextSelection(4)

			const handled = typeText(editor, 4, "-")

			expect(handled).toBe(false)
			expect(editor.state.doc.firstChild?.type.name).toBe("blockquote")
		})
	})
})

describe("editorProseClass", () => {
	it.for([
		"focus:outline-none",
		"prose",
		"dark:prose-invert",
		"lg:[&>*]:mx-12.5",
		"prose-h1:text-[1.5em]",
	])("keeps the %s class after merging", (className, { expect }) => {
		expect(editorProseClass.split(" ")).toContain(className)
	})
})
