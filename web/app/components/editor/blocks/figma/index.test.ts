import { getSchema } from "@tiptap/core"
import Blockquote from "@tiptap/extension-blockquote"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { NodeType } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { describe, it, vi } from "vitest"
import { FIGMA_BLOCK_NAME } from "../node-names"
import { nodeType, parseAttributes } from "~/components/editor/test-helpers"
import {
	convertToEmbedUrl,
	createFigmaLinkHandler,
	FigmaBlock,
	isFigmaUrl,
} from "."

const schema = getSchema([Document, Text, Paragraph, Blockquote, FigmaBlock])

// the same schema without the figma node, for the branch where the
// pasted url has nowhere to go
const schemaWithoutFigma = getSchema([Document, Text, Paragraph])

function figmaType(): NodeType {
	return nodeType(schema, FIGMA_BLOCK_NAME)
}

// pulls the paste handler out of the extension's plugin; the factory
// reads nothing off the extension instance
function pasteHandler(): (view: EditorView, event: ClipboardEvent) => boolean {
	const addPlugins = createFigmaLinkHandler().config.addProseMirrorPlugins as
		(() => Plugin[]) | undefined

	if (!addPlugins) {
		throw new Error("the figma link handler registers no plugins")
	}

	const handler = addPlugins()[0]?.props.handlePaste
	if (!handler) {
		throw new Error("the figma link handler plugin handles no paste")
	}

	return handler as (view: EditorView, event: ClipboardEvent) => boolean
}

function pasteEvent(text?: string): ClipboardEvent {
	const clipboardData = text === undefined ? undefined : { getData: () => text }

	return { clipboardData } as unknown as ClipboardEvent
}

// a view stand-in exposing only the state and the dispatch the paste
// handler touches, applying transactions so the result is observable
function makeView(state: EditorState) {
	let current = state
	const dispatch = vi.fn((tr: Parameters<EditorView["dispatch"]>[0]) => {
		current = current.apply(tr)
	})

	const view = {
		get state() {
			return current
		},
		dispatch,
	} as unknown as EditorView

	return { view, dispatch, state: () => current }
}

describe("isFigmaUrl", () => {
	it.for([
		{ name: "accepts a file url", url: "https://www.figma.com/file/abc123/Ui" },
		{ name: "accepts a design url", url: "https://figma.com/design/abc123" },
		{ name: "accepts a board url", url: "https://figma.com/board/abc123" },
		{ name: "accepts a proto url", url: "https://figma.com/proto/abc123" },
		{ name: "accepts plain http", url: "http://figma.com/file/abc123" },
		{
			name: "accepts surrounding whitespace",
			url: "  \nhttps://figma.com/file/a  ",
		},
	])("$name", ({ url }, { expect }) => {
		expect(isFigmaUrl(url)).toBe(true)
	})

	it.for([
		{ name: "rejects an empty string", url: "" },
		{ name: "rejects a url without a scheme", url: "figma.com/file/abc123" },
		{ name: "rejects another host", url: "https://notfigma.com/file/abc123" },
		{
			name: "rejects an unknown path type",
			url: "https://figma.com/community/abc",
		},
		{
			name: "rejects a type without a file key",
			url: "https://figma.com/file/",
		},
		{
			name: "rejects a url that only contains a figma link",
			url: "see https://figma.com/file/abc",
		},
	])("$name", ({ url }, { expect }) => {
		expect(isFigmaUrl(url)).toBe(false)
	})
})

