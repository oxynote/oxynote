import type { Node as PMNode } from "@tiptap/pm/model"
import { Schema } from "@tiptap/pm/model"
import type { Plugin } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import { describe, it } from "vitest"
import { mermaidHighlightPlugin } from "./mermaid-highlight"
import { MERMAID_BLOCK_NAME } from "../node-names"
import { decorationClassShape } from "~/components/editor/test-helpers"

const schema = new Schema({
	nodes: {
		doc: { content: "block+" },
		[MERMAID_BLOCK_NAME]: {
			group: "block",
			content: "text*",
			code: true,
			whitespace: "pre",
		},
		paragraph: { group: "block", content: "inline*" },
		text: { group: "inline" },
	},
})

function mermaid(source: string): PMNode {
	return schema.nodes[MERMAID_BLOCK_NAME].create(
		null,
		source ? schema.text(source) : null,
	)
}

function paragraph(text: string): PMNode {
	return schema.nodes.paragraph.create(null, schema.text(text))
}

function stateWithBlocks(...blocks: PMNode[]): {
	state: EditorState
	plugin: Plugin
} {
	const plugin = mermaidHighlightPlugin(MERMAID_BLOCK_NAME)
	const state = EditorState.create({
		doc: schema.nodes.doc.create(null, blocks),
		plugins: [plugin],
	})

	return { state, plugin }
}

