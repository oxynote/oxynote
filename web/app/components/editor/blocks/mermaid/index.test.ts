import { getExtensionField } from "@tiptap/core"
import type { DOMOutputSpec, Node as PMNode } from "@tiptap/pm/model"
import { Schema } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { describe, it } from "vitest"
import { MermaidBlock, TAB_SIZE } from "./index"
import { MERMAID_BLOCK_NAME } from "../node-names"
import {
	docBuilder,
	extensionContext,
	shortcutsAt as extensionShortcutsAt,
} from "~/components/editor/test-helpers"

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		[MERMAID_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			code: true,
			defining: true,
			isolating: true,
			whitespace: "pre",
		},
		paragraph: { group: "block", content: "inline*" },
		horizontalRule: { group: "block" },
		text: { group: "inline" },
	},
})

// the "no paragraph in the schema" branches need a schema that really
// lacks one, which rules out reusing the fixture above
const schemaWithoutParagraph = new Schema({
	nodes: {
		doc: { content: `${MERMAID_BLOCK_NAME}+` },
		[MERMAID_BLOCK_NAME]: { group: "block", content: "text*", code: true },
		text: { group: "inline" },
	},
})

type TestSchema = typeof schema | typeof schemaWithoutParagraph

function mermaid(source: string, target: TestSchema = schema): PMNode {
	return target.nodes[MERMAID_BLOCK_NAME].create(
		null,
		source ? target.text(source) : null,
	)
}

function paragraph(text: string): PMNode {
	return schema.nodes.paragraph.create(null, schema.text(text))
}

const docOf = docBuilder(schema)

// every shortcut this suite drives belongs to the mermaid block
const shortcutsAt = (docNode: PMNode, anchor: number, head?: number) =>
	extensionShortcutsAt(MermaidBlock, MERMAID_BLOCK_NAME, docNode, anchor, head)

