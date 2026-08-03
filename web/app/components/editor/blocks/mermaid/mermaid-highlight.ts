import { findChildren } from "@tiptap/core"
import type { Node as ProsemirrorNode } from "@tiptap/pm/model"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import type { LanguageFn } from "highlight.js"
import { createLowlight } from "lowlight"

// Mermaid syntax grammar for highlight.js / lowlight.
// Built from the official syntax reference: https://mermaid.js.org/intro/syntax-reference.html
const mermaidGrammar: LanguageFn = (hljs) => ({
	name: "Mermaid",
	case_insensitive: false,
	contains: [
		// Comments: %% to end of line (but not directives %%{)
		hljs.COMMENT(/%%(?!\{)/, /$/),

		// Directives: %%{ init: { ... } }%%
		{
			className: "meta",
			begin: /%%\{/,
			end: /\}%%/,
			contains: [hljs.QUOTE_STRING_MODE],
		},

		// YAML frontmatter
		{
			className: "meta",
			begin: /^---$/,
			end: /^---$/,
			contains: [
				{
					className: "attr",
					begin: /[A-Za-z][A-Za-z0-9_-]*/,
					end: /(?=:)/,
				},
				hljs.QUOTE_STRING_MODE,
			],
		},

		// Diagram type declarations
		{
			className: "keyword",
			begin:
				/\b(?:flowchart|graph|sequenceDiagram|classDiagram|stateDiagram(?:-v2)?|erDiagram|journey|pie|gantt|requirementDiagram|gitGraph|mindmap|timeline|sankey(?:-beta)?|xychart(?:-beta)?|block(?:-beta)?|packet(?:-beta)?|kanban|architecture(?:-beta)?|quadrantChart|C4Context|C4Container|C4Component|C4Dynamic|C4Deployment)\b/,
		},

		// Direction keywords
		{
			className: "keyword",
			begin: /\b(?:LR|RL|TB|BT|TD)\b/,
		},

		// Structural / control flow keywords
		{
			className: "keyword",
			begin:
				/\b(?:subgraph|end|loop|alt|else|opt|par|and|critical|option|break|rect|section|namespace|box|over)\b/,
		},

		// Definition & config keywords
		{
			className: "built_in",
			begin:
				/\b(?:participant|actor|class|state|note|title|create|destroy|activate|deactivate|autonumber|showData|direction|as|dateFormat|axisFormat|tickInterval|excludes|inclusiveEndDates|todayMarker|displayMode|classDef|style|linkStyle|click|call|href|commit|branch|checkout|merge|cherry-pick|order)\b/,
		},

		// Task status / modifiers (gantt)
		{
			className: "type",
			begin: /\b(?:done|active|crit|milestone|after|until)\b/,
		},

		// Git graph commit types
		{
			className: "type",
			begin: /\b(?:NORMAL|REVERSE|HIGHLIGHT)\b/,
		},

		// Special state markers: [*] and <<fork>> / <<join>> / <<choice>>
		{
			className: "variable",
			begin: /\[\*\]/,
		},
		{
			className: "variable",
			begin: /<<(?:fork|join|choice)>>/,
		},

		// UML annotations: <<Interface>>, <<Abstract>>, etc.
		{
			className: "type",
			begin: /<</,
			end: />>/,
		},

		// Arrows & edges — ordered from longest/most-specific to shortest
		{
			className: "operator",
			variants: [
				// Bidirectional sequence arrows
				{ begin: /<<-->>/ },
				{ begin: /<<->>/ },
				// Sequence arrows
				{ begin: /-->>/ },
				{ begin: /->>/ },
				{ begin: /--\)/ },
				// Class / ER relationships
				{ begin: /<\|--/ },
				{ begin: /--\|>/ },
				{ begin: /\.\.\|>/ },
				{ begin: /\*--/ },
				{ begin: /o--/ },
				{ begin: /--o/ },
				{ begin: /\.\.>/ },
				// ER cardinality connectors
				{ begin: /[|}][|o]--+[|o][|{]/ },
				{ begin: /[|}][|o]\.\.+[|o][|{]/ },
				// Flowchart thick / dotted
				{ begin: /<=+=>/ },
				{ begin: /={2,}>/ },
				// Dotted arrows
				{ begin: /-\.-+>/ },
				{ begin: /-\.-+/ },
				// Flowchart cross / circle ends
				{ begin: /--+[xo]/ },
				{ begin: /[xo]--+[xo]/ },
				// Standard arrows
				{ begin: /<--+>/ },
				{ begin: /--+>/ },
				{ begin: /---+/ },
				{ begin: /\.\.+/ },
			],
		},

		// Quoted strings
		hljs.QUOTE_STRING_MODE,

		// Hex colors (#fff, #f0f0f0)
		{
			className: "number",
			begin: /#[0-9a-fA-F]{3,8}\b/,
		},

		// Numbers (including percentages and time-like 12:30)
		{
			className: "number",
			begin: /\b\d+(?:[.:]\d+)*%?\b/,
		},

		// CSS-like properties in style definitions
		{
			className: "attr",
			begin:
				/\b(?:fill|stroke|stroke-width|color|font-size|font-weight|font-style|opacity|rx|ry|background)\b/,
		},
	],
})

const lowlight = createLowlight()
lowlight.register("mermaid", mermaidGrammar)

interface ParsedToken {
	text: string
	classes: string[]
}

function parseNodes(nodes: any[], className: string[] = []): ParsedToken[] {
	return nodes.flatMap((node) => {
		const classes = [...className, ...(node.properties?.className ?? [])]

		if (node.children) {
			return parseNodes(node.children, classes)
		}

		return { text: node.value as string, classes }
	})
}

function getDecorations(doc: ProsemirrorNode, name: string) {
	const decorations: Decoration[] = []

	findChildren(doc, (node) => node.type.name === name).forEach((block) => {
		let from = block.pos + 1
		const result = lowlight.highlight("mermaid", block.node.textContent)

		parseNodes(result.children ?? []).forEach((token) => {
			const to = from + token.text.length

			if (token.classes.length) {
				decorations.push(
					Decoration.inline(from, to, {
						class: token.classes.join(" "),
					}),
				)
			}

			from = to
		})
	})

	return DecorationSet.create(doc, decorations)
}

export function mermaidHighlightPlugin(name: string) {
	const pluginKey = new PluginKey("mermaidHighlight")

	const plugin: Plugin<DecorationSet> = new Plugin({
		key: pluginKey,

		state: {
			init: (_, { doc }) => getDecorations(doc, name),
			apply: (transaction, decorationSet, oldState, newState) => {
				const oldNodeName = oldState.selection.$head.parent.type.name
				const newNodeName = newState.selection.$head.parent.type.name
				const oldNodes = findChildren(
					oldState.doc,
					(node) => node.type.name === name,
				)
				const newNodes = findChildren(
					newState.doc,
					(node) => node.type.name === name,
				)

				if (
					transaction.docChanged &&
					([oldNodeName, newNodeName].includes(name) ||
						newNodes.length !== oldNodes.length ||
						transaction.steps.some((step) =>
							oldNodes.some((node) => {
								const s = step as unknown as {
									from?: number
									to?: number
								}

								return (
									s.from !== undefined &&
									s.to !== undefined &&
									node.pos >= s.from &&
									node.pos + node.node.nodeSize <= s.to
								)
							}),
						))
				) {
					return getDecorations(transaction.doc, name)
				}

				return decorationSet.map(transaction.mapping, transaction.doc)
			},
		},

		props: {
			decorations(state) {
				return plugin.getState(state)
			},
		},
	})

	return plugin
}