describe("mermaidHighlightPlugin", () => {
	it.for([
		{
			name: "marks a line comment",
			source: "%% a comment",
			expected: [["hljs-comment", "%% a comment"]],
		},
		{
			name: "marks a diagram declaration and its direction",
			source: "flowchart TD\n  A-->B",
			expected: [
				["hljs-keyword", "flowchart"],
				["hljs-keyword", "TD"],
				["hljs-operator", "-->"],
			],
		},
		{
			name: "marks a versioned diagram declaration and a state marker",
			source: "stateDiagram-v2\n  [*] --> S",
			expected: [
				["hljs-keyword", "stateDiagram-v2"],
				["hljs-variable", "[*]"],
				["hljs-operator", "-->"],
			],
		},
		{
			name: "marks a definition keyword and a sequence arrow",
			source: "sequenceDiagram\n  participant A\n  A->>B: hi",
			expected: [
				["hljs-keyword", "sequenceDiagram"],
				["hljs-built_in", "participant"],
				["hljs-operator", "->>"],
			],
		},
		{
			name: "marks a class relationship and a UML annotation",
			source: "classDiagram\n  A <|-- B\n  class Foo\n  <<Interface>>",
			expected: [
				["hljs-keyword", "classDiagram"],
				["hljs-operator", "<|--"],
				["hljs-built_in", "class"],
				["hljs-type", "<<Interface>>"],
			],
		},
		{
			name: "marks a special state marker",
			source: "stateDiagram\n  s1 --> <<fork>>",
			expected: [
				["hljs-keyword", "stateDiagram"],
				["hljs-operator", "-->"],
				["hljs-variable", "<<fork>>"],
			],
		},
		{
			name: "marks a structural keyword and a gantt task status",
			source: "gantt\n  section S\n  task :done, 2014-01-06",
			expected: [
				["hljs-keyword", "gantt"],
				["hljs-keyword", "section"],
				["hljs-type", "done"],
				["hljs-number", "2014"],
				["hljs-number", "01"],
				["hljs-number", "06"],
			],
		},
		{
			name: "marks a git graph commit type",
			source: "gitGraph\n  commit type: NORMAL",
			expected: [
				["hljs-keyword", "gitGraph"],
				["hljs-built_in", "commit"],
				["hljs-type", "NORMAL"],
			],
		},
		{
			name: "marks a quoted string and a plain number",
			source: 'pie title Pets\n  "Dogs" : 50',
			expected: [
				["hljs-keyword", "pie"],
				["hljs-built_in", "title"],
				["hljs-string", '"Dogs"'],
				["hljs-number", "50"],
			],
		},
		{
			name: "marks a hex color and a style property",
			source: "style A fill:#f9f",
			expected: [
				["hljs-built_in", "style"],
				["hljs-attr", "fill"],
				["hljs-number", "#f9f"],
			],
		},
		{
			name: "marks a hyphenated style property in full",
			source: "style A stroke-width:2px,stroke:#333",
			expected: [
				["hljs-built_in", "style"],
				["hljs-attr", "stroke-width"],
				["hljs-attr", "stroke"],
				["hljs-number", "#333"],
			],
		},
		{
			name: "marks a time-like number",
			source: "timeline\n  12:30 : ev",
			expected: [
				["hljs-keyword", "timeline"],
				["hljs-number", "12:30"],
			],
		},
		{
			name: "marks the longest matching arrow variant",
			source: "graph LR\n  C -.-> D\n  E ==> F\n  G --x H",
			expected: [
				["hljs-keyword", "graph"],
				["hljs-keyword", "LR"],
				["hljs-operator", "-.->"],
				["hljs-operator", "==>"],
				["hljs-operator", "--x"],
			],
		},
		{
			name: "marks an ER cardinality connector",
			source: "erDiagram\n  A ||--o{ B : has",
			expected: [
				["hljs-keyword", "erDiagram"],
				["hljs-operator", "||--o{"],
			],
		},
		{
			name: "marks a circle-ended edge in full",
			source: "classDiagram\n  A o--o B\n  C --o D",
			expected: [
				["hljs-keyword", "classDiagram"],
				["hljs-operator", "o--o"],
				["hljs-operator", "--o"],
			],
		},
		{
			name: "closes the frontmatter and highlights the diagram below it",
			source: "---\ntitle: Hi\n---\nflowchart LR",
			expected: [
				["hljs-meta", "---\n"],
				["hljs-meta hljs-attr", "title"],
				["hljs-meta", ": Hi\n---"],
				["hljs-keyword", "flowchart"],
				["hljs-keyword", "LR"],
			],
		},
		{
			name: "produces no decorations for unrecognised text",
			source: "hello there",
			expected: [],
		},
		{
			name: "produces no decorations for an empty block",
			source: "",
			expected: [],
		},
	])("$name", ({ source, expected }, { expect }) => {
		const { state, plugin } = stateWithBlocks(mermaid(source))

		expect(decorationClassShape(state, plugin)).toEqual(expected)
	})

	it("joins the classes of nested tokens", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(
			mermaid('%%{init: {"theme": "x"} }%%'),
		)

		expect(decorationClassShape(state, plugin)).toEqual([
			["hljs-meta", "%%{init: {"],
			["hljs-meta hljs-string", '"theme"'],
			["hljs-meta", ": "],
			["hljs-meta hljs-string", '"x"'],
			["hljs-meta", "} }%%"],
		])
	})

	it("ignores text outside mermaid blocks", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(paragraph("flowchart TD"))

		expect(decorationClassShape(state, plugin)).toEqual([])
	})

	it("decorates every mermaid block at its own offset", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(
			mermaid("graph LR"),
			paragraph("graph LR"),
			mermaid("pie"),
		)

		expect(decorationClassShape(state, plugin)).toEqual([
			["hljs-keyword", "graph"],
			["hljs-keyword", "LR"],
			["hljs-keyword", "pie"],
		])
	})

	it("exposes the decoration set through the editor props", ({ expect }) => {
		const { state, plugin } = stateWithBlocks(mermaid("graph LR"))

		expect(plugin.props.decorations?.call(plugin, state)).toBe(
			plugin.getState(state),
		)
	})

	it("keeps the decoration set when the document is unchanged", ({
		expect,
	}) => {
		const { state, plugin } = stateWithBlocks(mermaid("graph LR"))

		const next = state.apply(state.tr)

		expect(plugin.getState(next)).toBe(plugin.getState(state))
	})

	it("rebuilds decorations when the edited block is the selected one", ({
		expect,
	}) => {
		const { state, plugin } = stateWithBlocks(mermaid("graph"))

		const next = state.apply(state.tr.insertText(" LR", 6))

		expect(decorationClassShape(next, plugin)).toEqual([
			["hljs-keyword", "graph"],
			["hljs-keyword", "LR"],
		])
	})

	it("rebuilds decorations when a mermaid block is inserted elsewhere", ({
		expect,
	}) => {
		const { state, plugin } = stateWithBlocks(paragraph("ab"), mermaid("graph"))

		const next = state.apply(state.tr.insert(4, mermaid("pie")))

		expect(decorationClassShape(next, plugin)).toEqual([
			["hljs-keyword", "pie"],
			["hljs-keyword", "graph"],
		])
	})

	it("rebuilds decorations when a step replaces a whole mermaid block", ({
		expect,
	}) => {
		const { state, plugin } = stateWithBlocks(
			paragraph("ab"),
			mermaid("graph"),
			paragraph("cd"),
		)

		// the selection stays in the leading paragraph and the block count
		// is unchanged, so only the step range identifies the edit
		const next = state.apply(state.tr.replaceWith(4, 11, mermaid("pie")))

		expect(decorationClassShape(next, plugin)).toEqual([
			["hljs-keyword", "pie"],
		])
	})

	it("maps decorations through an edit outside every mermaid block", ({
		expect,
	}) => {
		const { state, plugin } = stateWithBlocks(paragraph("ab"), mermaid("graph"))
		const selected = state.apply(
			state.tr.setSelection(TextSelection.create(state.doc, 1)),
		)

		const next = selected.apply(selected.tr.insertText("xyz", 1))

		expect(next.doc.textBetween(1, 6)).toBe("xyzab")
		expect(decorationClassShape(next, plugin)).toEqual([
			["hljs-keyword", "graph"],
		])
	})
})
