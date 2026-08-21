import type {
	AnyExtension,
	ChainedCommands,
	ExtendedRegExpMatchArray,
	Range,
} from "@tiptap/core"
import { getExtensionField } from "@tiptap/core"
import type { DOMOutputSpec, Node as PMNode } from "@tiptap/pm/model"
import { Schema } from "@tiptap/pm/model"
import { describe, it, vi } from "vitest"
import type { CodeBlockOptions } from "./index"
import {
	CodeBlock,
	CodeBlockTitle,
	lowlight,
	setUpCodeBlockNode,
	TitledCodeBlock,
} from "./index"
import { KeywordColor } from "./keyword"
import {
	docBuilder,
	extensionContext,
	shortcutsAt,
} from "~/components/editor/test-helpers"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "../node-names"

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "inline*" },
		[CODE_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			code: true,
			whitespace: "pre",
			attrs: { language: { default: null } },
		},
		[CODE_BLOCK_TITLE_NAME]: { content: "text*" },
		[TITLED_CODE_BLOCK_NAME]: {
			group: "block",
			content: `${CODE_BLOCK_TITLE_NAME} ${CODE_BLOCK_NAME}`,
		},
		[METRIC_BLOCK_NAME]: { group: "block" },
		[SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME]: {
			group: "block",
			content: `(${TITLED_CODE_BLOCK_NAME} | ${METRIC_BLOCK_NAME})+`,
		},
		horizontalRule: { group: "block" },
		text: { group: "inline" },
	},
})

// the "no paragraph in the schema" branches need a schema that really
// lacks one, which rules out reusing the fixture above
const schemaWithoutParagraph = new Schema({
	nodes: {
		doc: { content: `${CODE_BLOCK_NAME}+` },
		[CODE_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			code: true,
			attrs: { language: { default: null } },
		},
		text: { group: "inline" },
	},
})

type TestSchema = typeof schema | typeof schemaWithoutParagraph

function code(text: string, target: TestSchema = schema): PMNode {
	return target.nodes[CODE_BLOCK_NAME].create(
		null,
		text ? target.text(text) : null,
	)
}

function title(text: string): PMNode {
	return schema.nodes[CODE_BLOCK_TITLE_NAME].create(null, schema.text(text))
}

function titled(titleText: string, codeText: string): PMNode {
	return schema.nodes[TITLED_CODE_BLOCK_NAME].create(null, [
		title(titleText),
		code(codeText),
	])
}

function paragraph(text: string): PMNode {
	return schema.nodes.paragraph.create(null, schema.text(text))
}

function rightSide(...children: PMNode[]): PMNode {
	return schema.nodes[SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME].create(
		null,
		children,
	)
}

const docOf = docBuilder(schema)

describe("lowlight", () => {
	it("registers curl alongside the common grammars", ({ expect }) => {
		expect(lowlight.listLanguages()).toContain("curl")
		expect(lowlight.listLanguages()).toContain("go")
	})
})

describe("CodeBlockTitle", () => {
	describe("parseHTML", () => {
		it("matches a tagged title div", ({ expect }) => {
			const parseHTML = getExtensionField<() => unknown[]>(
				CodeBlockTitle,
				"parseHTML",
				extensionContext(CodeBlockTitle, CODE_BLOCK_TITLE_NAME),
			)

			expect(parseHTML()).toEqual([
				{ tag: `div[data-type="code-block-title"]` },
			])
		})
	})

	describe("renderHTML", () => {
		it("renders a tagged div with a content hole", ({ expect }) => {
			const renderHTML = getExtensionField<
				(props: { HTMLAttributes: Record<string, unknown> }) => DOMOutputSpec
			>(
				CodeBlockTitle,
				"renderHTML",
				extensionContext(CodeBlockTitle, CODE_BLOCK_TITLE_NAME),
			)

			expect(renderHTML({ HTMLAttributes: { class: "custom" } })).toEqual([
				"div",
				{ class: "custom", "data-type": "code-block-title" },
				0,
			])
		})
	})

	describe("addExtensions", () => {
		it("pulls in the keyword colouring extension", ({ expect }) => {
			const addExtensions = getExtensionField<() => AnyExtension[]>(
				CodeBlockTitle,
				"addExtensions",
				extensionContext(CodeBlockTitle, CODE_BLOCK_TITLE_NAME),
			)

			expect(addExtensions()).toEqual([KeywordColor])
		})
	})

	describe("addNodeView", () => {
		it("renders through the vue node view", ({ expect }) => {
			const addNodeView = getExtensionField<() => unknown>(
				CodeBlockTitle,
				"addNodeView",
				extensionContext(CodeBlockTitle, CODE_BLOCK_TITLE_NAME),
			)

			expect(addNodeView()).toBeTypeOf("function")
		})
	})

	describe("Backspace", () => {
		it("deletes the surrounding titled code block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(titled("t", "ab"), paragraph("tail")),
				2,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.childCount).toBe(1)
			expect(run.state().doc.firstChild?.type.name).toBe("paragraph")
		})

		it("keeps the last code element of a split documentation side", ({
			expect,
		}) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(rightSide(titled("t", "ab"))),
				3,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.dispatched).toHaveLength(0)
		})

		it("deletes the titled code block when the side holds another code element", ({
			expect,
		}) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(
					rightSide(
						titled("t", "ab"),
						schema.nodes[METRIC_BLOCK_NAME].create(),
					),
				),
				3,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.firstChild?.childCount).toBe(1)
			expect(run.state().doc.firstChild?.firstChild?.type.name).toBe(
				METRIC_BLOCK_NAME,
			)
		})

		it("ignores a caret away from the title start", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(titled("ta", "ab"), paragraph("tail")),
				3,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a non-empty selection", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(titled("ta", "ab"), paragraph("tail")),
				2,
				4,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a code block title", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlockTitle,
				CODE_BLOCK_TITLE_NAME,
				docOf(paragraph("ab")),
				1,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})
})