describe("convertToEmbedUrl", () => {
	it.for([
		{
			name: "rewrites a file url to a design embed",
			src: "https://www.figma.com/file/KEY/My-Design",
			isDark: false,
			expected:
				"https://embed.figma.com/design/KEY?embed-host=oxynote&theme=light",
		},
		{
			name: "keeps a design url as a design embed",
			src: "https://www.figma.com/design/KEY/My-Design",
			isDark: false,
			expected:
				"https://embed.figma.com/design/KEY?embed-host=oxynote&theme=light",
		},
		{
			name: "keeps a board url as a board embed",
			src: "https://figma.com/board/KEY/Board",
			isDark: false,
			expected:
				"https://embed.figma.com/board/KEY?embed-host=oxynote&theme=light",
		},
		{
			name: "keeps a proto url as a proto embed",
			src: "https://figma.com/proto/KEY/Proto",
			isDark: false,
			expected:
				"https://embed.figma.com/proto/KEY?embed-host=oxynote&theme=light",
		},
		{
			name: "asks for the dark theme when the editor is dark",
			src: "https://figma.com/design/KEY",
			isDark: true,
			expected:
				"https://embed.figma.com/design/KEY?embed-host=oxynote&theme=dark",
		},
		{
			name: "carries the node id over to the embed",
			src: "https://figma.com/design/KEY/Name?node-id=12-34&t=xyz",
			isDark: false,
			expected:
				"https://embed.figma.com/design/KEY?embed-host=oxynote&theme=light&node-id=12-34",
		},
	])("$name", ({ src, isDark, expected }, { expect }) => {
		expect(convertToEmbedUrl(src, isDark)).toBe(expected)
	})

	// the caller is expected to have checked isFigmaUrl first; without a
	// type and a file key the embed url is built from undefined segments
	it("builds an unusable embed url for a non-figma source", ({ expect }) => {
		expect(convertToEmbedUrl("https://example.com/", false)).toBe(
			"https://embed.figma.com/undefined/undefined?embed-host=oxynote&theme=light",
		)
	})

	it("throws for a string that is not a url", ({ expect }) => {
		expect(() => convertToEmbedUrl("not a url", false)).toThrow()
	})
})

describe("FigmaBlock", () => {
	it("defines an unselectable, undraggable atomic block", ({ expect }) => {
		expect(figmaType().spec).toMatchObject({
			group: "block",
			atom: true,
			draggable: false,
			selectable: false,
		})
	})

	it("matches only figma block markers when parsing html", ({ expect }) => {
		expect(figmaType().spec.parseDOM?.[0]?.tag).toBe(
			`div[data-type="figma-block"]`,
		)
	})

	it("renders every attribute as a data attribute", ({ expect }) => {
		const node = figmaType().create({
			src: "https://figma.com/design/KEY",
			width: 640,
			height: 480,
		})

		expect(figmaType().spec.toDOM?.(node)).toEqual([
			"div",
			{
				"data-src": "https://figma.com/design/KEY",
				"data-width": 640,
				"data-height": 480,
				"data-type": "figma-block",
			},
		])
	})

	it("omits unset attributes when rendering", ({ expect }) => {
		const node = figmaType().create()

		expect(figmaType().spec.toDOM?.(node)).toEqual([
			"div",
			{ "data-type": "figma-block" },
		])
	})

	it("defaults every attribute to null", ({ expect }) => {
		expect(figmaType().create().attrs).toEqual({
			src: null,
			width: null,
			height: null,
		})
	})

	it("parses the data attributes back, with the sizes as numbers", ({
		expect,
	}) => {
		expect(
			parseAttributes(figmaType(), {
				"data-src": "https://figma.com/design/KEY",
				"data-width": "640",
				"data-height": "480",
			}),
		).toEqual({
			src: "https://figma.com/design/KEY",
			width: 640,
			height: 480,
		})
	})

	// tiptap drops null results from an attribute parser, so the node
	// falls back to the attribute defaults
	it("parses an element without data attributes into no attributes", ({
		expect,
	}) => {
		expect(parseAttributes(figmaType(), {})).toEqual({})
	})

	it("round-trips a node through render and parse", ({ expect }) => {
		const node = figmaType().create({
			src: "https://figma.com/board/KEY",
			width: 320,
			height: 200,
		})
		const rendered = figmaType().spec.toDOM?.(node) as [
			string,
			Record<string, string>,
		]

		expect(parseAttributes(figmaType(), rendered[1])).toEqual(node.attrs)
	})
})