describe("MermaidBlock", () => {
	it("indents with four spaces", ({ expect }) => {
		expect(TAB_SIZE).toBe(4)
	})

	describe("parseHTML", () => {
		it("matches a mermaid pre and keeps its whitespace", ({ expect }) => {
			const context = extensionContext(MermaidBlock, MERMAID_BLOCK_NAME)
			const parseHTML = getExtensionField<() => unknown[]>(
				MermaidBlock,
				"parseHTML",
				context,
			)

			expect(parseHTML()).toEqual([
				{ tag: `pre[data-type="mermaid-block"]`, preserveWhitespace: "full" },
			])
		})
	})

	describe("renderHTML", () => {
		it("wraps the content in a tagged pre and a code hole", ({ expect }) => {
			const context = extensionContext(MermaidBlock, MERMAID_BLOCK_NAME)
			const renderHTML = getExtensionField<
				(props: { HTMLAttributes: Record<string, unknown> }) => DOMOutputSpec
			>(MermaidBlock, "renderHTML", context)

			expect(renderHTML({ HTMLAttributes: { class: "custom" } })).toEqual([
				"pre",
				{ class: "custom", "data-type": "mermaid-block" },
				["code", 0],
			])
		})
	})

	describe("addInputRules", () => {
		it("registers none, so markdown fences never create the block", ({
			expect,
		}) => {
			const context = extensionContext(MermaidBlock, MERMAID_BLOCK_NAME)
			const addInputRules = getExtensionField<() => unknown[]>(
				MermaidBlock,
				"addInputRules",
				context,
			)

			expect(addInputRules()).toEqual([])
		})
	})

	describe("addProseMirrorPlugins", () => {
		it("registers the syntax highlighter for its own node name", ({
			expect,
		}) => {
			const context = extensionContext(MermaidBlock, MERMAID_BLOCK_NAME)
			const addPlugins = getExtensionField<() => Plugin[]>(
				MermaidBlock,
				"addProseMirrorPlugins",
				context,
			)
			const plugins = addPlugins()
			const plugin = plugins[0]

			if (!plugin) {
				throw new Error("MermaidBlock produced no plugin")
			}

			const state = EditorState.create({
				doc: docOf(mermaid("graph LR")),
				plugins: [plugin],
			})

			expect(plugins).toHaveLength(1)
			expect((plugin.getState(state) as DecorationSet).find()).toHaveLength(2)
		})
	})

	describe("addNodeView", () => {
		it("renders through the vue node view", ({ expect }) => {
			const context = extensionContext(MermaidBlock, MERMAID_BLOCK_NAME)
			const addNodeView = getExtensionField<() => unknown>(
				MermaidBlock,
				"addNodeView",
				context,
			)

			expect(addNodeView()).toBeTypeOf("function")
		})
	})

	describe("Tab", () => {
		it("inserts a tab character at the caret", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab")), 2)

			const handled = run.shortcuts.Tab?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.textContent).toBe("a\tb")
		})

		it("ignores the key outside a mermaid block", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("ab")), 2)

			const handled = run.shortcuts.Tab?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("Shift-Tab", () => {
		it.for([
			{
				name: "removes a leading tab",
				source: "\tabc",
				handled: true,
				after: "abc",
			},
			{
				name: "removes at most four leading spaces",
				source: "      abc",
				handled: true,
				after: "  abc",
			},
			{
				name: "removes exactly four leading spaces",
				source: "    abc",
				handled: true,
				after: "abc",
			},
			{
				name: "removes fewer than four leading spaces",
				source: "  abc",
				handled: true,
				after: "abc",
			},
			{
				name: "removes the tab of the line the caret sits on",
				source: "a\n\tb",
				handled: true,
				after: "a\nb",
			},
			{
				name: "leaves an unindented line untouched",
				source: "abc",
				handled: false,
				after: "abc",
			},
		])("$name", ({ source, handled, after }, { expect }) => {
			const run = shortcutsAt(docOf(mermaid(source)), source.length + 1)

			const result = run.shortcuts["Shift-Tab"]?.({ editor: run.editor })

			expect(result).toBe(handled)
			expect(run.state().doc.textContent).toBe(after)
		})

		it("ignores the key outside a mermaid block", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("  ab")), 5)

			const handled = run.shortcuts["Shift-Tab"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("Backspace", () => {
		it("deletes an empty block", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid(""), paragraph("tail")), 1)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.childCount).toBe(1)
			expect(run.state().doc.firstChild?.type.name).toBe("paragraph")
		})

		it("keeps a non-empty block when the caret is at its start", ({
			expect,
		}) => {
			const run = shortcutsAt(docOf(mermaid("ab")), 1)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a caret away from the block start", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab")), 2)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a non-empty selection", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab")), 1, 3)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a mermaid block", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("ab")), 1)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("ArrowDown", () => {
		it("moves the caret into the following text block", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab"), paragraph("tail")), 3)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().selection.from).toBe(5)
		})

		it("selects the following non-text block", ({ expect }) => {
			const run = shortcutsAt(
				docOf(mermaid("ab"), schema.nodes.horizontalRule.create()),
				3,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().selection.from).toBe(4)
			expect(run.state().selection.to).toBe(5)
		})

		it("ignores the key when the block ends the document", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("head"), mermaid("ab")), 9)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a caret away from the block end", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab"), paragraph("tail")), 2)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a non-empty selection", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab"), paragraph("tail")), 1, 3)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a mermaid block", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("ab"), paragraph("tail")), 3)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("Shift-Enter", () => {
		it("appends a paragraph after the block and moves the caret into it", ({
			expect,
		}) => {
			const run = shortcutsAt(docOf(mermaid("ab")), 2)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.childCount).toBe(2)
			expect(run.state().doc.lastChild?.type.name).toBe("paragraph")
			expect(run.state().selection.from).toBe(5)
		})

		it("ignores the key when the schema has no paragraph", ({ expect }) => {
			const run = shortcutsAt(docOf(mermaid("ab", schemaWithoutParagraph)), 2)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a mermaid block", ({ expect }) => {
			const run = shortcutsAt(docOf(paragraph("ab")), 2)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})
})