describe("CodeBlock", () => {
	describe("addOptions", () => {
		it("defaults to a plain document code block driven by the shared lowlight", ({
			expect,
		}) => {
			expect(CodeBlock.options.defaultLanguage).toBeNull()
			expect(CodeBlock.options.enableTabIndentation).toBe(true)
			expect(CodeBlock.options.exitOnTripleEnter).toBe(false)
			expect(CodeBlock.options.lowlight).toBe(lowlight)
			expect(CodeBlock.options.type).toBe("document")
			expect(CodeBlock.options.languageClassPrefix).toBe("language-")
		})
	})

	describe("parseHTML", () => {
		it("matches a code pre and keeps its whitespace", ({ expect }) => {
			const parseHTML = getExtensionField<() => unknown[]>(
				CodeBlock,
				"parseHTML",
				extensionContext(CodeBlock, CODE_BLOCK_NAME),
			)

			expect(parseHTML()).toEqual([
				{ tag: `pre[data-type="code-block"]`, preserveWhitespace: "full" },
			])
		})
	})

	describe("renderHTML", () => {
		function renderWith(
			node: PMNode,
			options: CodeBlockOptions = CodeBlock.options,
		): DOMOutputSpec {
			const renderHTML = getExtensionField<
				(props: {
					node: PMNode
					HTMLAttributes: Record<string, unknown>
				}) => DOMOutputSpec
			>(CodeBlock, "renderHTML", {
				...extensionContext(CodeBlock, CODE_BLOCK_NAME),
				options,
			})

			return renderHTML({ node, HTMLAttributes: { class: "custom" } })
		}

		it("prefixes the language onto the code element class", ({ expect }) => {
			const node = schema.nodes[CODE_BLOCK_NAME].create({ language: "go" })

			expect(renderWith(node)).toEqual([
				"pre",
				{ class: "custom", "data-type": "code-block" },
				["code", { class: "language-go" }, 0],
			])
		})

		it("leaves the code element unclassed without a language", ({ expect }) => {
			const node = schema.nodes[CODE_BLOCK_NAME].create()

			expect(renderWith(node)).toEqual([
				"pre",
				{ class: "custom", "data-type": "code-block" },
				["code", { class: null }, 0],
			])
		})

		it("falls back to a bare language class without a prefix option", ({
			expect,
		}) => {
			const node = schema.nodes[CODE_BLOCK_NAME].create({ language: "go" })

			expect(
				renderWith(node, { ...CodeBlock.options, languageClassPrefix: null }),
			).toEqual([
				"pre",
				{ class: "custom", "data-type": "code-block" },
				["code", { class: "go" }, 0],
			])
		})
	})

	describe("addInputRules", () => {
		it("registers none, so markdown fences never create the block", ({
			expect,
		}) => {
			const addInputRules = getExtensionField<() => unknown[]>(
				CodeBlock,
				"addInputRules",
				extensionContext(CodeBlock, CODE_BLOCK_NAME),
			)

			expect(addInputRules()).toEqual([])
		})
	})

	describe("addKeyboardShortcuts", () => {
		it("keeps the inherited shortcuts alongside its own", ({ expect }) => {
			const run = shortcutsAt(CodeBlock, CODE_BLOCK_NAME, docOf(code("ab")), 1)

			expect(Object.keys(run.shortcuts)).toEqual(
				expect.arrayContaining([
					"Mod-Alt-c",
					"Backspace",
					"ArrowDown",
					"Shift-Enter",
				]),
			)
		})
	})

	describe("addNodeView", () => {
		it("renders through the vue node view", ({ expect }) => {
			const addNodeView = getExtensionField<() => unknown>(
				CodeBlock,
				"addNodeView",
				extensionContext(CodeBlock, CODE_BLOCK_NAME),
			)

			expect(addNodeView()).toBeTypeOf("function")
		})
	})

	describe("Backspace", () => {
		it("converts the block into a paragraph keeping its text", ({ expect }) => {
			const run = shortcutsAt(CodeBlock, CODE_BLOCK_NAME, docOf(code("ab")), 1)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.firstChild?.type.name).toBe("paragraph")
			expect(run.state().doc.textContent).toBe("ab")
			expect(run.state().selection.from).toBe(1)
		})

		it("keeps a code block that belongs to a titled code block", ({
			expect,
		}) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(titled("t", "ab")),
				5,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key when the schema has no paragraph", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab", schemaWithoutParagraph)),
				1,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a caret away from the block start", ({ expect }) => {
			const run = shortcutsAt(CodeBlock, CODE_BLOCK_NAME, docOf(code("ab")), 2)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a non-empty selection", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab")),
				1,
				3,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a code block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(paragraph("ab")),
				1,
			)

			const handled = run.shortcuts.Backspace?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("ArrowDown", () => {
		it("moves the caret into the following text block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab"), paragraph("tail")),
				3,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().selection.from).toBe(5)
		})

		it("selects the following non-text block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab"), schema.nodes.horizontalRule.create()),
				3,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().selection.from).toBe(4)
			expect(run.state().selection.to).toBe(5)
		})

		it("ignores the key when the block ends the document", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(paragraph("head"), code("ab")),
				9,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a caret away from the block end", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab"), paragraph("tail")),
				2,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores a non-empty selection", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab"), paragraph("tail")),
				1,
				3,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a code block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(paragraph("ab"), paragraph("tail")),
				3,
			)

			const handled = run.shortcuts.ArrowDown?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})

	describe("Shift-Enter", () => {
		it("appends a paragraph after the block and moves the caret into it", ({
			expect,
		}) => {
			const run = shortcutsAt(CodeBlock, CODE_BLOCK_NAME, docOf(code("ab")), 2)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(true)
			expect(run.state().doc.childCount).toBe(2)
			expect(run.state().doc.lastChild?.type.name).toBe("paragraph")
			expect(run.state().selection.from).toBe(5)
		})

		it("ignores the key inside a titled code block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(titled("t", "ab")),
				5,
			)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key when the schema has no paragraph", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(code("ab", schemaWithoutParagraph)),
				2,
			)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})

		it("ignores the key outside a code block", ({ expect }) => {
			const run = shortcutsAt(
				CodeBlock,
				CODE_BLOCK_NAME,
				docOf(paragraph("ab")),
				2,
			)

			const handled = run.shortcuts["Shift-Enter"]?.({ editor: run.editor })

			expect(handled).toBe(false)
			expect(run.dispatched).toHaveLength(0)
		})
	})
})