describe("createFigmaLinkHandler", () => {
	it("registers itself under the figma link handler name", ({ expect }) => {
		expect(createFigmaLinkHandler().name).toBe("figmaLinkHandler")
	})

	describe("handlePaste", () => {
		it("replaces the empty paragraph with a figma block", ({ expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "intro" }] },
					{ type: "paragraph" },
				],
			})
			const { view, dispatch, state } = makeView(
				EditorState.create({
					doc,
					selection: TextSelection.create(doc, 8),
				}),
			)

			const handled = pasteHandler()(
				view,
				pasteEvent("https://figma.com/design/KEY"),
			)

			expect(handled).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(1)
			expect(state().doc.toJSON()).toEqual({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "intro" }] },
					{
						type: FIGMA_BLOCK_NAME,
						attrs: {
							src: "https://figma.com/design/KEY",
							width: null,
							height: null,
						},
					},
				],
			})
		})

		it("trims the pasted url before storing it", ({ expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [{ type: "paragraph" }],
			})
			const { view, state } = makeView(EditorState.create({ doc }))

			expect(
				pasteHandler()(view, pasteEvent("  https://figma.com/file/KEY  ")),
			).toBe(true)
			expect(state().doc.child(0).attrs.src).toBe("https://figma.com/file/KEY")
		})

		it.for([
			{ name: "ignores a paste without clipboard data", text: undefined },
			{ name: "ignores an empty paste", text: "   " },
			{ name: "ignores a non-figma url", text: "https://example.com/a" },
		])("$name", ({ text }, { expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [{ type: "paragraph" }],
			})
			const { view, dispatch } = makeView(EditorState.create({ doc }))

			expect(pasteHandler()(view, pasteEvent(text))).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("ignores a paste over a non-empty selection", ({ expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "select" }] },
				],
			})
			const { view, dispatch } = makeView(
				EditorState.create({
					doc,
					selection: TextSelection.create(doc, 1, 4),
				}),
			)

			expect(
				pasteHandler()(view, pasteEvent("https://figma.com/design/KEY")),
			).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("ignores a paste nested deeper than the root level", ({ expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [
					{
						type: "blockquote",
						content: [{ type: "blockquote", content: [{ type: "paragraph" }] }],
					},
				],
			})
			const { view, dispatch } = makeView(EditorState.create({ doc }))

			expect(
				pasteHandler()(view, pasteEvent("https://figma.com/design/KEY")),
			).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("accepts a paste one level deep", ({ expect }) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [{ type: "blockquote", content: [{ type: "paragraph" }] }],
			})
			const { view, dispatch } = makeView(EditorState.create({ doc }))

			expect(
				pasteHandler()(view, pasteEvent("https://figma.com/design/KEY")),
			).toBe(true)
			expect(dispatch).toHaveBeenCalledTimes(1)
		})

		it("ignores a paste into a paragraph that already has content", ({
			expect,
		}) => {
			const doc = schema.nodeFromJSON({
				type: "doc",
				content: [
					{ type: "paragraph", content: [{ type: "text", text: "taken" }] },
				],
			})
			const { view, dispatch } = makeView(EditorState.create({ doc }))

			expect(
				pasteHandler()(view, pasteEvent("https://figma.com/design/KEY")),
			).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
		})

		it("ignores a paste when the schema has no figma block", ({ expect }) => {
			const doc = schemaWithoutFigma.nodeFromJSON({
				type: "doc",
				content: [{ type: "paragraph" }],
			})
			const { view, dispatch } = makeView(EditorState.create({ doc }))

			expect(
				pasteHandler()(view, pasteEvent("https://figma.com/design/KEY")),
			).toBe(false)
			expect(dispatch).toHaveBeenCalledTimes(0)
			expect(schemaWithoutFigma.nodes[FIGMA_BLOCK_NAME]).toBeUndefined()
		})
	})
})