describe("TitledCodeBlock", () => {
	describe("parseHTML", () => {
		it("matches a tagged wrapper div", ({ expect }) => {
			const parseHTML = getExtensionField<() => unknown[]>(
				TitledCodeBlock,
				"parseHTML",
				extensionContext(TitledCodeBlock, TITLED_CODE_BLOCK_NAME),
			)

			expect(parseHTML()).toEqual([
				{ tag: `div[data-type="titled-code-block"]` },
			])
		})
	})

	describe("renderHTML", () => {
		it("renders a tagged div with a content hole", ({ expect }) => {
			const renderHTML = getExtensionField<
				(props: { HTMLAttributes: Record<string, unknown> }) => DOMOutputSpec
			>(
				TitledCodeBlock,
				"renderHTML",
				extensionContext(TitledCodeBlock, TITLED_CODE_BLOCK_NAME),
			)

			expect(renderHTML({ HTMLAttributes: { class: "custom" } })).toEqual([
				"div",
				{ class: "custom", "data-type": "titled-code-block" },
				0,
			])
		})
	})
})

describe("setUpCodeBlockNode", () => {
	function stubChain() {
		const chain = {
			deleteRange: vi.fn(() => chain),
			insertContent: vi.fn(() => chain),
			run: vi.fn(() => true),
		}

		return chain as unknown as ChainedCommands & {
			deleteRange: ReturnType<typeof vi.fn>
			insertContent: ReturnType<typeof vi.fn>
			run: ReturnType<typeof vi.fn>
		}
	}

	function match(...groups: (string | undefined)[]): ExtendedRegExpMatchArray {
		return groups as unknown as ExtendedRegExpMatchArray
	}

	const range: Range = { from: 1, to: 5 }

	it("replaces the matched range with a code block in the matched language", ({
		expect,
	}) => {
		const chain = stubChain()

		setUpCodeBlockNode(range, match("```Go", "Go"), () => chain)

		expect(chain.deleteRange).toHaveBeenCalledTimes(1)
		expect(chain.deleteRange).toHaveBeenCalledWith(range)
		expect(chain.insertContent).toHaveBeenCalledTimes(1)
		expect(chain.insertContent).toHaveBeenCalledWith({
			type: CODE_BLOCK_NAME,
			attrs: { language: "go" },
		})
		expect(chain.run).toHaveBeenCalledTimes(1)
	})

	it("leaves the language empty when the match captured none", ({ expect }) => {
		const chain = stubChain()

		setUpCodeBlockNode(range, match("```", undefined), () => chain)

		expect(chain.insertContent).toHaveBeenCalledWith({
			type: CODE_BLOCK_NAME,
			attrs: { language: "" },
		})
		expect(chain.run).toHaveBeenCalledTimes(1)
	})
})
